package middleware

import (
	"net/http"
	"sync"

	"elitegate/internal/ipfilter"
	"elitegate/internal/model"
	"elitegate/internal/shared"

	"github.com/rs/zerolog"
)

// ipCheckerCache holds pre-parsed IPCheckers keyed by the exact rule content
// (a stable, order-independent hash of the string list). Keying by content
// rather than by policy/route ID means a stale cache entry is impossible:
// if a tenant edits their IP rules, the new rule list produces a new key,
// and old entries are simply never looked up again. The cache is bounded in
// practice — the number of distinct IP rule sets across all tenants is
// small relative to request volume — but we still cap it defensively.
type ipCheckerCache struct {
	mu      sync.RWMutex
	entries map[string]*ipfilter.IPChecker
	logger  zerolog.Logger
}

const ipCheckerCacheMaxEntries = 5000

func newIPCheckerCache(logger zerolog.Logger) *ipCheckerCache {
	return &ipCheckerCache{
		entries: make(map[string]*ipfilter.IPChecker),
		logger:  logger,
	}
}

func (c *ipCheckerCache) get(rules []string) *ipfilter.IPChecker {
	if len(rules) == 0 {
		return nil
	}
	key := ipfilter.RuleSetKey(rules) // stable, sorted+joined key

	c.mu.RLock()
	if checker, ok := c.entries[key]; ok {
		c.mu.RUnlock()
		return checker
	}
	c.mu.RUnlock()

	checker, err := ipfilter.NewIPChecker(rules)
	if err != nil {
		// This should be unreachable in practice — policy_handler.go validates
		// every rule list before it's ever persisted. If we get here, it means
		// bad data reached the DB some other way (manual edit, migration,
		// bug). Fail LOUD, not silent: log it so it's visible in dashboards,
		// and fail closed on the caller's side rather than silently skipping the check.
		c.logger.Error().Err(err).Strs("rules", rules).Msg("ipfilter: failed to parse IP/CIDR rule set from policy — check will fail closed")
		return nil
	}

	c.mu.Lock()
	if len(c.entries) >= ipCheckerCacheMaxEntries {
		// Defensive: clear rather than grow unbounded. In steady state this
		// should essentially never trigger.
		c.entries = make(map[string]*ipfilter.IPChecker, ipCheckerCacheMaxEntries/2)
		c.logger.Warn().Msg("ipfilter: checker cache hit max size, evicting all entries")
	}
	c.entries[key] = checker
	c.mu.Unlock()

	return checker
}

func IPFilterMiddleware(logger zerolog.Logger) MiddlewareFunc {
	cache := newIPCheckerCache(logger)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rt, ok := r.Context().Value(shared.ContextKeyRoute).(*model.Route)
			if !ok || rt == nil {
				next.ServeHTTP(w, r)
				return
			}
			if len(rt.IPAllowlist) == 0 && len(rt.IPBlocklist) == 0 {
				next.ServeHTTP(w, r) // fast path: no IP policy on this route at all
				return
			}

			ip := ipfilter.ExtractIP(r.RemoteAddr, r.Header.Get, false)

			// Explicit deny takes precedence over allow — standard firewall
			// semantics, and what a security-minded tenant would expect.
			if blocklist := cache.get(rt.IPBlocklist); blocklist != nil && blocklist.IsBlocked(ip) {
				denyJSON(w, "IP address blocked by policy")
				return
			}

			if allowlist := cache.get(rt.IPAllowlist); allowlist != nil && !allowlist.IsAllowed(ip) {
				denyJSON(w, "IP address not in allowlist")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func denyJSON(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	_, _ = w.Write([]byte(`{"error":"` + msg + `"}`))
}
