// Package setuerrors defines all structured error types for the Setu SDK.
// Every error returned by SDK methods implements the [Error] interface and
// is fully compatible with [errors.As] / [errors.Is].
package setuerrors

import (
	"errors"
	"fmt"
	"net/http"
)

// Error is the common interface every SDK error implements.
type Error interface {
	error
	HTTPStatus() int // 0 for non-HTTP errors
	Code() string    // machine-readable Setu error code
	TraceID() string // empty when unavailable
	Retryable() bool // true means the caller may retry
}

// ── APIError ──────────────────────────────────────────────────────────────

// APIError represents a structured HTTP error response from the Setu API.
type APIError struct {
	httpStatus int
	code       string
	message    string
	traceID    string
}

// NewAPIError constructs an [*APIError].
func NewAPIError(httpStatus int, code, message, traceID string) *APIError {
	return &APIError{httpStatus: httpStatus, code: code, message: message, traceID: traceID}
}

func (e *APIError) Error() string {
	return fmt.Sprintf("setu: api error status=%d code=%q message=%q traceId=%q",
		e.httpStatus, e.code, e.message, e.traceID)
}

func (e *APIError) HTTPStatus() int { return e.httpStatus }
func (e *APIError) Code() string    { return e.code }
func (e *APIError) TraceID() string { return e.traceID }
func (e *APIError) Retryable() bool {
	switch e.httpStatus {
	case http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	}
	return false
}

// ── AuthError ─────────────────────────────────────────────────────────────

// AuthError represents a 401/403 authentication or authorisation failure.
type AuthError struct {
	httpStatus int
	message    string
	traceID    string
}

// NewAuthError constructs an [*AuthError].
func NewAuthError(httpStatus int, message, traceID string) *AuthError {
	return &AuthError{httpStatus: httpStatus, message: message, traceID: traceID}
}

func (e *AuthError) Error() string {
	return fmt.Sprintf("setu: auth error status=%d message=%q traceId=%q",
		e.httpStatus, e.message, e.traceID)
}

func (e *AuthError) HTTPStatus() int { return e.httpStatus }
func (e *AuthError) Code() string    { return "AUTH_ERROR" }
func (e *AuthError) TraceID() string { return e.traceID }
func (e *AuthError) Retryable() bool { return false }

// ── RateLimitError ────────────────────────────────────────────────────────

// RateLimitError is returned for HTTP 429 or when the client-side limiter
// is exhausted.
type RateLimitError struct {
	traceID    string
	retryAfter string
}

// NewRateLimitError constructs a [*RateLimitError].
func NewRateLimitError(traceID, retryAfter string) *RateLimitError {
	return &RateLimitError{traceID: traceID, retryAfter: retryAfter}
}

func (e *RateLimitError) Error() string {
	if e.retryAfter != "" {
		return fmt.Sprintf("setu: rate limit exceeded, retry after %s", e.retryAfter)
	}
	return "setu: rate limit exceeded"
}

func (e *RateLimitError) HTTPStatus() int    { return http.StatusTooManyRequests }
func (e *RateLimitError) Code() string       { return "RATE_LIMIT_EXCEEDED" }
func (e *RateLimitError) TraceID() string    { return e.traceID }
func (e *RateLimitError) Retryable() bool    { return true }
func (e *RateLimitError) RetryAfter() string { return e.retryAfter }

// ── NetworkError ──────────────────────────────────────────────────────────

// NetworkError wraps a transport-level failure (DNS, TCP, TLS, timeout).
type NetworkError struct {
	message string
	cause   error
}

// NewNetworkError constructs a [*NetworkError].
func NewNetworkError(message string, cause error) *NetworkError {
	return &NetworkError{message: message, cause: cause}
}

func (e *NetworkError) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("setu: network error: %s: %v", e.message, e.cause)
	}
	return fmt.Sprintf("setu: network error: %s", e.message)
}

func (e *NetworkError) Unwrap() error   { return e.cause }
func (e *NetworkError) HTTPStatus() int { return 0 }
func (e *NetworkError) Code() string    { return "NETWORK_ERROR" }
func (e *NetworkError) TraceID() string { return "" }
func (e *NetworkError) Retryable() bool { return true }

// ── ValidationError ───────────────────────────────────────────────────────

// ValidationError is returned when client-side validation fails before
// any HTTP request is made.
type ValidationError struct {
	Field   string
	Message string
}

// NewValidationError constructs a [*ValidationError].
func NewValidationError(field, message string) *ValidationError {
	return &ValidationError{Field: field, Message: message}
}

func (e *ValidationError) Error() string {
	if e.Field != "" {
		return fmt.Sprintf("setu: validation error field=%q: %s", e.Field, e.Message)
	}
	return fmt.Sprintf("setu: validation error: %s", e.Message)
}

func (e *ValidationError) HTTPStatus() int { return http.StatusBadRequest }
func (e *ValidationError) Code() string    { return "VALIDATION_ERROR" }
func (e *ValidationError) TraceID() string { return "" }
func (e *ValidationError) Retryable() bool { return false }

// ── Predicates ────────────────────────────────────────────────────────────

// IsNotFound reports whether err represents an HTTP 404.
func IsNotFound(err error) bool {
	var ae *APIError
	return errors.As(err, &ae) && ae.httpStatus == http.StatusNotFound
}

// IsUnauthorized reports whether err is an authentication error.
func IsUnauthorized(err error) bool {
	var ae *AuthError
	return errors.As(err, &ae)
}

// IsRateLimit reports whether err is a rate-limit error.
func IsRateLimit(err error) bool {
	var re *RateLimitError
	return errors.As(err, &re)
}

// IsRetryable reports whether err indicates a retryable condition.
func IsRetryable(err error) bool {
	var se Error
	return errors.As(err, &se) && se.Retryable()
}

// GetTraceID extracts the Setu traceID from any SDK error.
// Returns "" when no traceID is available.
func GetTraceID(err error) string {
	var se Error
	if errors.As(err, &se) {
		return se.TraceID()
	}
	return ""
}
