// Package bbps provides the Setu BBPS BillCollect client.
// Setu docs: https://docs.setu.co/payments/bbps
package bbps

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/iamkanishka/setu-client-go/internal/auth"
	"github.com/iamkanishka/setu-client-go/internal/transport"
	"github.com/iamkanishka/setu-client-go/pkg/setuerrors"
)

// Client is the BBPS BillCollect client.
type Client struct {
	tc      *transport.Client
	baseURL string
	tm      *auth.TokenManager
}

// New creates a [*Client].
func New(tc *transport.Client, baseURL string, tm *auth.TokenManager) *Client {
	return &Client{tc: tc, baseURL: baseURL, tm: tm}
}

// ── Biller-side types (implement these endpoints on your server) ──────────

// BillFetchStatus indicates availability of bills for a customer.
type BillFetchStatus string

const (
	BillFetchStatusAvailable    BillFetchStatus = "AVAILABLE"
	BillFetchStatusNotAvailable BillFetchStatus = "NOT_AVAILABLE"
	BillFetchStatusPartial      BillFetchStatus = "PARTIAL"
)

// AmountExactness constrains how the customer payment amount is validated.
type AmountExactness string

const (
	AmountExactnessExact   AmountExactness = "EXACT"
	AmountExactnessExactUp AmountExactness = "EXACT_UP"
	AmountExactnessAny     AmountExactness = "ANY"
)

// BillAmount is a monetary value used in BBPS bill and settlement objects.
type BillAmount struct {
	CurrencyCode string  `json:"currencyCode"` // always "INR"
	Value        float64 `json:"value"`        // in paise
}

// SettlementAccount identifies a bank account for settlement funds.
type SettlementAccount struct {
	ID   string `json:"id"` // bank account number
	IFSC string `json:"ifsc"`
	Name string `json:"name,omitempty"`
}

// SettlementSplit defines one fractional settlement part (value in paise).
type SettlementSplit struct {
	Unit  string `json:"unit"`  // always "INR"
	Value int64  `json:"value"` // paise
}

// SettlementPart is a single split rule within a [SettlementObject].
type SettlementPart struct {
	Account SettlementAccount `json:"account"`
	Split   SettlementSplit   `json:"split"`
	Remarks string            `json:"remarks,omitempty"`
}

// SettlementObject instructs Setu how to distribute collected bill funds.
// Supports up to 5 parts. Only works with AmountExactness EXACT or EXACT_UP.
// If omitted, funds go to the primary account configured on The Bridge.
type SettlementObject struct {
	PrimaryAccount SettlementAccount `json:"primaryAccount"`
	Parts          []SettlementPart  `json:"parts,omitempty"`
}

// Bill is one outstanding bill returned by your /bills/fetch/ endpoint.
type Bill struct {
	GeneratedOn     time.Time         `json:"generatedOn"`
	DueDate         time.Time         `json:"dueDate"`
	Recurrence      string            `json:"recurrence"` // "ONE_TIME", "MONTHLY", …
	AmountExactness AmountExactness   `json:"amountExactness"`
	BillerBillID    string            `json:"billerBillID"`
	Amount          BillAmount        `json:"amount"`
	Aggregates      *BillAggregates   `json:"aggregates,omitempty"`
	Settlement      *SettlementObject `json:"settlement,omitempty"`
}

// BillAggregates is the total amount payable by the customer.
type BillAggregates struct {
	Total BillAmount `json:"total"`
}

// CustomerDetails holds customer name returned with bills.
type CustomerDetails struct {
	Name string `json:"name"`
}

// BillDetails wraps bill-fetch status and the list of bills.
type BillDetails struct {
	BillFetchStatus BillFetchStatus `json:"billFetchStatus"`
	Bills           []Bill          `json:"bills,omitempty"`
}

// FetchCustomerBillsRequest is the payload Setu POSTs to your
// {baseURL}/bills/fetch/ endpoint.
type FetchCustomerBillsRequest struct {
	CustomerIdentifiers map[string]string `json:"customerIdentifiers"`
}

// FetchCustomerBillsResponse is what your /bills/fetch/ endpoint must return.
type FetchCustomerBillsResponse struct {
	Customer    CustomerDetails `json:"customer"`
	BillDetails BillDetails     `json:"billDetails"`
}

// FetchBillReceiptRequest is the payload Setu sends to your
// {baseURL}/bills/fetchReceipt endpoint.
type FetchBillReceiptRequest struct {
	PlatformBillID string `json:"platformBillID"`
	BillerBillID   string `json:"billerBillID"`
}

// FetchBillReceiptResponse is what your /bills/fetchReceipt endpoint must return.
type FetchBillReceiptResponse struct {
	PlatformBillID string     `json:"platformBillID"`
	BillerBillID   string     `json:"billerBillID"`
	GeneratedOn    time.Time  `json:"generatedOn"`
	Amount         BillAmount `json:"amount"`
}

// ── BILL_SETTLEMENT_STATUS notification ───────────────────────────────────

// SettlementNotification is posted to your callbackURL + /notifications
// when a BILL_SETTLEMENT_STATUS event fires.
type SettlementNotification struct {
	PartnerDetails struct {
		AppID             string `json:"appID"`
		ProductInstanceID string `json:"productInstanceID"`
	} `json:"partnerDetails"`
	Events []SettlementEvent `json:"events"`
}

// SettlementEvent is one settlement event in a [SettlementNotification].
type SettlementEvent struct {
	ID        string              `json:"id"`
	Type      string              `json:"type"`      // "BILL_SETTLEMENT_STATUS"
	TimeStamp int64               `json:"timeStamp"` // Unix milliseconds
	Data      SettlementEventData `json:"data"`
}

// SettlementEventData carries the settlement payload.
type SettlementEventData struct {
	AmountSettled   BillAmount `json:"amountSettled"` // in paise
	PlatformBillIDs []string   `json:"platformBillIds"`
	Status          string     `json:"status"` // "SETTLEMENT_SUCCESSFUL"
	TransactionID   string     `json:"transactionId"`
}

// ── Setu-side transaction lookup ──────────────────────────────────────────

// Transaction is a BBPS payment transaction record from Setu.
type Transaction struct {
	ID             string     `json:"id"`
	BillerBillID   string     `json:"billerBillId"`
	PlatformBillID string     `json:"platformBillId"`
	Status         string     `json:"status"`
	Amount         BillAmount `json:"amount"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
}

// GetTransaction fetches a BBPS transaction by its Setu platform ID.
func (c *Client) GetTransaction(ctx context.Context, txnID string) (*Transaction, error) {
	if txnID == "" {
		return nil, setuerrors.NewValidationError("txnID", "txnID is required")
	}
	tok, err := c.tm.Token(ctx)
	if err != nil {
		return nil, fmt.Errorf("bbps: get token: %w", err)
	}
	req, err := transport.NewJSONRequest(ctx, http.MethodGet,
		fmt.Sprintf("%s/v1/bbps/transactions/%s", c.baseURL, txnID), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	var out Transaction
	return &out, c.tc.DoJSON(req, &out)
}
