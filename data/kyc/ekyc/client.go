// Package ekyc provides the Setu Aadhaar eKYC API client.
//
// Flow:
//  1. [Client.Create] — initiate request, receive kycURL.
//  2. Redirect customer to kycURL for Aadhaar OTP verification.
//  3. Poll [Client.Get] or receive webhook when Status == SUCCESS.
//
// Status: CREATED → KYC_REQUESTED → SUCCESS | ERROR
// Setu docs: https://docs.setu.co/data/kyc (eKYC section)
package ekyc

import (
	"context"
	"fmt"
	"net/http"

	"github.com/iamkanishka/setu-client-go/internal/transport"
	"github.com/iamkanishka/setu-client-go/pkg/setuerrors"
)

// Status is the eKYC request lifecycle state.
type Status string

const (
	StatusCreated      Status = "CREATED"
	StatusKYCRequested Status = "KYC_REQUESTED"
	StatusSuccess      Status = "SUCCESS"
	StatusError        Status = "ERROR"
)

// Client is the eKYC API client.
type Client struct {
	tc      *transport.Client
	baseURL string
	headers map[string]string
}

// New creates a [*Client].
func New(tc *transport.Client, baseURL string, headers map[string]string) *Client {
	return &Client{tc: tc, baseURL: baseURL, headers: headers}
}

// CreateRequest is the input for [Client.Create].
type CreateRequest struct {
	// WebhookURL overrides the product-level webhook configured on Bridge.
	WebhookURL string `json:"webhook_url,omitempty"`
	// RedirectionURL is where Setu redirects the customer after verification.
	// Query params appended: ?status=SUCCESS&id=<ekyc_id>&errorCode=<code>
	RedirectionURL string `json:"redirection_url,omitempty"`
}

// CreateResponse is returned by [Client.Create].
type CreateResponse struct {
	ID     string `json:"id"`
	Status Status `json:"status"`
	KYCUrl string `json:"kycURL"`
}

// AadhaarAddress is the residential address from the Aadhaar record.
type AadhaarAddress struct {
	House       string `json:"house,omitempty"`
	Street      string `json:"street,omitempty"`
	Locality    string `json:"locality,omitempty"`
	Landmark    string `json:"landmark,omitempty"`
	PostOffice  string `json:"postOffice,omitempty"`
	SubDistrict string `json:"subDistrict,omitempty"`
	District    string `json:"district,omitempty"`
	State       string `json:"state,omitempty"`
	PinCode     string `json:"pin,omitempty"`
	Country     string `json:"country,omitempty"`
	VTC         string `json:"vtc,omitempty"`
}

// AadhaarDetails holds the identity data from the Aadhaar record.
type AadhaarDetails struct {
	Name          string         `json:"name"`
	DateOfBirth   string         `json:"dateOfBirth"` // year only: "1989"
	Gender        string         `json:"gender"`      // "M", "F", "T"
	Address       AadhaarAddress `json:"address"`
	Photo         string         `json:"photo,omitempty"` // base64 JPEG
	AadhaarNumber string         `json:"aadhaarNumber"`   // masked: "XXXXXXXX8888"
	GeneratedAt   string         `json:"generatedAt"`
}

// EKYCData contains the Aadhaar data returned on success.
type EKYCData struct {
	Aadhaar AadhaarDetails `json:"aadhaar"`
	XML     *struct {
		XMLBase64 string `json:"xmlBase64"`
	} `json:"xml,omitempty"`
}

// GetResponse is returned by [Client.Get].
type GetResponse struct {
	ID     string    `json:"id"`
	Status Status    `json:"status"`
	KYCUrl string    `json:"kycUrl"`
	Data   *EKYCData `json:"data,omitempty"`
}

// IsComplete returns true when the eKYC request has completed successfully.
func (r *GetResponse) IsComplete() bool { return r.Status == StatusSuccess }

// Create initiates an eKYC request and returns a URL to redirect the customer.
//
//	POST /api/ekyc/
func (c *Client) Create(ctx context.Context, req *CreateRequest) (*CreateResponse, error) {
	if req == nil {
		req = &CreateRequest{}
	}
	httpReq, err := transport.NewJSONRequest(ctx, http.MethodPost, c.baseURL+"/api/ekyc/", req)
	if err != nil {
		return nil, fmt.Errorf("ekyc: build create request: %w", err)
	}
	c.apply(httpReq)
	var out CreateResponse
	return &out, c.tc.DoJSON(httpReq, &out)
}

// Get retrieves the status and data for an eKYC request.
// Poll until [GetResponse.IsComplete] returns true.
//
//	GET /api/ekyc/:id
func (c *Client) Get(ctx context.Context, id string) (*GetResponse, error) {
	if id == "" {
		return nil, setuerrors.NewValidationError("id", "eKYC request ID is required")
	}
	httpReq, err := transport.NewJSONRequest(ctx, http.MethodGet,
		fmt.Sprintf("%s/api/ekyc/%s", c.baseURL, id), nil)
	if err != nil {
		return nil, fmt.Errorf("ekyc: build get request: %w", err)
	}
	c.apply(httpReq)
	var out GetResponse
	return &out, c.tc.DoJSON(httpReq, &out)
}

func (c *Client) apply(req *http.Request) {
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}
}
