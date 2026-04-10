// Package retry implements exponential backoff with full jitter.
package retry

import (
	"context"
	"math"
	"math/rand/v2"
	"net/http"
	"time"
)

// ShouldRetryFunc decides whether a given response/error warrants a retry.
type ShouldRetryFunc func(resp *http.Response, err error) bool

// DefaultShouldRetry retries on network errors, 429, and 5xx responses.
var DefaultShouldRetry ShouldRetryFunc = func(resp *http.Response, err error) bool {
	if err != nil {
		return true
	}
	if resp == nil {
		return false
	}
	switch resp.StatusCode {
	case http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	}
	return false
}

// Config holds retry policy parameters.
type Config struct {
	// MaxAttempts is the total number of attempts (1 = no retries).
	MaxAttempts int
	// WaitBase is the initial backoff duration before jitter.
	WaitBase time.Duration
	// WaitMax caps the backoff duration.
	WaitMax time.Duration
	// ShouldRetry decides whether to retry. Defaults to DefaultShouldRetry.
	ShouldRetry ShouldRetryFunc
}

// Policy is a resolved, immutable retry policy.
type Policy struct {
	cfg Config
}

// New creates a [*Policy] from cfg.
func New(cfg Config) *Policy {
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 1
	}
	if cfg.ShouldRetry == nil {
		cfg.ShouldRetry = DefaultShouldRetry
	}
	return &Policy{cfg: cfg}
}

// MaxAttempts returns the configured maximum attempt count.
func (p *Policy) MaxAttempts() int { return p.cfg.MaxAttempts }

// ShouldRetry returns true when another attempt should be made.
func (p *Policy) ShouldRetry(attempt int, resp *http.Response, err error) bool {
	if attempt >= p.cfg.MaxAttempts-1 {
		return false
	}
	return p.cfg.ShouldRetry(resp, err)
}

// Wait blocks for the jittered backoff duration before the next attempt.
// Duration = random value in [0, min(WaitMax, WaitBase * 2^attempt)].
// Returns ctx.Err() if the context is cancelled while waiting.
func (p *Policy) Wait(ctx context.Context, attempt int) error {
	cap := float64(p.cfg.WaitMax)
	base := float64(p.cfg.WaitBase) * math.Pow(2, float64(attempt))
	if base > cap {
		base = cap
	}
	sleep := time.Duration(rand.Float64() * base) //nolint:gosec // PRNG jitter is intentional
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(sleep):
		return nil
	}
}
