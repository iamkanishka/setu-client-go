// Package gst provides the Setu GSTIN Verification API client.
// Sandbox: 27AAICB3918J1CT (valid), 27AAICB3919J1CT (valid + additional address).
// Setu docs: https://docs.setu.co/data/gst
package gst

import (
	"context"
	"fmt"
	"net/http"

	"github.com/iamkanishka/setu-client-go/internal/transport"
	"github.com/iamkanishka/setu-client-go/pkg/setuerrors"
)

// Client is the GST Verification API client.
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
	// GSTIN is the 15-character GST Identification Number.
	GSTIN string `json:"gstin"`
}

// VerifyResponse is returned by [Client.Verify].
type VerifyResponse struct {
	Data         GSTData `json:"data"`
	Message      string  `json:"message"`
	RequestID    string  `json:"requestId"`
	Verification string  `json:"verification"`
	TraceID      string  `json:"traceId"`
}

// GSTData contains GSTIN entity details.
type GSTData struct {
	Address      GSTAddress      `json:"address"`
	Company      GSTCompany      `json:"company"`
	GST          GSTDetails      `json:"gst"`
	Jurisdiction GSTJurisdiction `json:"jurisdiction"`
}

// GSTAddress holds principle and additional place-of-business addresses.
type GSTAddress struct {
	Principle  GSTPlace   `json:"principle"`
	Additional []GSTPlace `json:"additional,omitempty"`
}

// GSTPlace is a physical address location.
type GSTPlace struct {
	BuildingName   string `json:"buildingName,omitempty"`
	BuildingNumber string `json:"buildingNumber,omitempty"`
	Floor          string `json:"floorNo,omitempty"`
	Street         string `json:"street,omitempty"`
	Location       string `json:"location,omitempty"`
	District       string `json:"district,omitempty"`
	PinCode        string `json:"pinCode,omitempty"`
	StateCode      string `json:"stateCode,omitempty"`
}

// GSTCompany holds business entity metadata.
type GSTCompany struct {
	Name                   string `json:"name"`
	TradeName              string `json:"tradeName,omitempty"`
	Type                   string `json:"type"`
	ConstitutionOfBusiness string `json:"constitutionOfBusiness"`
	TaxPayerType           string `json:"taxPayerType"`
	Status                 string `json:"status"`
	State                  string `json:"state"`
}

// GSTDetails holds GST registration metadata.
type GSTDetails struct {
	ID                 string `json:"id"`
	RegistrationDate   string `json:"registrationDate"`
	DateOfCancellation string `json:"dateOfCancellation,omitempty"`
}

// GSTJurisdiction holds jurisdictional details for the GSTIN.
type GSTJurisdiction struct {
	Centre     string `json:"centre"`
	CentreCode string `json:"centreCode"`
	State      string `json:"state"`
	StateCode  string `json:"stateCode"`
}

// IsActive returns true when the GST registration status is "Active".
func (r *VerifyResponse) IsActive() bool { return r.Data.Company.Status == "Active" }

// Verify verifies a GSTIN.
//
//	POST /api/verify/gst
func (c *Client) Verify(ctx context.Context, req *VerifyRequest) (*VerifyResponse, error) {
	if req == nil || len(req.GSTIN) != 15 {
		return nil, setuerrors.NewValidationError("gstin", "GSTIN must be exactly 15 characters")
	}
	httpReq, err := transport.NewJSONRequest(ctx, http.MethodPost, c.baseURL+"/api/verify/gst", req)
	if err != nil {
		return nil, fmt.Errorf("gst: build request: %w", err)
	}
	for k, v := range c.headers {
		httpReq.Header.Set(k, v)
	}
	var out VerifyResponse
	return &out, c.tc.DoJSON(httpReq, &out)
}
