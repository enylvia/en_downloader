package engine

import (
	"context"
	"sync"
	"time"
)

// RateLimiter provides a thread-safe token-bucket rate limiter that coordinates
// bandwidth throttling across all concurrent chunks and download streams.
type RateLimiter struct {
	mu         sync.Mutex
	limitBytes int64   // bytes per second; 0 = unlimited
	tokens     float64 // available byte tokens
	lastCheck  time.Time
}

func NewRateLimiter(limitKB int) *RateLimiter {
	limitBytes := int64(limitKB) * 1024
	return &RateLimiter{
		limitBytes: limitBytes,
		tokens:     float64(limitBytes),
		lastCheck:  time.Now(),
	}
}

func (rl *RateLimiter) SetLimit(limitKB int) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.limitBytes = int64(limitKB) * 1024
	if rl.limitBytes <= 0 {
		rl.tokens = 0
	} else {
		// Reset tokens to 1 burst limit so change takes effect cleanly
		rl.tokens = float64(rl.limitBytes)
	}
	rl.lastCheck = time.Now()
}

func (rl *RateLimiter) LimitKB() int {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	return int(rl.limitBytes / 1024)
}

func (rl *RateLimiter) Wait(ctx context.Context, n int) {
	if n <= 0 {
		return
	}

	rl.mu.Lock()
	limit := rl.limitBytes
	if limit <= 0 {
		rl.mu.Unlock()
		return
	}

	now := time.Now()
	elapsed := now.Sub(rl.lastCheck).Seconds()
	rl.lastCheck = now

	// Refill tokens based on time elapsed
	rl.tokens += elapsed * float64(limit)
	maxBurst := float64(limit)
	if rl.tokens > maxBurst {
		rl.tokens = maxBurst
	}

	// Consume n tokens
	rl.tokens -= float64(n)

	if rl.tokens >= 0 {
		rl.mu.Unlock()
		return
	}

	// Deficit: sleep for the time needed to pay off the consumed tokens
	deficit := -rl.tokens
	sleepSeconds := deficit / float64(limit)
	rl.mu.Unlock()

	sleepDuration := time.Duration(sleepSeconds * float64(time.Second))
	if sleepDuration > 500*time.Millisecond {
		sleepDuration = 500 * time.Millisecond
	}

	select {
	case <-ctx.Done():
		return
	case <-time.After(sleepDuration):
	}
}
