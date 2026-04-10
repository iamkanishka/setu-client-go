// Package pan provides the Setu PAN Verification API client.
// Setu docs: https://docs.setu.co/data/pan
package pan

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/iamkanishka/setu-client-go/internal/transport"
	"github.com/iamkanishka/setu-client-go/pkg/setuerrors"
)

// Client is the PAN Verification API client.
type Client struct {
	tc      *transport.Client
	baseURL string
	headers map[string]string
}

// New creates a [*Client].
func New(tc *transport.Client, baseURL string, headers map[string]string) *Client {
	return &Client{tc: tc, baseURL: baseURL, headers: headers}
}

// VerifyRequest is the input for [Client.Verify].
type VerifyRequest struct {
	// PAN is the 10-character Permanent Account Number.
	PAN string `json:"pan"`
	// Consent must be "Y".
	Consent string `json:"consent"`
	// Reason must be at least 20 characters.
	Reason string `json:"reason"`
}

// VerifyResponse is returned by [Client.Verify].
type VerifyResponse struct {
	Data         VerifyData `json:"data"`
	Message      string     `json:"message"`
	Verification string     `json:"verification"`
	TraceID      string     `json:"traceId"`
}

// VerifyData holds the PAN holder details.
type VerifyData struct {
	FullName             string `json:"full_name"`
	FirstName            string `json:"first_name,omitempty"`
	MiddleName           string `json:"middle_name,omitempty"`
	LastName             string `json:"last_name,omitempty"`
	Category             string `json:"category"`
	AadhaarSeedingStatus string `json:"aadhaar_seeding_status,omitempty"`
}

// IsValid returns true when the PAN is verified and active.
func (r *VerifyResponse) IsValid() bool {
	return strings.EqualFold(r.Verification, "success") &&
		strings.EqualFold(r.Message, "PAN is valid")
}

// Sandbox test values:
//
//   - ABCDE1234A → valid PAN
//
//   - ABCDE1234B → invalid/blacklisted PAN
//
//   - Any other  → 404 PAN not found
//
//     POST /api/verify/pan
func (c *Client) Verify(ctx context.Context, req *VerifyRequest) (*VerifyResponse, error) {
	if req == nil {
		return nil, setuerrors.NewValidationError("", "request is required")
	}
	if len(req.PAN) != 10 {
		return nil, setuerrors.NewValidationError("pan", "PAN must be exactly 10 characters")
	}
	if !strings.EqualFold(req.Consent, "Y") {
		return nil, setuerrors.NewValidationError("consent", `consent must be "Y"`)
	}
	if len(req.Reason) < 20 {
		return nil, setuerrors.NewValidationError("reason", "reason must be at least 20 characters")
	}
	httpReq, err := transport.NewJSONRequest(ctx, http.MethodPost, c.baseURL+"/api/verify/pan", req)
	if err != nil {
		return nil, fmt.Errorf("pan: build request: %w", err)
	}
	for k, v := range c.headers {
		httpReq.Header.Set(k, v)
	}
	var out VerifyResponse
	return &out, c.tc.DoJSON(httpReq, &out)
}
