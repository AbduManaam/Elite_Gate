package ratelimit

import "time"

// RateResult is returned by CheckAndConsume — a single atomic check that
// combines the old two-call Count()+AllowWithLimit() pattern into one
// round trip (one Redis call, or one mutex-protected pass in memory).
type RateResult struct {
	Allowed   bool
	Remaining int
	ResetAt   time.Time
}

type Limiter interface {
	Allow(key string) bool
	AllowWithLimit(key string, limit int) bool
	Count(key string) int
	Limit() int

	// CheckAndConsume is the preferred entry point for HTTP middleware —
	// it evaluates and (if allowed) records the request in a single call.
	CheckAndConsume(key string, limit int) RateResult
}

