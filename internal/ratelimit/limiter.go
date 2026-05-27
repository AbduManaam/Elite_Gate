package ratelimit

type Limiter interface {
	Allow(key string) bool
	Count(key string) int
	Limit() int
}