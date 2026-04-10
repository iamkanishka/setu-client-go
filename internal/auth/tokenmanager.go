// Package auth manages Setu UPI/BBPS access-token lifecycle.
//
// Setu issues Bearer tokens valid for 300 seconds. This package:
//   - Caches the token and its expiry
//   - Proactively refreshes 60 seconds before expiry
//   - Deduplicates concurrent refresh calls (singleflight)
package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

const (
	tokenTTL      = 300 * time.Second
	refreshBuffer = 60 * time.Second
	loginPath     = "/v1/users/login"
)

// HTTPDoer is the minimal HTTP interface needed by [TokenManager].
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// TokenManagerConfig holds configuration for [TokenManager].
type TokenManagerConfig struct {
	ClientID     string
	ClientSecret string
	BaseURL      string
	HTTPClient   HTTPDoer
}

type cachedToken struct {
	value     string
	expiresAt time.Time
}

func (t *cachedToken) valid() bool {
	return t != nil && time.Now().Before(t.expiresAt.Add(-refreshBuffer))
}

// TokenManager is a thread-safe access-token cache with auto-refresh.
type TokenManager struct {
	cfg TokenManagerConfig

	mu     sync.RWMutex
	cached *cachedToken

	// inflight deduplicates concurrent refreshes.
	flightMu sync.Mutex
	flightCh chan struct{}
}

// NewTokenManager creates a [*TokenManager].
func NewTokenManager(cfg TokenManagerConfig) *TokenManager {
	return &TokenManager{cfg: cfg}
}

// Token returns a valid access token, refreshing transparently if needed.
func (m *TokenManager) Token(ctx context.Context) (string, error) {
	m.mu.RLock()
	if m.cached.valid() {
		tok := m.cached.value
		m.mu.RUnlock()
		return tok, nil
	}
	m.mu.RUnlock()
	return m.refresh(ctx)
}

// Invalidate clears the cached token, forcing a fresh fetch on next call.
// Call this when the API returns HTTP 401.
func (m *TokenManager) Invalidate() {
	m.mu.Lock()
	m.cached = nil
	m.mu.Unlock()
}

func (m *TokenManager) refresh(ctx context.Context) (string, error) {
	m.flightMu.Lock()
	if m.flightCh != nil {
		ch := m.flightCh
		m.flightMu.Unlock()
		select {
		case <-ch:
		case <-ctx.Done():
			return "", ctx.Err()
		}
		m.mu.RLock()
		defer m.mu.RUnlock()
		if m.cached.valid() {
			return m.cached.value, nil
		}
		return "", fmt.Errorf("auth: token refresh failed")
	}
	ch := make(chan struct{})
	m.flightCh = ch
	m.flightMu.Unlock()

	tok, err := m.fetchToken(ctx)

	m.flightMu.Lock()
	m.flightCh = nil
	m.flightMu.Unlock()
	close(ch)

	return tok, err
}

type loginReq struct {
	ClientID  string `json:"clientID"`
	Secret    string `json:"secret"`
	GrantType string `json:"grant_type"`
}

type loginResp struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

func (m *TokenManager) fetchToken(ctx context.Context) (string, error) {
	body, err := json.Marshal(loginReq{
		ClientID:  m.cfg.ClientID,
		Secret:    m.cfg.ClientSecret,
		GrantType: "client_credentials",
	})
	if err != nil {
		return "", fmt.Errorf("auth: marshal login request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		m.cfg.BaseURL+loginPath, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("auth: build login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("client", "bridge")

	resp, err := m.cfg.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("auth: login http request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("auth: read login response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("auth: login returned status %d: %s", resp.StatusCode, string(raw))
	}

	var lr loginResp
	if err = json.Unmarshal(raw, &lr); err != nil {
		return "", fmt.Errorf("auth: decode login response: %w", err)
	}
	if lr.AccessToken == "" {
		return "", fmt.Errorf("auth: empty access_token in login response")
	}

	ttl := tokenTTL
	if lr.ExpiresIn > 0 {
		ttl = time.Duration(lr.ExpiresIn) * time.Second
	}

	m.mu.Lock()
	m.cached = &cachedToken{
		value:     lr.AccessToken,
		expiresAt: time.Now().Add(ttl),
	}
	m.mu.Unlock()

	return lr.AccessToken, nil
}
