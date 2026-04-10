package setu

import (
	"net/http"
	"time"

	"github.com/iamkanishka/setu-client-go/pkg/types"
)

// Option is a functional option for configuring a [Client].
type Option func(*config)

// WithClientID sets the Setu API client ID. Required.
func WithClientID(id string) Option {
	return func(c *config) { c.clientID = id }
}

// WithClientSecret sets the Setu API client secret. Required.
func WithClientSecret(secret string) Option {
	return func(c *config) { c.clientSecret = secret }
}

// WithProductInstanceID sets the x-product-instance-id header used by
// KYC and Account Aggregator APIs.
func WithProductInstanceID(id string) Option {
	return func(c *config) { c.productInstanceID = id }
}

// WithEnvironment selects [types.Sandbox] or [types.Production].
// Defaults to [types.Sandbox].
func WithEnvironment(env types.Environment) Option {
	return func(c *config) { c.environment = env }
}

// WithTimeout sets the per-request HTTP timeout. Default: 30s.
func WithTimeout(d time.Duration) Option {
	return func(c *config) { c.timeout = d }
}

// WithMaxAttempts sets the total attempt count (1 = no retries). Default: 4.
func WithMaxAttempts(n int) Option {
	return func(c *config) { c.maxAttempts = n }
}

// WithRetryWait sets the base and maximum backoff duration. Default: 500ms / 10s.
func WithRetryWait(base, max time.Duration) Option {
	return func(c *config) {
		c.retryWaitBase = base
		c.retryWaitMax = max
	}
}

// WithRateLimit configures the client-side token-bucket rate limiter.
// rps = sustained requests/second; burst = maximum burst capacity.
// Default: 100 RPS, burst 20.
func WithRateLimit(rps float64, burst int) Option {
	return func(c *config) {
		c.rateLimitRPS = rps
		c.rateLimitBurst = burst
	}
}

// WithHTTPTransport replaces the default HTTP transport.
// Useful for custom TLS, proxies, or test mocks.
func WithHTTPTransport(t http.RoundTripper) Option {
	return func(c *config) { c.httpTransport = t }
}

// WithUserAgent overrides the SDK User-Agent header.
func WithUserAgent(ua string) Option {
	return func(c *config) { c.userAgent = ua }
}
