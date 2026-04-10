// Package bav provides the Setu Bank Account Verification (BAV / penny-drop) client.
// Setu docs: https://docs.setu.co/data/bav
package bav

import (
	"context"
	"fmt"
	"net/http"

	"github.com/iamkanishka/setu-client-go/internal/transport"
	"github.com/iamkanishka/setu-client-go/pkg/setuerrors"
)

// Client is the BAV API client.
type Client struct {
	tc      *transport.Client
	baseURL string
	headers map[string]string
}

// New creates a [*Client].
func New(tc *transport.Client, baseURL string, headers map[string]string) *Client {
	return &Client{tc: tc, baseURL: baseURL, headers: headers}
}

// VerifySyncRequest is the input for [Client.VerifySync].
type VerifySyncRequest struct {
	AccountNumber string `json:"accountNumber"`
	IFSC          string `json:"ifsc"`
	Name          string `json:"name,omitempty"`
}

// VerifyAsyncRequest is the input for [Client.VerifyAsync].
type VerifyAsyncRequest struct {
	AccountNumber string `json:"accountNumber"`
	IFSC          string `json:"ifsc"`
	Name          string `json:"name,omitempty"`
}

// VerifyResponse is returned by [Client.VerifySync] and [Client.GetAsyncStatus].
type VerifyResponse struct {
	AccountExists bool   `json:"accountExists"`
	NameAtBank    string `json:"nameAtBank,omitempty"`
	AccountNumber string `json:"accountNumber"`
	IFSC          string `json:"ifsc"`
	Status        string `json:"status"`
	TraceID       string `json:"traceId"`
}

// AsyncVerifyResponse is returned by [Client.VerifyAsync].
type AsyncVerifyResponse struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	TraceID string `json:"traceId"`
}

// VerifySync performs a synchronous penny-drop verification.
// Blocks until ₹1 is credited and result is available.
//
//	POST /api/verify/ban/sync
func (c *Client) VerifySync(ctx context.Context, req *VerifySyncRequest) (*VerifyResponse, error) {
	if err := validateAccount(req.AccountNumber, req.IFSC, req == nil); err != nil {
		return nil, err
	}
	httpReq, err := transport.NewJSONRequest(ctx, http.MethodPost, c.baseURL+"/api/verify/ban/sync", req)
	if err != nil {
		return nil, fmt.Errorf("bav: build sync request: %w", err)
	}
	c.apply(httpReq)
	var out VerifyResponse
	return &out, c.tc.DoJSON(httpReq, &out)
}

// VerifyAsync initiates an asynchronous penny-drop.
// Use the returned ID with [Client.GetAsyncStatus] or a webhook for the result.
//
//	POST /api/verify/ban/async
func (c *Client) VerifyAsync(ctx context.Context, req *VerifyAsyncRequest) (*AsyncVerifyResponse, error) {
	if err := validateAccount(req.AccountNumber, req.IFSC, req == nil); err != nil {
		return nil, err
	}
	httpReq, err := transport.NewJSONRequest(ctx, http.MethodPost, c.baseURL+"/api/verify/ban/async", req)
	if err != nil {
		return nil, fmt.Errorf("bav: build async request: %w", err)
	}
	c.apply(httpReq)
	var out AsyncVerifyResponse
	return &out, c.tc.DoJSON(httpReq, &out)
}

// GetAsyncStatus retrieves the result of an async BAV request.
//
//	GET /api/verify/ban/async/:id
func (c *Client) GetAsyncStatus(ctx context.Context, id string) (*VerifyResponse, error) {
	if id == "" {
		return nil, setuerrors.NewValidationError("id", "async verification ID is required")
	}
	httpReq, err := transport.NewJSONRequest(ctx, http.MethodGet,
		fmt.Sprintf("%s/api/verify/ban/async/%s", c.baseURL, id), nil)
	if err != nil {
		return nil, fmt.Errorf("bav: build status request: %w", err)
	}
	c.apply(httpReq)
	var out VerifyResponse
	return &out, c.tc.DoJSON(httpReq, &out)
}

func (c *Client) apply(req *http.Request) {
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}
}

func validateAccount(accountNumber, ifsc string, isNil bool) error {
	if isNil {
		return setuerrors.NewValidationError("", "request is required")
	}
	if accountNumber == "" {
		return setuerrors.NewValidationError("accountNumber", "accountNumber is required")
	}
	if ifsc == "" {
		return setuerrors.NewValidationError("ifsc", "IFSC is required")
	}
	return nil
}
