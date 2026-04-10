// Package transport provides the shared HTTP client for all Setu SDK
// sub-clients. It applies rate-limiting, retry with exponential backoff,
// request-body replay, and structured error decoding.
package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/iamkanishka/setu-client-go/internal/ratelimit"
	"github.com/iamkanishka/setu-client-go/internal/retry"
	"github.com/iamkanishka/setu-client-go/pkg/setuerrors"
)

const defaultUserAgent = "setu-go/1.0.0"

// Config holds configuration for a [Client].
type Config struct {
	// HTTPClient overrides the default production-tuned transport.
	HTTPClient *http.Client
	// RateLimiter is applied before each attempt.
	RateLimiter *ratelimit.Limiter
	// RetryPolicy governs retry behaviour.
	RetryPolicy *retry.Policy
	// UserAgent is used in the User-Agent request header.
	UserAgent string
}

// Client is the shared HTTP transport layer.
type Client struct {
	hc    *http.Client
	rl    *ratelimit.Limiter
	retry *retry.Policy
	ua    string
}

// New builds a [*Client] from cfg.
func New(cfg Config) *Client {
	hc := cfg.HTTPClient
	if hc == nil {
		hc = defaultHTTPClient()
	}
	ua := cfg.UserAgent
	if ua == "" {
		ua = defaultUserAgent
	}
	return &Client{hc: hc, rl: cfg.RateLimiter, retry: cfg.RetryPolicy, ua: ua}
}

func defaultHTTPClient() *http.Client {
	t, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return &http.Client{Timeout: 30 * time.Second}
	}
	c := t.Clone()
	c.MaxIdleConns = 100
	c.MaxIdleConnsPerHost = 20
	c.IdleConnTimeout = 90 * time.Second
	c.ForceAttemptHTTP2 = true
	return &http.Client{Transport: c, Timeout: 30 * time.Second}
}

// NewJSONRequest constructs an *http.Request with a JSON body and canonical headers.
func NewJSONRequest(ctx context.Context, method, url string, body any) (*http.Request, error) {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("transport: marshal body: %w", err)
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, r)
	if err != nil {
		return nil, fmt.Errorf("transport: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	return req, nil
}

// Do executes req with rate-limiting and retry. The caller owns the body.
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	req.Header.Set("User-Agent", c.ua)

	var bodyBuf []byte
	if req.Body != nil && req.Body != http.NoBody {
		var err error
		bodyBuf, err = io.ReadAll(req.Body)
		if err != nil {
			return nil, setuerrors.NewNetworkError("read request body", err)
		}
		req.Body.Close() //nolint:errcheck
	}

	maxAttempts := 1
	if c.retry != nil {
		maxAttempts = c.retry.MaxAttempts()
	}

	var (
		lastResp *http.Response
		lastErr  error
	)

	for attempt := 0; attempt < maxAttempts; attempt++ {
		if c.rl != nil {
			if _, err := c.rl.Wait(req.Context()); err != nil {
				return nil, err
			}
		}
		if bodyBuf != nil {
			req.Body = io.NopCloser(bytes.NewReader(bodyBuf))
			req.ContentLength = int64(len(bodyBuf))
		}
		lastResp, lastErr = c.hc.Do(req)
		if c.retry == nil || !c.retry.ShouldRetry(attempt, lastResp, lastErr) {
			break
		}
		if lastResp != nil {
			_, _ = io.Copy(io.Discard, lastResp.Body)
			lastResp.Body.Close() //nolint:errcheck
		}
		if waitErr := c.retry.Wait(req.Context(), attempt); waitErr != nil {
			return nil, waitErr
		}
	}

	if lastErr != nil {
		return nil, setuerrors.NewNetworkError("http do", lastErr)
	}
	return lastResp, nil
}

// DoJSON executes req and JSON-decodes a 2xx response into dst.
// Non-2xx responses are mapped to typed SDK errors.
// dst may be nil when no response body is needed.
func (c *Client) DoJSON(req *http.Request, dst any) error {
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close() //nolint:errcheck

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return setuerrors.NewNetworkError("read response body", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return decodeError(resp, raw)
	}
	if dst == nil || len(raw) == 0 {
		return nil
	}
	if err = json.Unmarshal(raw, dst); err != nil {
		return fmt.Errorf("transport: decode response: %w (body: %.200s)", err, string(raw))
	}
	return nil
}

// apiErrorEnvelope matches common Setu error response shapes.
type apiErrorEnvelope struct {
	Code    string `json:"code"`
	Error   string `json:"error"`
	Message string `json:"message"`
	TraceID string `json:"traceId"`
}

func decodeError(resp *http.Response, body []byte) error {
	traceID := resp.Header.Get("X-Trace-Id")
	retryAfter := resp.Header.Get("Retry-After")

	var env apiErrorEnvelope
	_ = json.Unmarshal(body, &env)

	if env.TraceID != "" {
		traceID = env.TraceID
	}
	code := env.Code
	if code == "" {
		code = env.Error
	}
	if code == "" {
		code = strconv.Itoa(resp.StatusCode)
	}
	msg := env.Message
	if msg == "" {
		msg = string(body)
	}

	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return setuerrors.NewAuthError(resp.StatusCode, msg, traceID)
	case http.StatusTooManyRequests:
		return setuerrors.NewRateLimitError(traceID, retryAfter)
	default:
		return setuerrors.NewAPIError(resp.StatusCode, code, msg, traceID)
	}
}
