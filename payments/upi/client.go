// Package upi provides the Setu UPI Setu (UMAP) API client.
//
// # Products
//
//   - Flash: Dynamic QR (DQR, single-use), Static QR (SQR, multi-use)
//   - Collect: VPA-push collect requests (⚠️ scheduled for NPCI deprecation)
//   - TPV: Third Party Validation for SEBI-regulated capital-markets merchants
//   - Mandates: Recurring, One-time, Single Block Multi-Debit (SBMD)
//   - Mandate operations: Update, Revoke, Pause, Unpause
//   - Pre-debit notifications and mandate execution
//   - Refunds and dispute management
//   - Merchant onboarding (aggregator only): create merchant, check/create VPA
//
// All endpoints use Bearer token auth managed by [auth.TokenManager].
// The UMAP host is https://umap.setu.co/api.
package upi

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/iamkanishka/setu-client-go/internal/auth"
	"github.com/iamkanishka/setu-client-go/internal/transport"
	"github.com/iamkanishka/setu-client-go/pkg/setuerrors"
	"github.com/iamkanishka/setu-client-go/pkg/types"
)

// Client is the UPI Setu API client.
type Client struct {
	tc      *transport.Client
	baseURL string
	tm      *auth.TokenManager
}

// New creates a [*Client].
func New(tc *transport.Client, baseURL string, tm *auth.TokenManager) *Client {
	return &Client{tc: tc, baseURL: baseURL, tm: tm}
}

// ── Shared payment types ───────────────────────────────────────────────────

// PaymentStatus is the lifecycle state of a UPI payment transaction.
type PaymentStatus string

const (
	PaymentStatusInitiated PaymentStatus = "initiated"
	PaymentStatusPending   PaymentStatus = "pending"
	PaymentStatusSuccess   PaymentStatus = "success"
	PaymentStatusFailed    PaymentStatus = "failed"
	PaymentStatusExpired   PaymentStatus = "expired"
)

// Payment is a single UPI payment record.
type Payment struct {
	ID                  string               `json:"id"`
	Amount              int64                `json:"amount"` // paise
	Currency            types.Currency       `json:"currency"`
	Status              PaymentStatus        `json:"status"`
	CreatedAt           time.Time            `json:"createdAt"`
	MerchantID          string               `json:"merchantId"`
	MerchantReferenceID string               `json:"merchantReferenceId"`
	MerchantVPA         string               `json:"merchantVpa"`
	CustomerVPA         string               `json:"customerVpa"`
	CustomerAccountType string               `json:"customerAccountType"`
	ProductInstanceID   string               `json:"productInstanceId"`
	ProductInstanceType string               `json:"productInstanceType"` // pay_single, pay_multi, pay_single_tpv, collect
	RefID               string               `json:"refId"`
	RRN                 string               `json:"rrn"`
	TxnID               string               `json:"txnId"`
	TxnType             string               `json:"txnType"` // "pay" or "collect"
	TxnNote             string               `json:"txnNote"`
	BIN                 string               `json:"bin,omitempty"` // first 6 digits; only for credit card payments
	Reason              *types.FailureReason `json:"reason,omitempty"`
	Metadata            map[string]any       `json:"metadata,omitempty"`
	// TPV fields — only present when ProductInstanceType == "pay_single_tpv"
	TPV *TPVPaymentInfo `json:"tpv,omitempty"`
}

// TPVPaymentInfo carries the customer's bank account validated via TPV.
type TPVPaymentInfo struct {
	CustomerAccount TPVCustomerAccount `json:"customerAccount"`
}

// TPVCustomerAccount holds the account details returned in a TPV payment.
type TPVCustomerAccount struct {
	IFSC          string `json:"ifsc,omitempty"`
	AccountNumber string `json:"accountNumber,omitempty"`
	AccountName   string `json:"accountName,omitempty"`
}

// ── Flash / DQR ───────────────────────────────────────────────────────────

// CreateDQRRequest is the input for [Client.CreateDQR].
type CreateDQRRequest struct {
	// Amount in paise. Optional — see MinAmount for flexible-amount QRs.
	// When only Amount is set: displayed, customer cannot change it.
	// When only MinAmount is set: customer chooses amount ≥ MinAmount.
	// When both are set: Amount displayed, customer can change it.
	Amount      int64  `json:"amount,omitempty"`
	MinAmount   int64  `json:"minAmount,omitempty"`
	MerchantVPA string `json:"merchantVpa"`
	// ExpiryDate is the link expiry in RFC 3339 format.
	ExpiryDate      string         `json:"expiryDate,omitempty"`
	ReferenceID     string         `json:"referenceId,omitempty"`
	TransactionNote string         `json:"transactionNote,omitempty"`
	Metadata        map[string]any `json:"metadata,omitempty"`
}

// QRLink is the response from [Client.CreateDQR] and [Client.CreateSQR].
type QRLink struct {
	ID                  string `json:"id"`
	Amount              int64  `json:"amount,omitempty"`
	MinAmount           int64  `json:"minAmount,omitempty"`
	MerchantID          string `json:"merchantId"`
	MerchantReferenceID string `json:"merchantReferenceId,omitempty"`
	MerchantVPA         string `json:"merchantVpa"`
	Status              string `json:"status"` // "active", "inactive", "expired"
	// IntentLink is the raw UPI intent URL (prefix with "upi://pay?…")
	IntentLink string `json:"intentLink,omitempty"`
	// ShortLink is the Setu-hosted short URL (e.g. "upipay.setu.co/Np3K…")
	ShortLink       string               `json:"shortLink,omitempty"`
	TransactionNote string               `json:"transactionNote,omitempty"`
	ExpiryDate      string               `json:"expiryDate,omitempty"`
	RefID           string               `json:"refId,omitempty"`
	CreatedAt       time.Time            `json:"createdAt"`
	ClosedAt        *time.Time           `json:"closedAt,omitempty"`
	Reason          *types.FailureReason `json:"reason,omitempty"`
	Metadata        map[string]any       `json:"metadata,omitempty"`
}

// CreateDQR creates a Dynamic QR — a single-use UPI payment link / QR code.
// The returned IntentLink can be converted to a QR image for in-store display.
//
//	POST /v1/merchants/dqr
func (c *Client) CreateDQR(ctx context.Context, merchantID string, req *CreateDQRRequest) (*QRLink, error) {
	if merchantID == "" {
		return nil, setuerrors.NewValidationError("merchantID", "merchantID is required")
	}
	if req == nil {
		return nil, setuerrors.NewValidationError("", "request is required")
	}
	if req.MerchantVPA == "" {
		return nil, setuerrors.NewValidationError("merchantVpa", "merchantVpa is required")
	}
	httpReq, err := c.authReq(ctx, http.MethodPost, "/v1/merchants/dqr", merchantID, req)
	if err != nil {
		return nil, err
	}
	var out QRLink
	return &out, c.tc.DoJSON(httpReq, &out)
}

// GetDQR fetches the status of a Dynamic QR by its ID.
//
//	GET /v1/merchants/dqr/{id}
func (c *Client) GetDQR(ctx context.Context, merchantID, dqrID string) (*QRLink, error) {
	if merchantID == "" {
		return nil, setuerrors.NewValidationError("merchantID", "merchantID is required")
	}
	if dqrID == "" {
		return nil, setuerrors.NewValidationError("dqrID", "dqrID is required")
	}
	httpReq, err := c.authReq(ctx, http.MethodGet, "/v1/merchants/dqr/"+dqrID, merchantID, nil)
	if err != nil {
		return nil, err
	}
	var out QRLink
	return &out, c.tc.DoJSON(httpReq, &out)
}

// CreateSQRRequest is the input for [Client.CreateSQR].
type CreateSQRRequest struct {
	MerchantVPA     string         `json:"merchantVpa"`
	ReferenceID     string         `json:"referenceId,omitempty"`
	TransactionNote string         `json:"transactionNote,omitempty"`
	Metadata        map[string]any `json:"metadata,omitempty"`
}

// CreateSQR creates a Static QR — a permanent, multi-use QR code for in-store display.
//
//	POST /v1/merchants/sqr
func (c *Client) CreateSQR(ctx context.Context, merchantID string, req *CreateSQRRequest) (*QRLink, error) {
	if merchantID == "" {
		return nil, setuerrors.NewValidationError("merchantID", "merchantID is required")
	}
	if req == nil || req.MerchantVPA == "" {
		return nil, setuerrors.NewValidationError("merchantVpa", "merchantVpa is required")
	}
	httpReq, err := c.authReq(ctx, http.MethodPost, "/v1/merchants/sqr", merchantID, req)
	if err != nil {
		return nil, err
	}
	var out QRLink
	return &out, c.tc.DoJSON(httpReq, &out)
}

// GetSQR fetches the status of a Static QR by its ID.
//
//	GET /v1/merchants/sqr/{id}
func (c *Client) GetSQR(ctx context.Context, merchantID, sqrID string) (*QRLink, error) {
	if merchantID == "" {
		return nil, setuerrors.NewValidationError("merchantID", "merchantID is required")
	}
	if sqrID == "" {
		return nil, setuerrors.NewValidationError("sqrID", "sqrID is required")
	}
	httpReq, err := c.authReq(ctx, http.MethodGet, "/v1/merchants/sqr/"+sqrID, merchantID, nil)
	if err != nil {
		return nil, err
	}
	var out QRLink
	return &out, c.tc.DoJSON(httpReq, &out)
}

// GetLastPayment returns the most recent payment on any product link (DQR, SQR, TPV, Collect).
//
//	GET /v1/merchants/payments/product-instances/{productInstanceId}/last
func (c *Client) GetLastPayment(ctx context.Context, merchantID, productInstanceID string) (*Payment, error) {
	if merchantID == "" {
		return nil, setuerrors.NewValidationError("merchantID", "merchantID is required")
	}
	if productInstanceID == "" {
		return nil, setuerrors.NewValidationError("productInstanceID", "productInstanceID is required")
	}
	path := fmt.Sprintf("/v1/merchants/payments/product-instances/%s/last", productInstanceID)
	httpReq, err := c.authReq(ctx, http.MethodGet, path, merchantID, nil)
	if err != nil {
		return nil, err
	}
	var out Payment
	return &out, c.tc.DoJSON(httpReq, &out)
}

// GetPaymentHistory returns the 5 most recent payments on a product link.
// Useful for multi-use links like SQR.
//
//	GET /v1/merchants/payments/product-instances/{productInstanceId}/history
func (c *Client) GetPaymentHistory(ctx context.Context, merchantID, productInstanceID string) ([]Payment, error) {
	if merchantID == "" {
		return nil, setuerrors.NewValidationError("merchantID", "merchantID is required")
	}
	if productInstanceID == "" {
		return nil, setuerrors.NewValidationError("productInstanceID", "productInstanceID is required")
	}
	path := fmt.Sprintf("/v1/merchants/payments/product-instances/%s/history", productInstanceID)
	httpReq, err := c.authReq(ctx, http.MethodGet, path, merchantID, nil)
	if err != nil {
		return nil, err
	}
	var out struct {
		Payments []Payment `json:"payments"`
	}
	return out.Payments, c.tc.DoJSON(httpReq, &out)
}

// ── TPV (Third Party Validation) ──────────────────────────────────────────

// CustomerAccount is the registered bank account used in TPV validation.
type CustomerAccount struct {
	// IFSC is the bank branch IFSC code.
	IFSC string `json:"ifsc"`
	// AccountNumber is the customer's bank account number.
	AccountNumber string `json:"accountNumber"`
}

// CreateTPVRequest is the input for [Client.CreateTPV].
type CreateTPVRequest struct {
	// Amount in paise.
	Amount      int64  `json:"amount,omitempty"`
	MinAmount   int64  `json:"minAmount,omitempty"`
	MerchantVPA string `json:"merchantVpa"`
	// CustomerAccount contains the bank account details to validate against.
	// Payment is accepted only when the customer pays from this account.
	CustomerAccount CustomerAccount `json:"customerAccount"`
	ExpiryDate      string          `json:"expiryDate,omitempty"`
	ReferenceID     string          `json:"referenceId,omitempty"`
	TransactionNote string          `json:"transactionNote,omitempty"`
	Metadata        map[string]any  `json:"metadata,omitempty"`
}

// CreateTPV creates a TPV (Third Party Validation) payment link.
// Payment is accepted only if the customer pays from the registered bank account.
// Required by SEBI for capital-markets merchants (mutual funds, equities, etc.).
//
//	POST /v1/merchants/tpv
func (c *Client) CreateTPV(ctx context.Context, merchantID string, req *CreateTPVRequest) (*QRLink, error) {
	if merchantID == "" {
		return nil, setuerrors.NewValidationError("merchantID", "merchantID is required")
	}
	if req == nil {
		return nil, setuerrors.NewValidationError("", "request is required")
	}
	if req.MerchantVPA == "" {
		return nil, setuerrors.NewValidationError("merchantVpa", "merchantVpa is required")
	}
	if req.CustomerAccount.IFSC == "" {
		return nil, setuerrors.NewValidationError("customerAccount.ifsc", "IFSC is required for TPV")
	}
	if req.CustomerAccount.AccountNumber == "" {
		return nil, setuerrors.NewValidationError("customerAccount.accountNumber", "accountNumber is required for TPV")
	}
	httpReq, err := c.authReq(ctx, http.MethodPost, "/v1/merchants/tpv", merchantID, req)
	if err != nil {
		return nil, err
	}
	var out QRLink
	return &out, c.tc.DoJSON(httpReq, &out)
}

// GetTPV fetches the status of a TPV link by its ID.
//
//	GET /v1/merchants/tpv/{id}
func (c *Client) GetTPV(ctx context.Context, merchantID, tpvID string) (*QRLink, error) {
	if merchantID == "" {
		return nil, setuerrors.NewValidationError("merchantID", "merchantID is required")
	}
	if tpvID == "" {
		return nil, setuerrors.NewValidationError("tpvID", "tpvID is required")
	}
	httpReq, err := c.authReq(ctx, http.MethodGet, "/v1/merchants/tpv/"+tpvID, merchantID, nil)
	if err != nil {
		return nil, err
	}
	var out QRLink
	return &out, c.tc.DoJSON(httpReq, &out)
}

// ── Collect ───────────────────────────────────────────────────────────────

// CreateCollectRequest is the input for [Client.CreateCollect].
type CreateCollectRequest struct {
	// Amount in paise.
	Amount   int64          `json:"amount"`
	Currency types.Currency `json:"currency"`
	// CustomerVPA is the customer's UPI VPA where the collect request is sent.
	CustomerVPA         string `json:"customerVpa"`
	MerchantVPA         string `json:"merchantVpa"`
	MerchantReferenceID string `json:"merchantReferenceId"`
	// ExpireAfter is the number of minutes after which the request expires.
	ExpireAfter     int            `json:"expireAfter"`
	TransactionNote string         `json:"transactionNote,omitempty"`
	ProductConfigID string         `json:"productConfigId,omitempty"`
	Metadata        map[string]any `json:"metadata,omitempty"`
}

// CollectRequest represents a UPI Collect request record.
type CollectRequest struct {
	ID                  string               `json:"id"`
	Amount              int64                `json:"amount"`
	CreatedAt           time.Time            `json:"createdAt"`
	ClosedAt            *time.Time           `json:"closedAt,omitempty"`
	Currency            types.Currency       `json:"currency"`
	CustomerVPA         string               `json:"customerVpa"`
	ExpireAfter         int                  `json:"expireAfter"`
	MerchantID          string               `json:"merchantId"`
	MerchantReferenceID string               `json:"merchantReferenceId"`
	MerchantVPA         string               `json:"merchantVpa"`
	ProductConfigID     string               `json:"productConfigId,omitempty"`
	RefID               string               `json:"refId"`
	Status              string               `json:"status"`
	TransactionNote     string               `json:"transactionNote"`
	Reason              *types.FailureReason `json:"reason,omitempty"`
}

// VerifyVPA checks whether a customer UPI VPA is valid and active before
// creating a collect request.
//
//	GET /v1/merchants/vpa/validate?vpa={vpa}
func (c *Client) VerifyVPA(ctx context.Context, merchantID, vpa string) (bool, error) {
	if merchantID == "" {
		return false, setuerrors.NewValidationError("merchantID", "merchantID is required")
	}
	if vpa == "" {
		return false, setuerrors.NewValidationError("vpa", "vpa is required")
	}
	path := fmt.Sprintf("/v1/merchants/vpa/validate?vpa=%s", vpa)
	httpReq, err := c.authReq(ctx, http.MethodGet, path, merchantID, nil)
	if err != nil {
		return false, err
	}
	var out struct {
		Valid bool `json:"valid"`
	}
	return out.Valid, c.tc.DoJSON(httpReq, &out)
}

// CreateCollect pushes a UPI collect request to a customer's UPI app.
//
// ⚠️ Collect flow is scheduled for deprecation per NPCI recommendation.
//
//	POST /v1/merchants/collect
func (c *Client) CreateCollect(ctx context.Context, merchantID string, req *CreateCollectRequest) (*CollectRequest, error) {
	if merchantID == "" {
		return nil, setuerrors.NewValidationError("merchantID", "merchantID is required")
	}
	if req == nil {
		return nil, setuerrors.NewValidationError("", "request is required")
	}
	if req.CustomerVPA == "" {
		return nil, setuerrors.NewValidationError("customerVpa", "customerVpa is required")
	}
	if req.MerchantVPA == "" {
		return nil, setuerrors.NewValidationError("merchantVpa", "merchantVpa is required")
	}
	if req.Amount <= 0 {
		return nil, setuerrors.NewValidationError("amount", "amount must be positive (paise)")
	}
	httpReq, err := c.authReq(ctx, http.MethodPost, "/v1/merchants/collect", merchantID, req)
	if err != nil {
		return nil, err
	}
	var out CollectRequest
	return &out, c.tc.DoJSON(httpReq, &out)
}

// GetCollect fetches the current status of a collect request by its ID.
//
//	GET /v1/merchants/collect/{id}
func (c *Client) GetCollect(ctx context.Context, merchantID, collectID string) (*CollectRequest, error) {
	if merchantID == "" {
		return nil, setuerrors.NewValidationError("merchantID", "merchantID is required")
	}
	if collectID == "" {
		return nil, setuerrors.NewValidationError("collectID", "collectID is required")
	}
	httpReq, err := c.authReq(ctx, http.MethodGet, "/v1/merchants/collect/"+collectID, merchantID, nil)
	if err != nil {
		return nil, err
	}
	var out CollectRequest
	return &out, c.tc.DoJSON(httpReq, &out)
}

// ── Mandates ──────────────────────────────────────────────────────────────

// MandateStatus is the lifecycle state of a UPI mandate.
type MandateStatus string

const (
	MandateStatusPending  MandateStatus = "pending"
	MandateStatusLive     MandateStatus = "live"
	MandateStatusRejected MandateStatus = "rejected"
	MandateStatusPaused   MandateStatus = "paused"
	MandateStatusRevoked  MandateStatus = "revoked"
	MandateStatusExpired  MandateStatus = "expired"
)

// Mandate is a UPI mandate record.
type Mandate struct {
	ID                   string         `json:"id"`
	Amount               int64          `json:"amount"`
	AmountRule           string         `json:"amountRule"`
	AllowMultipleDebit   bool           `json:"allowMultipleDebit"`
	AutoExecute          bool           `json:"autoExecute"`
	AutoPreNotify        bool           `json:"autoPreNotify"`
	BlockFunds           bool           `json:"blockFunds"`
	CreatedAt            time.Time      `json:"createdAt"`
	CreationMode         string         `json:"creationMode"` // intent, qr, collect
	Currency             types.Currency `json:"currency"`
	CustomerRevocable    bool           `json:"customerRevocable"`
	CustomerVPA          string         `json:"customerVpa,omitempty"`
	EndDate              string         `json:"endDate"`
	ExpireAfter          int            `json:"expireAfter"`
	FirstExecutionAmount int64          `json:"firstExecutionAmount"`
	// Frequency: "as presented", "daily", "weekly", "fortnightly",
	// "monthly", "bimonthly", "quarterly", "halfyearly", "yearly", "one time",
	// "single block multi debit"
	Frequency           string        `json:"frequency"`
	InitiationMode      string        `json:"initiationMode"`
	IntentLink          string        `json:"intentLink,omitempty"`
	MaxAmountLimit      int64         `json:"maxAmountLimit"`
	MerchantID          string        `json:"merchantId"`
	MerchantReferenceID string        `json:"merchantReferenceId"`
	MerchantVPA         string        `json:"merchantVpa"`
	ProductConfigID     string        `json:"productConfigId,omitempty"`
	Purpose             string        `json:"purpose"` // "14" subscriptions, "76" block
	RecurrenceRule      string        `json:"recurrenceRule"`
	RecurrenceValue     int           `json:"recurrenceValue"`
	ShareToPayee        bool          `json:"shareToPayee"`
	StartDate           string        `json:"startDate"`
	Status              MandateStatus `json:"status"`
	TransactionNote     string        `json:"transactionNote,omitempty"`
	TxnID               string        `json:"txnId,omitempty"`
	// UMN (Unique Mandate Number) is needed for pre-debit notification and execution.
	UMN string `json:"umn,omitempty"`
}

// CreateMandateRequest is shared across recurring, one-time, and SBMD mandates.
type CreateMandateRequest struct {
	// Amount in paise.
	Amount             int64          `json:"amount"`
	AmountRule         string         `json:"amountRule"` // "max" or "exact"
	AllowMultipleDebit bool           `json:"allowMultipleDebit"`
	AutoExecute        bool           `json:"autoExecute"`
	AutoPreNotify      bool           `json:"autoPreNotify"`
	BlockFunds         bool           `json:"blockFunds"`
	CreationMode       string         `json:"creationMode"` // "intent", "qr", "collect"
	Currency           types.Currency `json:"currency"`
	CustomerRevocable  bool           `json:"customerRevocable"`
	// CustomerVPA is required only for collect-based mandates.
	CustomerVPA string `json:"customerVpa,omitempty"`
	// EndDate in ddMMyyyy format.
	EndDate              string `json:"endDate"`
	ExpireAfter          int    `json:"expireAfter"`
	FirstExecutionAmount int64  `json:"firstExecutionAmount"`
	// Frequency: "as presented" | "daily" | "weekly" | "fortnightly" |
	// "monthly" | "bimonthly" | "quarterly" | "halfyearly" | "yearly" |
	// "one time"
	Frequency string `json:"frequency"`
	// InitiationMode: "00" collect, "01" qr, "04" intent
	InitiationMode      string `json:"initiationMode"`
	MaxAmountLimit      int64  `json:"maxAmountLimit"`
	MerchantReferenceID string `json:"merchantReferenceId"`
	MerchantVPA         string `json:"merchantVpa"`
	ProductConfigID     string `json:"productConfigId,omitempty"`
	// Purpose: "14" for subscriptions, "76" for single-block-multi-debit
	Purpose string `json:"purpose"`
	// RecurrenceRule: "on", "before", "after"; use "on" for most cases
	RecurrenceRule string `json:"recurrenceRule"`
	// RecurrenceValue: 0 for "as presented" / one-time mandates
	RecurrenceValue int  `json:"recurrenceValue"`
	ShareToPayee    bool `json:"shareToPayee"`
	// StartDate in ddMMyyyy format.
	StartDate       string `json:"startDate"`
	TransactionNote string `json:"transactionNote,omitempty"`
}

// CreateMandate creates a UPI mandate (recurring, one-time, or SBMD).
//
//	POST /v1/merchants/mandates
func (c *Client) CreateMandate(ctx context.Context, merchantID string, req *CreateMandateRequest) (*Mandate, error) {
	if merchantID == "" {
		return nil, setuerrors.NewValidationError("merchantID", "merchantID is required")
	}
	if err := validateCreateMandate(req); err != nil {
		return nil, err
	}
	httpReq, err := c.authReq(ctx, http.MethodPost, "/v1/merchants/mandates", merchantID, req)
	if err != nil {
		return nil, err
	}
	var out Mandate
	return &out, c.tc.DoJSON(httpReq, &out)
}

// GetMandate retrieves the current status of a mandate by ID.
//
//	GET /v1/merchants/mandates/{id}
func (c *Client) GetMandate(ctx context.Context, merchantID, mandateID string) (*Mandate, error) {
	if merchantID == "" {
		return nil, setuerrors.NewValidationError("merchantID", "merchantID is required")
	}
	if mandateID == "" {
		return nil, setuerrors.NewValidationError("mandateID", "mandateID is required")
	}
	httpReq, err := c.authReq(ctx, http.MethodGet, "/v1/merchants/mandates/"+mandateID, merchantID, nil)
	if err != nil {
		return nil, err
	}
	var out Mandate
	return &out, c.tc.DoJSON(httpReq, &out)
}

// MandateOperation represents the result of an update/revoke/pause/unpause operation.
type MandateOperation struct {
	ID                  string    `json:"id"`
	AmountLimit         int64     `json:"amountLimit,omitempty"`
	CreatedAt           time.Time `json:"createdAt"`
	EndDate             string    `json:"endDate,omitempty"`
	ExpireAfter         int       `json:"expireAfter,omitempty"`
	MandateID           string    `json:"mandateId"`
	MerchantID          string    `json:"merchantId"`
	MerchantReferenceID string    `json:"merchantReferenceId"`
	Mode                string    `json:"mode"` // "collect" or "intent"/"qr"
	Status              string    `json:"status"`
	TxnID               string    `json:"txnId,omitempty"`
	Type                string    `json:"type"` // "update", "revoke", "pause", "unpause"
	UMN                 string    `json:"umn,omitempty"`
	// IntentLink is only set for intent/QR-based operations.
	IntentLink string `json:"intentLink,omitempty"`
}

// UpdateMandateRequest is the input for [Client.UpdateMandate].
type UpdateMandateRequest struct {
	// AmountLimit is the new maximum debit amount in paise.
	AmountLimit int64 `json:"amountLimit,omitempty"`
	// EndDate is the new mandate end date in ddMMyyyy format.
	// Cannot be updated for SBMD mandates.
	EndDate             string `json:"endDate,omitempty"`
	ExpireAfter         int    `json:"expireAfter,omitempty"`
	MerchantReferenceID string `json:"merchantReferenceId"`
}

// UpdateMandate modifies a mandate's amount or end date.
// Returns an intent link for intent-based mandates, which the customer must approve.
//
//	PUT /v1/merchants/mandates/{mandateId}/modify
func (c *Client) UpdateMandate(ctx context.Context, merchantID, mandateID string, req *UpdateMandateRequest) (*MandateOperation, error) {
	if merchantID == "" {
		return nil, setuerrors.NewValidationError("merchantID", "merchantID is required")
	}
	if mandateID == "" {
		return nil, setuerrors.NewValidationError("mandateID", "mandateID is required")
	}
	if req == nil {
		return nil, setuerrors.NewValidationError("", "request is required")
	}
	path := fmt.Sprintf("/v1/merchants/mandates/%s/modify", mandateID)
	httpReq, err := c.authReq(ctx, http.MethodPut, path, merchantID, req)
	if err != nil {
		return nil, err
	}
	var out MandateOperation
	return &out, c.tc.DoJSON(httpReq, &out)
}

// RevokeMandateRequest is the input for [Client.RevokeMandate].
type RevokeMandateRequest struct {
	ExpireAfter         int    `json:"expireAfter,omitempty"`
	MerchantReferenceID string `json:"merchantReferenceId"`
}

// RevokeMandate initiates a merchant-side mandate revocation via collect flow.
// The customer must approve the revocation via their UPI app.
// Note: SBMD mandates can only be revoked by the merchant, not the customer.
//
//	PUT /v1/merchants/mandates/{mandateId}/revoke
func (c *Client) RevokeMandate(ctx context.Context, merchantID, mandateID string, req *RevokeMandateRequest) (*MandateOperation, error) {
	if merchantID == "" {
		return nil, setuerrors.NewValidationError("merchantID", "merchantID is required")
	}
	if mandateID == "" {
		return nil, setuerrors.NewValidationError("mandateID", "mandateID is required")
	}
	if req == nil {
		req = &RevokeMandateRequest{}
	}
	path := fmt.Sprintf("/v1/merchants/mandates/%s/revoke", mandateID)
	httpReq, err := c.authReq(ctx, http.MethodPut, path, merchantID, req)
	if err != nil {
		return nil, err
	}
	var out MandateOperation
	return &out, c.tc.DoJSON(httpReq, &out)
}

// GetMandateOperation retrieves the status of a mandate operation (update/revoke/pause/unpause).
//
//	GET /v1/merchants/mandate-operations/{id}
func (c *Client) GetMandateOperation(ctx context.Context, merchantID, operationID string) (*MandateOperation, error) {
	if merchantID == "" {
		return nil, setuerrors.NewValidationError("merchantID", "merchantID is required")
	}
	if operationID == "" {
		return nil, setuerrors.NewValidationError("operationID", "operationID is required")
	}
	httpReq, err := c.authReq(ctx, http.MethodGet, "/v1/merchants/mandate-operations/"+operationID, merchantID, nil)
	if err != nil {
		return nil, err
	}
	var out MandateOperation
	return &out, c.tc.DoJSON(httpReq, &out)
}

// PreDebitNotifyRequest is the input for [Client.PreDebitNotify].
type PreDebitNotifyRequest struct {
	// Amount in paise — must satisfy the amountRule and maxAmountLimit of the mandate.
	Amount int64 `json:"amount"`
	// ExecutionDate in ddMMyyyy format — must be 48–72 hours from now.
	ExecutionDate       string `json:"executionDate"`
	MerchantReferenceID string `json:"merchantReferenceId"`
	SequenceNumber      int    `json:"sequenceNumber"`
	// UMN is the Unique Mandate Number from the mandate record.
	UMN string `json:"umn"`
}

// PreDebitNotification is the response from [Client.PreDebitNotify].
type PreDebitNotification struct {
	ID                  string    `json:"id"`
	Amount              int64     `json:"amount"`
	CreatedAt           time.Time `json:"createdAt"`
	ExecutionDate       string    `json:"executionDate"`
	MandateID           string    `json:"mandateId"`
	MerchantID          string    `json:"merchantId"`
	MerchantReferenceID string    `json:"merchantReferenceId"`
	SequenceNumber      int       `json:"sequenceNumber"`
	Status              string    `json:"status"`
	TxnID               string    `json:"txnId"`
	UMN                 string    `json:"umn"`
}

// PreDebitNotify sends a pre-debit notification 48–72 hours before mandate execution.
// Required when autoPreNotify is false.
//
//	POST /v1/merchants/mandates/{mandateId}/notify
func (c *Client) PreDebitNotify(ctx context.Context, merchantID, mandateID string, req *PreDebitNotifyRequest) (*PreDebitNotification, error) {
	if merchantID == "" {
		return nil, setuerrors.NewValidationError("merchantID", "merchantID is required")
	}
	if mandateID == "" {
		return nil, setuerrors.NewValidationError("mandateID", "mandateID is required")
	}
	if req == nil {
		return nil, setuerrors.NewValidationError("", "request is required")
	}
	if req.UMN == "" {
		return nil, setuerrors.NewValidationError("umn", "UMN is required")
	}
	path := fmt.Sprintf("/v1/merchants/mandates/%s/notify", mandateID)
	httpReq, err := c.authReq(ctx, http.MethodPost, path, merchantID, req)
	if err != nil {
		return nil, err
	}
	var out PreDebitNotification
	return &out, c.tc.DoJSON(httpReq, &out)
}

// GetPreDebitNotification retrieves the status of a pre-debit notification.
//
//	GET /v1/merchants/mandate-pre-debit-notifications/{id}
func (c *Client) GetPreDebitNotification(ctx context.Context, merchantID, notificationID string) (*PreDebitNotification, error) {
	if merchantID == "" {
		return nil, setuerrors.NewValidationError("merchantID", "merchantID is required")
	}
	if notificationID == "" {
		return nil, setuerrors.NewValidationError("notificationID", "notificationID is required")
	}
	httpReq, err := c.authReq(ctx, http.MethodGet,
		"/v1/merchants/mandate-pre-debit-notifications/"+notificationID, merchantID, nil)
	if err != nil {
		return nil, err
	}
	var out PreDebitNotification
	return &out, c.tc.DoJSON(httpReq, &out)
}

// ExecuteMandateRequest is the input for [Client.ExecuteMandate].
type ExecuteMandateRequest struct {
	// Amount in paise — must equal the amount in the pre-debit notification.
	Amount              int64  `json:"amount"`
	MerchantReferenceID string `json:"merchantReferenceId"`
	Remark              string `json:"remark,omitempty"`
	SequenceNumber      int    `json:"sequenceNumber"`
	UMN                 string `json:"umn"`
}

// MandateExecution is the response from [Client.ExecuteMandate].
type MandateExecution struct {
	ID                  string    `json:"id"`
	Amount              int64     `json:"amount"`
	CreatedAt           time.Time `json:"createdAt"`
	MandateID           string    `json:"mandateId"`
	MerchantID          string    `json:"merchantId"`
	MerchantReferenceID string    `json:"merchantReferenceId"`
	Remark              string    `json:"remark,omitempty"`
	SequenceNumber      int       `json:"sequenceNumber"`
	Status              string    `json:"status"`
	TxnID               string    `json:"txnId"`
	UMN                 string    `json:"umn"`
}

// ExecuteMandate debits the customer's bank account for an active mandate.
// Pre-debit notification must be sent successfully before calling this.
// Only required when autoExecute is false.
//
//	POST /v1/merchants/mandates/{mandateId}/execute
func (c *Client) ExecuteMandate(ctx context.Context, merchantID, mandateID string, req *ExecuteMandateRequest) (*MandateExecution, error) {
	if merchantID == "" {
		return nil, setuerrors.NewValidationError("merchantID", "merchantID is required")
	}
	if mandateID == "" {
		return nil, setuerrors.NewValidationError("mandateID", "mandateID is required")
	}
	if req == nil {
		return nil, setuerrors.NewValidationError("", "request is required")
	}
	if req.UMN == "" {
		return nil, setuerrors.NewValidationError("umn", "UMN is required")
	}
	path := fmt.Sprintf("/v1/merchants/mandates/%s/execute", mandateID)
	httpReq, err := c.authReq(ctx, http.MethodPost, path, merchantID, req)
	if err != nil {
		return nil, err
	}
	var out MandateExecution
	return &out, c.tc.DoJSON(httpReq, &out)
}

// GetMandateExecution retrieves the status of a mandate execution by its ID.
//
//	GET /v1/merchants/mandate-executions/{id}
func (c *Client) GetMandateExecution(ctx context.Context, merchantID, executionID string) (*MandateExecution, error) {
	if merchantID == "" {
		return nil, setuerrors.NewValidationError("merchantID", "merchantID is required")
	}
	if executionID == "" {
		return nil, setuerrors.NewValidationError("executionID", "executionID is required")
	}
	httpReq, err := c.authReq(ctx, http.MethodGet, "/v1/merchants/mandate-executions/"+executionID, merchantID, nil)
	if err != nil {
		return nil, err
	}
	var out MandateExecution
	return &out, c.tc.DoJSON(httpReq, &out)
}

// ── Refunds ───────────────────────────────────────────────────────────────

// RefundStatus is the lifecycle state of a UPI refund.
type RefundStatus string

const (
	RefundStatusPending    RefundStatus = "refund_pending"
	RefundStatusSuccessful RefundStatus = "refund_successful"
)

// CreateRefundRequest is the input for [Client.CreateRefund].
type CreateRefundRequest struct {
	// Amount in paise — must not exceed the original payment amount.
	Amount              int64          `json:"amount"`
	Currency            types.Currency `json:"currency"`
	MerchantReferenceID string         `json:"merchantReferenceId"`
	// PaymentID is the payment `id` from the original payment record.
	PaymentID string `json:"paymentId"`
	Remarks   string `json:"remarks,omitempty"`
	Type      string `json:"type"` // "online"
}

// Refund is a UPI refund record.
type Refund struct {
	ID                  string         `json:"id"`
	Amount              int64          `json:"amount"`
	CreatedAt           time.Time      `json:"createdAt"`
	Currency            types.Currency `json:"currency"`
	MerchantReferenceID string         `json:"merchantReferenceId"`
	PaymentID           string         `json:"paymentId"`
	Remarks             string         `json:"remarks,omitempty"`
	Status              RefundStatus   `json:"status"`
	Type                string         `json:"type"`
}

// CreateRefund initiates a refund for a completed UPI payment.
// Refunds must be initiated within 60 days from the payment date.
//
//	POST /v1/merchants/refunds
func (c *Client) CreateRefund(ctx context.Context, merchantID string, req *CreateRefundRequest) (*Refund, error) {
	if merchantID == "" {
		return nil, setuerrors.NewValidationError("merchantID", "merchantID is required")
	}
	if req == nil {
		return nil, setuerrors.NewValidationError("", "request is required")
	}
	if req.PaymentID == "" {
		return nil, setuerrors.NewValidationError("paymentId", "paymentId is required")
	}
	if req.Amount <= 0 {
		return nil, setuerrors.NewValidationError("amount", "amount must be positive (paise)")
	}
	if req.Type == "" {
		req.Type = "online"
	}
	httpReq, err := c.authReq(ctx, http.MethodPost, "/v1/merchants/refunds", merchantID, req)
	if err != nil {
		return nil, err
	}
	var out Refund
	return &out, c.tc.DoJSON(httpReq, &out)
}

// GetRefund retrieves the status of a refund by its ID.
//
//	GET /v1/merchants/refunds/{id}
func (c *Client) GetRefund(ctx context.Context, merchantID, refundID string) (*Refund, error) {
	if merchantID == "" {
		return nil, setuerrors.NewValidationError("merchantID", "merchantID is required")
	}
	if refundID == "" {
		return nil, setuerrors.NewValidationError("refundID", "refundID is required")
	}
	httpReq, err := c.authReq(ctx, http.MethodGet, "/v1/merchants/refunds/"+refundID, merchantID, nil)
	if err != nil {
		return nil, err
	}
	var out Refund
	return &out, c.tc.DoJSON(httpReq, &out)
}

// ── Disputes ──────────────────────────────────────────────────────────────

// DisputeStatus is the lifecycle state of a UPI dispute.
type DisputeStatus string

const (
	DisputeStatusOpen     DisputeStatus = "dispute_open"
	DisputeStatusClosed   DisputeStatus = "dispute_closed"
	DisputeStatusInReview DisputeStatus = "dispute_in_review"
	DisputeStatusWon      DisputeStatus = "dispute_won"
	DisputeStatusLost     DisputeStatus = "dispute_lost"
)

// Dispute is a UPI dispute record raised by a customer.
type Dispute struct {
	ID        string        `json:"id"`
	PaymentID string        `json:"paymentId"`
	Amount    int64         `json:"amount"`
	Status    DisputeStatus `json:"status"`
	Reason    string        `json:"reason,omitempty"`
	CreatedAt time.Time     `json:"createdAt"`
	UpdatedAt time.Time     `json:"updatedAt"`
}

// GetDispute retrieves the details of a dispute by its ID.
//
//	GET /v1/merchants/disputes/{id}
func (c *Client) GetDispute(ctx context.Context, merchantID, disputeID string) (*Dispute, error) {
	if merchantID == "" {
		return nil, setuerrors.NewValidationError("merchantID", "merchantID is required")
	}
	if disputeID == "" {
		return nil, setuerrors.NewValidationError("disputeID", "disputeID is required")
	}
	httpReq, err := c.authReq(ctx, http.MethodGet, "/v1/merchants/disputes/"+disputeID, merchantID, nil)
	if err != nil {
		return nil, err
	}
	var out Dispute
	return &out, c.tc.DoJSON(httpReq, &out)
}

// AcceptDispute accepts a customer dispute, triggering an automatic refund.
// Accepting marks the dispute as dispute_closed.
//
//	PUT /v1/merchants/disputes/{id}/accept
func (c *Client) AcceptDispute(ctx context.Context, merchantID, disputeID string) (*Dispute, error) {
	if merchantID == "" {
		return nil, setuerrors.NewValidationError("merchantID", "merchantID is required")
	}
	if disputeID == "" {
		return nil, setuerrors.NewValidationError("disputeID", "disputeID is required")
	}
	httpReq, err := c.authReq(ctx, http.MethodPut, "/v1/merchants/disputes/"+disputeID+"/accept", merchantID, nil)
	if err != nil {
		return nil, err
	}
	var out Dispute
	return &out, c.tc.DoJSON(httpReq, &out)
}

// RejectDisputeRequest is the input for [Client.RejectDispute].
type RejectDisputeRequest struct {
	// Evidence is a base64-encoded document (e.g. invoice PDF) to submit to NPCI.
	Evidence     string `json:"evidence"`
	EvidenceType string `json:"evidenceType"` // "invoice", "receipt", etc.
	Remarks      string `json:"remarks,omitempty"`
}

// RejectDispute contests a customer dispute by submitting evidence to NPCI.
// The dispute moves to dispute_in_review. NPCI will decide the outcome.
//
//	PUT /v1/merchants/disputes/{id}/reject
func (c *Client) RejectDispute(ctx context.Context, merchantID, disputeID string, req *RejectDisputeRequest) (*Dispute, error) {
	if merchantID == "" {
		return nil, setuerrors.NewValidationError("merchantID", "merchantID is required")
	}
	if disputeID == "" {
		return nil, setuerrors.NewValidationError("disputeID", "disputeID is required")
	}
	if req == nil || req.Evidence == "" {
		return nil, setuerrors.NewValidationError("evidence", "evidence is required to reject a dispute")
	}
	httpReq, err := c.authReq(ctx, http.MethodPut, "/v1/merchants/disputes/"+disputeID+"/reject", merchantID, req)
	if err != nil {
		return nil, err
	}
	var out Dispute
	return &out, c.tc.DoJSON(httpReq, &out)
}

// ── Aggregator Merchant Onboarding ────────────────────────────────────────

// CreateMerchantRequest is the input for [Client.CreateMerchant] (aggregator only).
type CreateMerchantRequest struct {
	AggregatorAccountID string `json:"aggregatorAccountId"`
	BusinessName        string `json:"businessName"`
	FranchiseName       string `json:"franchiseName"`
	LegalName           string `json:"legalName"`
	BusinessType        string `json:"businessType"` // "PUBLIC", "PRIVATE", "PARTNERSHIP"
	Genre               string `json:"genre"`        // "ONLINE", "OFFLINE"
	MCC                 string `json:"mcc"`
	// Optional overrides
	StandardAccountID string   `json:"standardAccountId,omitempty"`
	MerchantType      string   `json:"merchantType,omitempty"`   // "small" (default: "large")
	OnboardingType    string   `json:"onboardingType,omitempty"` // "bank" (default: "aggregator")
	AcceptDeemedTxns  bool     `json:"acceptDeemedTxns,omitempty"`
	PaymentModes      []string `json:"paymentModes,omitempty"`
	Products          []string `json:"products,omitempty"`
	VPAHandles        []string `json:"vpaHandles,omitempty"`
}

// Merchant represents a merchant entity on the UPI Setu platform.
type Merchant struct {
	ID           string `json:"id"`
	BusinessName string `json:"businessName"`
	LegalName    string `json:"legalName"`
	MCC          string `json:"mcc"`
	Status       string `json:"status"`
}

// CreateMerchant onboards a new merchant under an aggregator account.
//
//	POST /v1/aggregators/merchants
func (c *Client) CreateMerchant(ctx context.Context, req *CreateMerchantRequest) (*Merchant, error) {
	if req == nil {
		return nil, setuerrors.NewValidationError("", "request is required")
	}
	if req.AggregatorAccountID == "" {
		return nil, setuerrors.NewValidationError("aggregatorAccountId", "aggregatorAccountId is required")
	}
	if req.BusinessName == "" {
		return nil, setuerrors.NewValidationError("businessName", "businessName is required")
	}
	httpReq, err := c.bearerReq(ctx, http.MethodPost, "/v1/aggregators/merchants", req)
	if err != nil {
		return nil, err
	}
	var out Merchant
	return &out, c.tc.DoJSON(httpReq, &out)
}

// GetMerchant retrieves a merchant by ID (aggregator only).
//
//	GET /v1/aggregators/merchants/{id}
func (c *Client) GetMerchant(ctx context.Context, merchantID string) (*Merchant, error) {
	if merchantID == "" {
		return nil, setuerrors.NewValidationError("merchantID", "merchantID is required")
	}
	httpReq, err := c.bearerReq(ctx, http.MethodGet, "/v1/aggregators/merchants/"+merchantID, nil)
	if err != nil {
		return nil, err
	}
	var out Merchant
	return &out, c.tc.DoJSON(httpReq, &out)
}

// CheckVPAAvailabilityRequest is the input for [Client.CheckVPAAvailability].
type CheckVPAAvailabilityRequest struct {
	VPA string `json:"vpa"`
}

// VPAAvailability is returned by [Client.CheckVPAAvailability].
type VPAAvailability struct {
	VPA       string `json:"vpa"`
	Available bool   `json:"available"`
}

// CheckVPAAvailability checks whether a VPA is available before assigning it
// to a merchant. Recommended before calling [Client.CreateVPA].
//
//	POST /v1/aggregators/vpa/check
func (c *Client) CheckVPAAvailability(ctx context.Context, req *CheckVPAAvailabilityRequest) (*VPAAvailability, error) {
	if req == nil || req.VPA == "" {
		return nil, setuerrors.NewValidationError("vpa", "vpa is required")
	}
	httpReq, err := c.bearerReq(ctx, http.MethodPost, "/v1/aggregators/vpa/check", req)
	if err != nil {
		return nil, err
	}
	var out VPAAvailability
	return &out, c.tc.DoJSON(httpReq, &out)
}

// CreateVPARequest is the input for [Client.CreateVPA].
type CreateVPARequest struct {
	VPA         string `json:"vpa"`
	ReferenceID string `json:"referenceId,omitempty"`
}

// VPA represents a UPI Virtual Payment Address record.
type VPA struct {
	ID          string `json:"id"`
	VPA         string `json:"vpa"`
	MerchantID  string `json:"merchantId"`
	ReferenceID string `json:"referenceId,omitempty"`
	Status      string `json:"status"`
}

// CreateVPA assigns a VPA to a merchant (aggregator only).
//
//	POST /v1/merchants/vpas
func (c *Client) CreateVPA(ctx context.Context, merchantID string, req *CreateVPARequest) (*VPA, error) {
	if merchantID == "" {
		return nil, setuerrors.NewValidationError("merchantID", "merchantID is required")
	}
	if req == nil || req.VPA == "" {
		return nil, setuerrors.NewValidationError("vpa", "vpa is required")
	}
	httpReq, err := c.authReq(ctx, http.MethodPost, "/v1/merchants/vpas", merchantID, req)
	if err != nil {
		return nil, err
	}
	var out VPA
	return &out, c.tc.DoJSON(httpReq, &out)
}

// ── Internal helpers ──────────────────────────────────────────────────────

// authReq builds a request with Bearer token + merchantId header.
func (c *Client) authReq(ctx context.Context, method, path, merchantID string, body any) (*http.Request, error) {
	tok, err := c.tm.Token(ctx)
	if err != nil {
		return nil, fmt.Errorf("upi: get token: %w", err)
	}
	req, err := transport.NewJSONRequest(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("merchantId", merchantID)
	return req, nil
}

// bearerReq builds a request with only a Bearer token (aggregator-level endpoints).
func (c *Client) bearerReq(ctx context.Context, method, path string, body any) (*http.Request, error) {
	tok, err := c.tm.Token(ctx)
	if err != nil {
		return nil, fmt.Errorf("upi: get token: %w", err)
	}
	req, err := transport.NewJSONRequest(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	return req, nil
}

// ── Validation ────────────────────────────────────────────────────────────

func validateCreateMandate(r *CreateMandateRequest) error {
	if r == nil {
		return setuerrors.NewValidationError("", "request is required")
	}
	if r.MerchantVPA == "" {
		return setuerrors.NewValidationError("merchantVpa", "merchantVpa is required")
	}
	if r.StartDate == "" {
		return setuerrors.NewValidationError("startDate", "startDate is required (ddMMyyyy)")
	}
	if r.EndDate == "" {
		return setuerrors.NewValidationError("endDate", "endDate is required (ddMMyyyy)")
	}
	if r.Frequency == "" {
		return setuerrors.NewValidationError("frequency", "frequency is required")
	}
	return nil
}
