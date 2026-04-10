// Package esign provides the Setu Aadhaar eSign API client.
//
// eSign enables legally enforceable Aadhaar-based electronic signatures on
// PDF documents, supporting up to 6 signers per document.
// Built on NSDL and eMudra infrastructure, fully privacy-compliant.
//
// Flow:
//  1. [Client.Create] — upload document as base64, specify signers, receive signing URL.
//  2. Redirect each signer to their signing URL.
//  3. Poll [Client.Get] or receive webhook when all signers have signed.
//  4. Download the signed PDF via [Client.Download].
//
// Setu docs: https://docs.setu.co/data/esign
package esign

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/iamkanishka/setu-client-go/internal/transport"
	"github.com/iamkanishka/setu-client-go/pkg/setuerrors"
)

// Status is the eSign request lifecycle state.
type Status string

const (
	StatusCreated   Status = "CREATED"
	StatusPending   Status = "PENDING"
	StatusCompleted Status = "COMPLETED"
	StatusFailed    Status = "FAILED"
	StatusExpired   Status = "EXPIRED"
)

// SignerStatus is an individual signer's lifecycle state.
type SignerStatus string

const (
	SignerStatusPending  SignerStatus = "PENDING"
	SignerStatusSigned   SignerStatus = "SIGNED"
	SignerStatusRejected SignerStatus = "REJECTED"
)

// Client is the Aadhaar eSign API client.
type Client struct {
	tc      *transport.Client
	baseURL string
	headers map[string]string
}

// New creates a [*Client].
func New(tc *transport.Client, baseURL string, headers map[string]string) *Client {
	return &Client{tc: tc, baseURL: baseURL, headers: headers}
}

// SignaturePosition specifies where on the page to render the signature.
type SignaturePosition struct {
	Page   int     `json:"page"` // 1-based
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width,omitempty"`
	Height float64 `json:"height,omitempty"`
}

// Signer is one person who must sign the document.
type Signer struct {
	Name              string             `json:"name"`
	Mobile            string             `json:"mobile"`
	Email             string             `json:"email,omitempty"`
	SignaturePosition *SignaturePosition `json:"signaturePosition,omitempty"`
}

// CreateRequest is the input for [Client.Create].
type CreateRequest struct {
	// DocumentBase64 is the PDF document to be signed, base64-encoded.
	DocumentBase64 string `json:"documentBase64"`
	DocumentName   string `json:"documentName"`
	// Signers must have 1–6 entries.
	Signers     []Signer `json:"signers"`
	RedirectURL string   `json:"redirectUrl,omitempty"`
	WebhookURL  string   `json:"webhookUrl,omitempty"`
	// ExpiryMinutes is the signing session validity in minutes. Default: 60.
	ExpiryMinutes int `json:"expiryMinutes,omitempty"`
}

// CreateResponse is returned by [Client.Create].
type CreateResponse struct {
	ID         string `json:"id"`
	Status     Status `json:"status"`
	SigningURL string `json:"signingUrl"`
	// SignerURLs maps signer mobile → individual signing URL.
	SignerURLs map[string]string `json:"signerUrls,omitempty"`
	ExpiresAt  time.Time         `json:"expiresAt,omitempty"`
}

// SignerDetail describes the current signing state of one signer.
type SignerDetail struct {
	Name       string       `json:"name"`
	Mobile     string       `json:"mobile"`
	Email      string       `json:"email,omitempty"`
	Status     SignerStatus `json:"status"`
	SignedAt   *time.Time   `json:"signedAt,omitempty"`
	SigningURL string       `json:"signingUrl,omitempty"`
}

// GetResponse is returned by [Client.Get].
type GetResponse struct {
	ID           string         `json:"id"`
	Status       Status         `json:"status"`
	DocumentName string         `json:"documentName"`
	Signers      []SignerDetail `json:"signers"`
	CreatedAt    time.Time      `json:"createdAt,omitempty"`
	CompletedAt  *time.Time     `json:"completedAt,omitempty"`
}

// IsComplete returns true when all signers have signed.
func (r *GetResponse) IsComplete() bool { return r.Status == StatusCompleted }

// DownloadResponse holds the base64-encoded signed PDF.
type DownloadResponse struct {
	DocumentBase64 string `json:"documentBase64"`
}

// Create initiates an Aadhaar eSign workflow.
//
//	POST /api/esign/request
func (c *Client) Create(ctx context.Context, req *CreateRequest) (*CreateResponse, error) {
	if req == nil {
		return nil, setuerrors.NewValidationError("", "request is required")
	}
	if req.DocumentBase64 == "" {
		return nil, setuerrors.NewValidationError("documentBase64", "document (base64) is required")
	}
	if req.DocumentName == "" {
		return nil, setuerrors.NewValidationError("documentName", "documentName is required")
	}
	if len(req.Signers) == 0 {
		return nil, setuerrors.NewValidationError("signers", "at least one signer is required")
	}
	if len(req.Signers) > 6 {
		return nil, setuerrors.NewValidationError("signers", "maximum 6 signers are supported")
	}
	for i, s := range req.Signers {
		if s.Name == "" {
			return nil, setuerrors.NewValidationError(
				fmt.Sprintf("signers[%d].name", i), "signer name is required")
		}
		if s.Mobile == "" {
			return nil, setuerrors.NewValidationError(
				fmt.Sprintf("signers[%d].mobile", i), "signer mobile is required")
		}
	}
	httpReq, err := transport.NewJSONRequest(ctx, http.MethodPost,
		c.baseURL+"/api/esign/request", req)
	if err != nil {
		return nil, fmt.Errorf("esign: build create request: %w", err)
	}
	c.apply(httpReq)
	var out CreateResponse
	return &out, c.tc.DoJSON(httpReq, &out)
}

// Get retrieves the current status of an eSign request.
// Poll until [GetResponse.IsComplete] returns true.
//
//	GET /api/esign/request/:id
func (c *Client) Get(ctx context.Context, id string) (*GetResponse, error) {
	if id == "" {
		return nil, setuerrors.NewValidationError("id", "eSign request ID is required")
	}
	httpReq, err := transport.NewJSONRequest(ctx, http.MethodGet,
		fmt.Sprintf("%s/api/esign/request/%s", c.baseURL, id), nil)
	if err != nil {
		return nil, fmt.Errorf("esign: build get request: %w", err)
	}
	c.apply(httpReq)
	var out GetResponse
	return &out, c.tc.DoJSON(httpReq, &out)
}

// Download returns the signed PDF as a base64 string.
// Call only after [GetResponse.IsComplete] returns true.
//
//	GET /api/esign/request/:id/download
func (c *Client) Download(ctx context.Context, id string) (*DownloadResponse, error) {
	if id == "" {
		return nil, setuerrors.NewValidationError("id", "eSign request ID is required")
	}
	httpReq, err := transport.NewJSONRequest(ctx, http.MethodGet,
		fmt.Sprintf("%s/api/esign/request/%s/download", c.baseURL, id), nil)
	if err != nil {
		return nil, fmt.Errorf("esign: build download request: %w", err)
	}
	c.apply(httpReq)
	var out DownloadResponse
	return &out, c.tc.DoJSON(httpReq, &out)
}

func (c *Client) apply(req *http.Request) {
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}
}
