// Package billpay provides the Setu BBPS BillPay (agent) client.
// Setu docs: https://docs.setu.co/payments/billpay
package billpay

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/iamkanishka/setu-client-go/internal/auth"
	"github.com/iamkanishka/setu-client-go/internal/transport"
	"github.com/iamkanishka/setu-client-go/pkg/setuerrors"
)

// Client is the BBPS BillPay API client.
type Client struct {
	tc      *transport.Client
	baseURL string
	tm      *auth.TokenManager
}

// New creates a [*Client].
func New(tc *transport.Client, baseURL string, tm *auth.TokenManager) *Client {
	return &Client{tc: tc, baseURL: baseURL, tm: tm}
}

// FetchBillRequest is the input for [Client.FetchBill].
type FetchBillRequest struct {
	BillerID            string            `json:"billerId"`
	CustomerIdentifiers map[string]string `json:"customerIdentifiers"`
}

// FetchedBill holds a customer's bill retrieved from the BBPS network.
type FetchedBill struct {
	BillerBillID string    `json:"billerBillId"`
	Amount       int64     `json:"amount"` // paise
	DueDate      time.Time `json:"dueDate"`
	BillerName   string    `json:"billerName"`
}

// PayBillRequest is the input for [Client.PayBill].
type PayBillRequest struct {
	BillerID            string            `json:"billerId"`
	CustomerIdentifiers map[string]string `json:"customerIdentifiers"`
	// Amount in paise.
	Amount              int64  `json:"amount"`
	PaymentMode         string `json:"paymentMode"` // "UPI", "NETBANKING", "CARD", etc.
	CustomerMobile      string `json:"customerMobile,omitempty"`
	MerchantReferenceID string `json:"merchantReferenceId"`
}

// PayBillResponse is returned by [Client.PayBill].
type PayBillResponse struct {
	TransactionID       string    `json:"transactionId"`
	Status              string    `json:"status"`
	Amount              int64     `json:"amount"`
	BillerBillID        string    `json:"billerBillId"`
	MerchantReferenceID string    `json:"merchantReferenceId"`
	CreatedAt           time.Time `json:"createdAt"`
}

// FetchBill retrieves a customer's bill from a BBPS biller.
func (c *Client) FetchBill(ctx context.Context, req *FetchBillRequest) (*FetchedBill, error) {
	if req == nil || req.BillerID == "" {
		return nil, setuerrors.NewValidationError("billerId", "billerId is required")
	}
	tok, err := c.tm.Token(ctx)
	if err != nil {
		return nil, fmt.Errorf("billpay: get token: %w", err)
	}
	httpReq, err := transport.NewJSONRequest(ctx, http.MethodPost, c.baseURL+"/v1/billpay/bills/fetch", req)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok)
	var out FetchedBill
	return &out, c.tc.DoJSON(httpReq, &out)
}

// PayBill executes a bill payment through the BBPS network.
func (c *Client) PayBill(ctx context.Context, req *PayBillRequest) (*PayBillResponse, error) {
	if req == nil {
		return nil, setuerrors.NewValidationError("", "request is required")
	}
	if req.BillerID == "" {
		return nil, setuerrors.NewValidationError("billerId", "billerId is required")
	}
	if req.Amount <= 0 {
		return nil, setuerrors.NewValidationError("amount", "amount must be positive (paise)")
	}
	tok, err := c.tm.Token(ctx)
	if err != nil {
		return nil, fmt.Errorf("billpay: get token: %w", err)
	}
	httpReq, err := transport.NewJSONRequest(ctx, http.MethodPost, c.baseURL+"/v1/billpay/bills/pay", req)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok)
	var out PayBillResponse
	return &out, c.tc.DoJSON(httpReq, &out)
}
