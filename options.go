package email

import (
	"crypto/rand"
	"math"
	mrand "math/rand"
	"time"

	"github.com/aatuh/email/types"
)

// Option configures per-send behavior.
type Option func(*SendConfig)

// SendConfig is applied during Send.
type SendConfig struct {
	ListUnsub string
	Backoff   Backoff
	Rate      *TokenBucket
	Pool      *ConnPool
	Hooks     *types.Hooks
	DKIM      *types.DKIMConfig
}

// WithListUnsubscribe sets the List-Unsubscribe header.
//
// Parameters:
//   - v: The List-Unsubscribe header value.
//
// Returns:
//   - Option: The option.
func WithListUnsubscribe(v string) Option {
	return func(c *SendConfig) { c.ListUnsub = v }
}

// WithRetry configures a retry backoff. Nil disables retries.
//
// Parameters:
//   - b: The retry backoff.
//
// Returns:
//   - Option: The option.
func WithRetry(b Backoff) Option {
	return func(c *SendConfig) { c.Backoff = b }
}

// WithRateLimit attaches a token bucket for throttling.
//
// Parameters:
//   - bucket: The token bucket.
//
// Returns:
//   - Option: The option.
func WithRateLimit(bucket *TokenBucket) Option {
	return func(c *SendConfig) { c.Rate = bucket }
}

// WithPool sets a connection pool to reuse adapter connections.
//
// Parameters:
//   - pool: The connection pool.
//
// Returns:
//   - Option: The option.
func WithPool(pool *ConnPool) Option {
	return func(c *SendConfig) { c.Pool = pool }
}

// WithHooks attaches observability hooks (OTel-friendly, no deps).
//
// Parameters:
//   - h: The hooks.
//
// Returns:
//   - Option: The option.
func WithHooks(h *types.Hooks) Option {
	return func(c *SendConfig) { c.Hooks = h }
}

// WithDKIM enables DKIM signing using the provided config.
//
// Parameters:
//   - cfg: The DKIM config.
//
// Returns:
//   - Option: The option.
func WithDKIM(cfg types.DKIMConfig) Option {
	return func(c *SendConfig) { c.DKIM = &cfg }
}

// Backoff describes retry sleep schedule.
type Backoff interface {
	// Next returns sleep before attempt i (0-based). ok=false when no more.
	Next(i int) (d time.Duration, ok bool)
}

// ExponentialBackoff returns a Backoff with jitter.
// attempts is total tries (>=1). base is initial delay, max caps delay.
// fullJitter picks [0,d) vs half-jitter [d/2, d).
//
// Parameters:
//   - attempts: The total tries.
//   - base: The initial delay.
//   - max: The max delay.
//   - fullJitter: The full jitter.
//
// Returns:
//   - Backoff: The backoff.
func ExponentialBackoff(
	attempts int,
	base time.Duration,
	max time.Duration,
	fullJitter bool,
) Backoff {
	if attempts <= 0 {
		attempts = 1
	}
	var seed int64
	_ = binaryReadRand(&seed)
	src := mrand.NewSource(seed ^ time.Now().UnixNano())
	r := mrand.New(src)

	return &expBackoff{
		attempts:   attempts,
		base:       base,
		max:        max,
		fullJitter: fullJitter,
		r:          r,
	}
}

// expBackoff is an exponential backoff with jitter.
type expBackoff struct {
	attempts   int
	base       time.Duration
	max        time.Duration
	fullJitter bool
	r          *mrand.Rand
}

// Next returns sleep before attempt i (0-based). ok=false when no more.
func (b *expBackoff) Next(i int) (time.Duration, bool) {
	if i >= b.attempts {
		return 0, false
	}
	if i == 0 {
		return 0, true
	}
	pow := math.Pow(2, float64(i-1))
	d := time.Duration(float64(b.base) * pow)
	if b.max > 0 && d > b.max {
		d = b.max
	}
	if b.fullJitter {
		return time.Duration(b.r.Int63n(int64(d + 1))), true
	}
	half := d / 2
	j := time.Duration(b.r.Int63n(int64(half + 1)))
	return half + j, true
}

// binaryReadRand reads a random number from the binary.
func binaryReadRand(out *int64) error {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return err
	}
	var v int64
	for i := 0; i < 8; i++ {
		v = (v << 8) | int64(buf[i])
	}
	*out = v
	return nil
}
