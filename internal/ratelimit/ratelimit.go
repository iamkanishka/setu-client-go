// Package ratelimit provides a client-side token-bucket rate limiter
// backed by golang.org/x/time/rate.
package ratelimit

import (
	"context"
	"time"

	"golang.org/x/time/rate"
)

// Limiter wraps a token-bucket rate limiter with wait-time reporting.
type Limiter struct {
	rl *rate.Limiter
}

// New creates a [*Limiter] that allows rps events per second with the given burst.
func New(rps float64, burst int) *Limiter {
	return &Limiter{rl: rate.NewLimiter(rate.Limit(rps), burst)}
}

// Wait blocks until a token is available or ctx is cancelled.
// Returns the time waited and any context error.
func (l *Limiter) Wait(ctx context.Context) (time.Duration, error) {
	start := time.Now()
	if err := l.rl.Wait(ctx); err != nil {
		return 0, err
	}
	return time.Since(start), nil
}
