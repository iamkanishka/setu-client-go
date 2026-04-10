// Package digilocker provides the Setu DigiLocker API client.
//
// DigiLocker is the Government of India's digital document wallet.
// Fetch Aadhaar, driving licences, mark sheets, and more directly from
// issuing authorities with user consent.
//
// Setu docs: https://docs.setu.co/data/kyc (DigiLocker section)
package digilocker

import (
	"context"
	"fmt"
	"net/http"

	"github.com/iamkanishka/setu-client-go/internal/transport"
	"github.com/iamkanishka/setu-client-go/pkg/setuerrors"
)

// Client is the DigiLocker API client.
type Client struct {
	tc      *transport.Client
	baseURL string
	headers map[string]string
}

// New creates a [*Client].
func New(tc *transport.Client, baseURL string, headers map[string]string) *Client {
	return &Client{tc: tc, baseURL: baseURL, headers: headers}
}

// CreateSessionRequest is the input for [Client.CreateSession].
type CreateSessionRequest struct {
	// RedirectURL is where Setu redirects the customer after DigiLocker auth.
	RedirectURL string `json:"redirectUrl"`
	WebhookURL  string `json:"webhookUrl,omitempty"`
	Purpose     string `json:"purpose,omitempty"`
}

// CreateSessionResponse is returned by [Client.CreateSession].
type CreateSessionResponse struct {
	ID         string `json:"id"`
	ConsentURL string `json:"consentUrl"`
	Status     string `json:"status"`
}

// DocumentMeta summarises an available document in a session.
type DocumentMeta struct {
	Type        string `json:"type"`
	Name        string `json:"name"`
	Issuer      string `json:"issuer"`
	Description string `json:"description,omitempty"`
}

// Session holds the full DigiLocker session state.
type Session struct {
	ID        string         `json:"id"`
	Status    string         `json:"status"`
	Documents []DocumentMeta `json:"documents,omitempty"`
}

// Document holds fetched document content.
type Document struct {
	Type    string         `json:"type"`
	FileURL string         `json:"fileUrl,omitempty"`
	Data    map[string]any `json:"data,omitempty"`
}

// CreateSession initiates a DigiLocker consent session.
// Redirect the customer to ConsentURL for document access approval.
func (c *Client) CreateSession(ctx context.Context, req *CreateSessionRequest) (*CreateSessionResponse, error) {
	if req == nil || req.RedirectURL == "" {
		return nil, setuerrors.NewValidationError("redirectUrl", "redirectUrl is required")
	}
	httpReq, err := transport.NewJSONRequest(ctx, http.MethodPost,
		c.baseURL+"/api/digilocker/session", req)
	if err != nil {
		return nil, fmt.Errorf("digilocker: build create session: %w", err)
	}
	c.apply(httpReq)
	var out CreateSessionResponse
	return &out, c.tc.DoJSON(httpReq, &out)
}

// GetSession retrieves the current state of a DigiLocker session.
// Documents are listed once the customer has granted consent.
func (c *Client) GetSession(ctx context.Context, sessionID string) (*Session, error) {
	if sessionID == "" {
		return nil, setuerrors.NewValidationError("sessionID", "session ID is required")
	}
	httpReq, err := transport.NewJSONRequest(ctx, http.MethodGet,
		fmt.Sprintf("%s/api/digilocker/session/%s", c.baseURL, sessionID), nil)
	if err != nil {
		return nil, fmt.Errorf("digilocker: build get session: %w", err)
	}
	c.apply(httpReq)
	var out Session
	return &out, c.tc.DoJSON(httpReq, &out)
}

// GetDocument fetches a specific document type from an authorised session.
func (c *Client) GetDocument(ctx context.Context, sessionID, documentType string) (*Document, error) {
	if sessionID == "" {
		return nil, setuerrors.NewValidationError("sessionID", "session ID is required")
	}
	if documentType == "" {
		return nil, setuerrors.NewValidationError("documentType", "document type is required")
	}
	httpReq, err := transport.NewJSONRequest(ctx, http.MethodGet,
		fmt.Sprintf("%s/api/digilocker/session/%s/documents/%s", c.baseURL, sessionID, documentType), nil)
	if err != nil {
		return nil, fmt.Errorf("digilocker: build get document: %w", err)
	}
	c.apply(httpReq)
	var out Document
	return &out, c.tc.DoJSON(httpReq, &out)
}

func (c *Client) apply(req *http.Request) {
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}
}
