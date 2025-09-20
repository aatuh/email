package email

import (
	"sync"
	"time"
)

// TokenBucket is a simple thread-safe token bucket.
type TokenBucket struct {
	rate   float64 // tokens per second
	burst  int     // max tokens
	mu     sync.Mutex
	tokens float64
	last   time.Time
}

// NewTokenBucket returns a token bucket generating "rate" tokens per
// second with a capacity of "burst".
//
// Parameters:
//   - rate: The rate of tokens per second.
//   - burst: The max tokens.
//
// Returns:
//   - *TokenBucket: The token bucket.
func NewTokenBucket(rate float64, burst int) *TokenBucket {
	if rate <= 0 {
		rate = 1
	}
	if burst <= 0 {
		burst = 1
	}
	return &TokenBucket{
		rate:   rate,
		burst:  burst,
		tokens: float64(burst),
		last:   time.Now(),
	}
}

// Wait blocks until one token is available.
//
// Parameters:
//   - tb: The token bucket.
//
// Returns:
//   - void: The token bucket is blocked until one token is available.
func (tb *TokenBucket) Wait() {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	for {
		now := time.Now()
		dt := now.Sub(tb.last).Seconds()
		tb.last = now

		tb.tokens += dt * tb.rate
		if tb.tokens > float64(tb.burst) {
			tb.tokens = float64(tb.burst)
		}
		if tb.tokens >= 1 {
			tb.tokens -= 1
			return
		}
		need := 1 - tb.tokens
		sleep := time.Duration(need/tb.rate*1000) * time.Millisecond
		if sleep < time.Millisecond {
			sleep = time.Millisecond
		}
		time.Sleep(sleep)
	}
}
