package helper

// OrEmpty returns s if non-nil, otherwise an empty slice.
// Prevents pq.Array() from storing NULL instead of '{}' in Postgres.
func OrEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// Contains reports whether target exists in list.
func Contains(list []string, target string) bool {
	for _, v := range list {
		if v == target {
			return true
		}
	}
	return false
}

// HasAllScopes reports whether clientScopes contains every scope in required.
func HasAllScopes(clientScopes, required []string) bool {
	have := make(map[string]struct{}, len(clientScopes))
	for _, s := range clientScopes {
		have[s] = struct{}{}
	}
	for _, s := range required {
		if _, ok := have[s]; !ok {
			return false
		}
	}
	return true
}

// SafePrefix returns the first n characters of s followed by "…".
// Useful for logging hashes without exposing the full value.
func SafePrefix(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
