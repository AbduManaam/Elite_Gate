package ratelimit

type Limiter interface {
	Allow(key string) bool
	AllowWithLimit(key string, limit int) bool
	Count(key string) int
	Limit() int
}