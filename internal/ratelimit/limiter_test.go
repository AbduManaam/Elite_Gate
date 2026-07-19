package ratelimit

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestMemoryLimiter_BasicAndSliding(t *testing.T) {
	lim := NewMemoryLimiter(3)
	t0 := time.Now()
	
	// Override the clock function to control time precisely
	var mockTime = t0
	lim.now = func() time.Time {
		return mockTime
	}

	// 1. Basic limit enforcement (Limit = 3)
	res := lim.CheckAndConsume("client1", 3)
	if !res.Allowed || res.Remaining != 2 {
		t.Errorf("expected allowed=true, remaining=2, got allowed=%v, remaining=%d", res.Allowed, res.Remaining)
	}

	res = lim.CheckAndConsume("client1", 3)
	if !res.Allowed || res.Remaining != 1 {
		t.Errorf("expected allowed=true, remaining=1, got allowed=%v, remaining=%d", res.Allowed, res.Remaining)
	}

	res = lim.CheckAndConsume("client1", 3)
	if !res.Allowed || res.Remaining != 0 {
		t.Errorf("expected allowed=true, remaining=0, got allowed=%v, remaining=%d", res.Allowed, res.Remaining)
	}

	// 4th request at the same time must be blocked
	res = lim.CheckAndConsume("client1", 3)
	if res.Allowed {
		t.Errorf("expected 4th request to be rejected, but it was allowed")
	}

	// 2. Sliding window boundary check
	// Advance time by 59.9 seconds (still within the 60s sliding window)
	mockTime = t0.Add(59900 * time.Millisecond)
	res = lim.CheckAndConsume("client1", 3)
	if res.Allowed {
		t.Errorf("expected request at t+59.9s to be rejected")
	}

	// Advance time to 60.1 seconds (the first 3 requests at t0 drop out of the window)
	mockTime = t0.Add(60100 * time.Millisecond)
	res = lim.CheckAndConsume("client1", 3)
	if !res.Allowed || res.Remaining != 2 {
		t.Errorf("expected request at t+60.1s to be allowed, got allowed=%v, remaining=%d", res.Allowed, res.Remaining)
	}
}

func TestMemoryLimiter_Race(t *testing.T) {
	lim := NewMemoryLimiter(100)
	var wg sync.WaitGroup
	workers := 10
	reqsPerWorker := 10

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < reqsPerWorker; j++ {
				lim.CheckAndConsume("race-client", 100)
			}
		}()
	}
	wg.Wait()

	count := lim.Count("race-client")
	if count != workers*reqsPerWorker {
		t.Errorf("expected count %d, got %d", workers*reqsPerWorker, count)
	}
}

func TestMemoryLimiter_Cleanup(t *testing.T) {
	lim := NewMemoryLimiter(3)
	t0 := time.Now()
	var mockTime = t0
	lim.now = func() time.Time {
		return mockTime
	}

	lim.CheckAndConsume("client-temp", 3)
	if len(lim.requests) != 1 {
		t.Fatalf("expected map to contain 1 client key, got %d", len(lim.requests))
	}

	// Start cleanup manually in context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	// Move clock forward past sliding window (60s)
	mockTime = t0.Add(61 * time.Second)

	// Call cleanup directly or via ticker. We can just test the inner trim/delete logic by running StartCleanup
	// and trigger it using a short ticker.
	lim.StartCleanup(ctx, 10*time.Millisecond)
	time.Sleep(50 * time.Millisecond) // wait for cleanup run

	lim.mu.Lock()
	l := len(lim.requests)
	lim.mu.Unlock()

	if l != 0 {
		t.Errorf("expected idle key to be garbage collected, but it was still present")
	}
}

func TestRedisLimiter_BasicAndSliding(t *testing.T) {
	// Start miniredis
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer rdb.Close()

	fallback := NewMemoryLimiter(3)
	lim := NewRedisLimiter(rdb, 3, fallback)

	t0 := time.Now()
	var mockTime = t0
	lim.now = func() time.Time {
		return mockTime
	}

	// 1. Basic limit enforcement (Limit = 3)
	res := lim.CheckAndConsume("redis-client1", 3)
	if !res.Allowed || res.Remaining != 2 {
		t.Errorf("expected allowed=true, remaining=2, got allowed=%v, remaining=%d", res.Allowed, res.Remaining)
	}

	res = lim.CheckAndConsume("redis-client1", 3)
	if !res.Allowed || res.Remaining != 1 {
		t.Errorf("expected allowed=true, remaining=1, got allowed=%v, remaining=%d", res.Allowed, res.Remaining)
	}

	res = lim.CheckAndConsume("redis-client1", 3)
	if !res.Allowed || res.Remaining != 0 {
		t.Errorf("expected allowed=true, remaining=0, got allowed=%v, remaining=%d", res.Allowed, res.Remaining)
	}

	// 4th request blocked
	res = lim.CheckAndConsume("redis-client1", 3)
	if res.Allowed {
		t.Errorf("expected 4th request to be blocked")
	}

	// 2. Sliding window boundary check
	// Advance time by 59.9s (still within window)
	mockTime = t0.Add(59900 * time.Millisecond)
	res = lim.CheckAndConsume("redis-client1", 3)
	if res.Allowed {
		t.Errorf("expected request at t+59.9s to be rejected")
	}

	// Advance time by 60.1s (first 3 drop out)
	mockTime = t0.Add(60100 * time.Millisecond)
	res = lim.CheckAndConsume("redis-client1", 3)
	if !res.Allowed || res.Remaining != 2 {
		t.Errorf("expected request at t+60.1s to be allowed, got allowed=%v, remaining=%d", res.Allowed, res.Remaining)
	}
}
