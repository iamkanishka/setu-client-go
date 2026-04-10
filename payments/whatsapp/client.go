// Package whatsapp provides the Setu WhatsApp Collect client.
// Setu docs: https://docs.setu.co/payments/whatsapp-collect
package whatsapp

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/iamkanishka/setu-client-go/internal/auth"
	"github.com/iamkanishka/setu-client-go/internal/transport"
	"github.com/iamkanishka/setu-client-go/pkg/setuerrors"
)

// Client is the WhatsApp Collect API client.
type Client struct {
	tc      *transport.Client
	baseURL string
	tm      *auth.TokenManager
}

// New creates a [*Client].
func New(tc *transport.Client, baseURL string, tm *auth.TokenManager) *Client {
	return &Client{tc: tc, baseURL: baseURL, tm: tm}
}

// SendReminderRequest is the input for [Client.SendReminder].
type SendReminderRequest struct {
	CustomerMobile string `json:"customerMobile"`
	CustomerName   string `json:"customerName,omitempty"`
	// BillAmount in paise.
	BillAmount   int64     `json:"billAmount"`
	BillerBillID string    `json:"billerBillId"`
	DueDate      time.Time `json:"dueDate"`
	TemplateName string    `json:"templateName,omitempty"`
	// ExpiryMinutes is the payment link validity in minutes (default 1440 = 24h).
	ExpiryMinutes int    `json:"expiryMinutes,omitempty"`
	LanguageCode  string `json:"languageCode,omitempty"` // "en", "hi", …
}

// SendReminderResponse is returned by [Client.SendReminder].
type SendReminderResponse struct {
	ID             string `json:"id"`
	DeliveryStatus string `json:"deliveryStatus"`
	PaymentStatus  string `json:"paymentStatus"`
	PaymentLinkURL string `json:"paymentLinkUrl,omitempty"`
}

// ReminderStatus is returned by [Client.GetReminderStatus].
type ReminderStatus struct {
	ID             string     `json:"id"`
	CustomerMobile string     `json:"customerMobile"`
	BillerBillID   string     `json:"billerBillId"`
	DeliveryStatus string     `json:"deliveryStatus"`
	PaymentStatus  string     `json:"paymentStatus"`
	BillAmount     int64      `json:"billAmount"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
	PaidAt         *time.Time `json:"paidAt,omitempty"`
}

// SendReminder sends a WhatsApp bill payment reminder with an embedded link.
func (c *Client) SendReminder(ctx context.Context, req *SendReminderRequest) (*SendReminderResponse, error) {
	if req == nil {
		return nil, setuerrors.NewValidationError("", "request is required")
	}
	if req.CustomerMobile == "" {
		return nil, setuerrors.NewValidationError("customerMobile", "customerMobile is required")
	}
	if req.BillAmount <= 0 {
		return nil, setuerrors.NewValidationError("billAmount", "billAmount must be positive (paise)")
	}
	if req.BillerBillID == "" {
		return nil, setuerrors.NewValidationError("billerBillId", "billerBillId is required")
	}
	tok, err := c.tm.Token(ctx)
	if err != nil {
		return nil, fmt.Errorf("whatsapp: get token: %w", err)
	}
	httpReq, err := transport.NewJSONRequest(ctx, http.MethodPost, c.baseURL+"/v1/whatsapp/bills", req)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok)
	var out SendReminderResponse
	return &out, c.tc.DoJSON(httpReq, &out)
}

// GetReminderStatus fetches delivery and payment status of a sent reminder.
func (c *Client) GetReminderStatus(ctx context.Context, id string) (*ReminderStatus, error) {
	if id == "" {
		return nil, setuerrors.NewValidationError("id", "reminder ID is required")
	}
	tok, err := c.tm.Token(ctx)
	if err != nil {
		return nil, fmt.Errorf("whatsapp: get token: %w", err)
	}
	httpReq, err := transport.NewJSONRequest(ctx, http.MethodGet,
		fmt.Sprintf("%s/v1/whatsapp/bills/%s", c.baseURL, id), nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok)
	var out ReminderStatus
	return &out, c.tc.DoJSON(httpReq, &out)
}
