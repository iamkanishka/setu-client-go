// Package webhook provides a unified http.Handler that dispatches all Setu
// webhook notifications (UPI Setu payments, mandates, refunds, disputes,
// Account Aggregator, and BBPS settlement) to typed callback functions.
//
// # Usage
//
//	h := webhook.NewHandler(webhook.Config{
//	    OnPaymentUpdate:  func(n *webhook.PaymentNotification)  { … },
//	    OnMandateUpdate:  func(n *webhook.MandateNotification)  { … },
//	    OnRefundUpdate:   func(n *webhook.RefundNotification)   { … },
//	    OnDisputeUpdate:  func(n *webhook.DisputeNotification)  { … },
//	    OnConsentUpdate:  func(n *aa.ConsentNotification)       { … },
//	    OnSessionUpdate:  func(n *aa.SessionNotification)       { … },
//	    OnBBPSSettlement: func(n *bbps.SettlementNotification)  { … },
//	})
//	http.Handle("/webhooks/setu", h)
package webhook

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/iamkanishka/setu-client-go/data/aa"
	"github.com/iamkanishka/setu-client-go/payments/bbps"
	"github.com/iamkanishka/setu-client-go/pkg/types"
)

// ── UPI Event types ───────────────────────────────────────────────────────

// EventType is the UPI Setu webhook event type string.
type EventType string

// Payment event types.
const (
	EventPaymentInitiated EventType = "payment.initiated"
	EventPaymentPending   EventType = "payment.pending"
	EventPaymentSuccess   EventType = "payment.success"
	EventPaymentFailed    EventType = "payment.failed"
)

// Mandate status event types.
const (
	EventMandateInitiated EventType = "mandate.initiated"
	EventMandateLive      EventType = "mandate.live"
	EventMandateRejected  EventType = "mandate.rejected"
	EventMandatePaused    EventType = "mandate.paused"
	EventMandateRevoked   EventType = "mandate.revoked"
	EventMandateUpdated   EventType = "mandate.updated"
)

// Mandate operation event types.
const (
	EventMandateOpCreateInitiated EventType = "mandate_operation.create.initiated"
	EventMandateOpCreateSuccess   EventType = "mandate_operation.create.success"
	EventMandateOpCreateFailed    EventType = "mandate_operation.create.failed"
	EventMandateOpUpdateInitiated EventType = "mandate_operation.update.initiated"
	EventMandateOpUpdateSuccess   EventType = "mandate_operation.update.success"
	EventMandateOpUpdateFailed    EventType = "mandate_operation.update.failed"
	EventMandateOpRevokeInitiated EventType = "mandate_operation.revoke.initiated"
	EventMandateOpRevokeSuccess   EventType = "mandate_operation.revoke.success"
	EventMandateOpRevokeFailed    EventType = "mandate_operation.revoke.failed"
	EventMandateOpExecuteSuccess  EventType = "mandate_operation.execute.success"
	EventMandateOpExecuteFailed   EventType = "mandate_operation.execute.failed"
	EventMandateOpNotifySuccess   EventType = "mandate_operation.notify.success"
	EventMandateOpNotifyFailed    EventType = "mandate_operation.notify.failed"
)

// Refund event types.
const (
	EventRefundPending    EventType = "refund.pending"
	EventRefundSuccessful EventType = "refund.successful"
)

// Dispute event types.
const (
	EventDisputeCreated  EventType = "dispute_created"
	EventDisputeOpen     EventType = "dispute_open"
	EventDisputeClosed   EventType = "dispute_closed"
	EventDisputeInReview EventType = "dispute_in_review"
	EventDisputeWon      EventType = "dispute_won"
	EventDisputeLost     EventType = "dispute_lost"
)

// ── UPI notification payload types ────────────────────────────────────────

// PaymentNotification is the full UPI payment webhook payload.
type PaymentNotification struct {
	EventID             string    `json:"eventId"`
	EventType           EventType `json:"eventType"`
	EventTS             time.Time `json:"eventTs"`
	Resource            string    `json:"resource"`
	ID                  string    `json:"id"`
	Status              string    `json:"status"`
	MerchantID          string    `json:"merchantId"`
	MerchantReferenceID string    `json:"merchantReferenceId"`
	ProductInstanceID   string    `json:"productInstanceId"`
	// ProductInstanceType: pay_single, pay_multi, pay_single_tpv, collect, offline_qr
	ProductInstanceType string         `json:"productInstanceType"`
	TxnID               string         `json:"txnId"`
	TxnType             string         `json:"txnType"` // "pay" or "collect"
	TxnTS               time.Time      `json:"txnTs"`
	RefID               string         `json:"refId"`
	RRN                 string         `json:"rrn"`
	Amount              int64          `json:"amount"` // paise
	Currency            types.Currency `json:"currency"`
	CustomerVPA         string         `json:"customerVpa"`
	MerchantVPA         string         `json:"merchantVpa"`
	CustomerAccountType string         `json:"customerAccountType"`
	TxnNote             string         `json:"txnNote"`
	BIN                 string         `json:"bin,omitempty"` // credit card payments only
	Reason              map[string]any `json:"reason,omitempty"`
	Metadata            map[string]any `json:"metadata,omitempty"`
	// TPV: only present for pay_single_tpv
	TPV *TPVInfo `json:"tpv,omitempty"`
	// TPVPlus: only present for pay_single_tpv_plus
	TPVPlus *TPVPlusInfo `json:"tpvPlus,omitempty"`
}

// TPVInfo holds the TPV bank account details (populated in payment.success).
type TPVInfo struct {
	CustomerAccount struct {
		IFSC          string `json:"ifsc"`
		AccountNumber string `json:"accountNumber"`
		AccountName   string `json:"accountName"`
	} `json:"customerAccount"`
}

// TPVPlusInfo holds TPV-Plus payer details.
type TPVPlusInfo struct {
	CustomerAccount struct {
		PayerApp  string `json:"payerApp"`
		PayerBank string `json:"payerBank"`
		PayerVPA  string `json:"payerVpa"`
	} `json:"customerAccount"`
}

// MandateNotification is the UPI mandate status webhook payload.
type MandateNotification struct {
	EventID    string    `json:"eventId"`
	EventType  EventType `json:"eventType"`
	EventTS    time.Time `json:"eventTs"`
	MerchantID string    `json:"merchantId"`
	// Data is the raw mandate or mandate-operation JSON.
	Data json.RawMessage `json:"data"`
}

// RefundNotification is the UPI refund status webhook payload.
type RefundNotification struct {
	EventID    string    `json:"eventId"`
	EventType  EventType `json:"eventType"`
	EventTS    time.Time `json:"eventTs"`
	MerchantID string    `json:"merchantId"`
	Data       struct {
		ID                  string `json:"id"`
		Amount              int64  `json:"amount"`
		Status              string `json:"status"`
		PaymentID           string `json:"paymentId"`
		MerchantReferenceID string `json:"merchantReferenceId"`
	} `json:"data"`
}

// DisputeNotification is the UPI dispute status webhook payload.
type DisputeNotification struct {
	EventID    string    `json:"eventId"`
	EventType  EventType `json:"eventType"`
	EventTS    time.Time `json:"eventTs"`
	MerchantID string    `json:"merchantId"`
	Data       struct {
		ID        string `json:"id"`
		PaymentID string `json:"paymentId"`
		Status    string `json:"status"`
		Amount    int64  `json:"amount"`
		Reason    string `json:"reason,omitempty"`
	} `json:"data"`
}

// ── Handler ───────────────────────────────────────────────────────────────

// Config holds callback functions for each notification category.
// Any nil callback is silently ignored.
type Config struct {
	// UPI Setu callbacks.
	OnPaymentUpdate func(n *PaymentNotification)
	OnMandateUpdate func(n *MandateNotification)
	OnRefundUpdate  func(n *RefundNotification)
	OnDisputeUpdate func(n *DisputeNotification)

	// Account Aggregator callbacks.
	OnConsentUpdate func(n *aa.ConsentNotification)
	OnSessionUpdate func(n *aa.SessionNotification)

	// BBPS BillCollect settlement callback.
	OnBBPSSettlement func(n *bbps.SettlementNotification)

	// Logger is used for structured error/warn logging. Defaults to slog.Default().
	Logger *slog.Logger
}

// Handler is a unified http.Handler for all Setu webhook events.
type Handler struct {
	cfg Config
	log *slog.Logger
}

// NewHandler builds an http.Handler that routes incoming Setu webhooks to
// the corresponding callback in cfg.
func NewHandler(cfg Config) *Handler {
	l := cfg.Logger
	if l == nil {
		l = slog.Default()
	}
	return &Handler{cfg: cfg, log: l}
}

// ServeHTTP implements [http.Handler].
// Always responds with 200 OK to acknowledge receipt.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.log.Error("webhook: read body", "error", err)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	defer r.Body.Close() //nolint:errcheck

	h.dispatch(body)
	w.WriteHeader(http.StatusOK)
}

// typeProbe extracts the top-level type/eventType fields without full decoding.
type typeProbe struct {
	Type      string `json:"type"`
	EventType string `json:"eventType"`
}

func (h *Handler) dispatch(body []byte) {
	var probe typeProbe
	if err := json.Unmarshal(body, &probe); err != nil {
		h.log.Warn("webhook: unmarshal type probe", "error", err)
		return
	}

	// Account Aggregator notifications.
	switch aa.NotificationType(probe.Type) {
	case aa.NotificationTypeConsent:
		if h.cfg.OnConsentUpdate == nil {
			return
		}
		n, err := aa.ParseConsentNotification(body)
		if err != nil {
			h.log.Error("webhook: parse consent notification", "error", err)
			return
		}
		h.cfg.OnConsentUpdate(n)
		return

	case aa.NotificationTypeSession:
		if h.cfg.OnSessionUpdate == nil {
			return
		}
		n, err := aa.ParseSessionNotification(body)
		if err != nil {
			h.log.Error("webhook: parse session notification", "error", err)
			return
		}
		h.cfg.OnSessionUpdate(n)
		return
	}

	// BBPS settlement notification (has "events" array, no eventType).
	if probe.Type == "" && probe.EventType == "" {
		// Attempt BBPS parse.
		if h.cfg.OnBBPSSettlement != nil {
			var n bbps.SettlementNotification
			if err := json.Unmarshal(body, &n); err == nil && len(n.Events) > 0 {
				h.cfg.OnBBPSSettlement(&n)
				return
			}
		}
	}

	// UPI Setu notifications (use eventType field).
	et := EventType(probe.EventType)
	switch {
	case et == EventPaymentInitiated || et == EventPaymentPending ||
		et == EventPaymentSuccess || et == EventPaymentFailed:
		if h.cfg.OnPaymentUpdate == nil {
			return
		}
		var n PaymentNotification
		if err := json.Unmarshal(body, &n); err != nil {
			h.log.Error("webhook: parse payment notification", "error", err)
			return
		}
		h.cfg.OnPaymentUpdate(&n)

	case et == EventMandateInitiated || et == EventMandateLive ||
		et == EventMandateRejected || et == EventMandatePaused ||
		et == EventMandateRevoked || et == EventMandateUpdated ||
		et == EventMandateOpCreateInitiated || et == EventMandateOpCreateSuccess ||
		et == EventMandateOpCreateFailed || et == EventMandateOpUpdateInitiated ||
		et == EventMandateOpUpdateSuccess || et == EventMandateOpUpdateFailed ||
		et == EventMandateOpRevokeInitiated || et == EventMandateOpRevokeSuccess ||
		et == EventMandateOpRevokeFailed || et == EventMandateOpExecuteSuccess ||
		et == EventMandateOpExecuteFailed || et == EventMandateOpNotifySuccess ||
		et == EventMandateOpNotifyFailed:
		if h.cfg.OnMandateUpdate == nil {
			return
		}
		var n MandateNotification
		if err := json.Unmarshal(body, &n); err != nil {
			h.log.Error("webhook: parse mandate notification", "error", err)
			return
		}
		h.cfg.OnMandateUpdate(&n)

	case et == EventRefundPending || et == EventRefundSuccessful:
		if h.cfg.OnRefundUpdate == nil {
			return
		}
		var n RefundNotification
		if err := json.Unmarshal(body, &n); err != nil {
			h.log.Error("webhook: parse refund notification", "error", err)
			return
		}
		h.cfg.OnRefundUpdate(&n)

	case et == EventDisputeCreated || et == EventDisputeOpen ||
		et == EventDisputeClosed || et == EventDisputeInReview ||
		et == EventDisputeWon || et == EventDisputeLost:
		if h.cfg.OnDisputeUpdate == nil {
			return
		}
		var n DisputeNotification
		if err := json.Unmarshal(body, &n); err != nil {
			h.log.Error("webhook: parse dispute notification", "error", err)
			return
		}
		h.cfg.OnDisputeUpdate(&n)

	default:
		h.log.Warn("webhook: unknown event", "type", probe.Type, "eventType", probe.EventType)
	}
}
