package helper

import "os"

// PrefixedKey formats a Redis key using REDIS_PREFIX environment variable if present.
// For example, if REDIS_PREFIX="tenant:proj_123:", PrefixedKey("ratelimit:foo")
// returns "tenant:proj_123:ratelimit:foo".
func PrefixedKey(key string) string {
	prefix := os.Getenv("REDIS_PREFIX")
	if prefix != "" {
		return prefix + key
	}
	return key
}
