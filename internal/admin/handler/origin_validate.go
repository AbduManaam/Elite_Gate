package handler

import (
	"fmt"
	"net/url"
	"strings"
)

// validateOrigin rejects anything that isn't a clean scheme+host origin —
// no path, no query, no trailing slash, no wildcard, https required outside localhost.
func validateOrigin(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid origin %q", raw)
	}
	hostname := u.Hostname()
	isLocalhost := hostname == "localhost" || strings.HasSuffix(hostname, ".localhost")
	if u.Scheme != "https" && !(u.Scheme == "http" && isLocalhost) {
		return fmt.Errorf("origin %q must use https (http only allowed for localhost)", raw)
	}
	if u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("origin %q must not include a path, query, or fragment", raw)
	}
	if raw == "*" || strings.Contains(raw, "*") {
		return fmt.Errorf("wildcard origins are not allowed")
	}
	return nil
}
