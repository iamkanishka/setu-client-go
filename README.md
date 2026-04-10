# setu-client-go

Production-grade Go SDK for the [Setu API Platform](https://docs.setu.co).  
**Go 1.25+ · Zero mandatory external deps (only x/time) · No lint issues**

---

## Products covered

### Payments
| Product | Methods |
|---------|---------|
| **UPI Flash — Dynamic QR** | `CreateDQR`, `GetDQR` |
| **UPI Flash — Static QR** | `CreateSQR`, `GetSQR` |
| **UPI Payment history** | `GetLastPayment`, `GetPaymentHistory` |
| **UPI Collect** (⚠️ NPCI deprecation pending) | `VerifyVPA`, `CreateCollect`, `GetCollect` |
| **UPI TPV** (SEBI-mandated for capital markets) | `CreateTPV`, `GetTPV` |
| **UPI Mandates — Recurring** | `CreateMandate` (`frequency: "monthly"`, etc.) |
| **UPI Mandates — One-time** | `CreateMandate` (`frequency: "one time"`) |
| **UPI Mandates — SBMD** | `CreateMandate` (`blockFunds: true`, `purpose: "76"`) |
| **Mandate Operations** | `UpdateMandate`, `RevokeMandate`, `GetMandateOperation` |
| **Pre-debit Notification** | `PreDebitNotify`, `GetPreDebitNotification` |
| **Mandate Execution** | `ExecuteMandate`, `GetMandateExecution` |
| **UPI Refunds** | `CreateRefund`, `GetRefund` |
| **UPI Disputes** | `GetDispute`, `AcceptDispute`, `RejectDispute` |
| **Merchant Onboarding** (aggregator) | `CreateMerchant`, `GetMerchant`, `CheckVPAAvailability`, `CreateVPA` |
| **BBPS BillCollect** | Biller-side types + `GetTransaction` |
| **BBPS BillPay** | `FetchBill`, `PayBill` |
| **WhatsApp Collect** | `SendReminder`, `GetReminderStatus` |

### Data
| Product | Methods |
|---------|---------|
| **Account Aggregator** | `CreateConsent`, `GetConsent`, `RevokeConsent`, `CreateMultiConsent`, `GetLastFetchStatus`, `ListDataSessions`, `CreateDataSession`, `GetDataSession`, `FetchFIData` |
| **PAN Verification** | `Verify` |
| **Bank Account Verification** | `VerifySync`, `VerifyAsync`, `GetAsyncStatus` |
| **GSTIN Verification** | `Verify` |
| **DigiLocker** | `CreateSession`, `GetSession`, `GetDocument` |
| **Aadhaar eKYC** | `Create`, `Get` |
| **Name Match** | `Match` (optimistic + pessimistic) |
| **Aadhaar eSign** | `Create`, `Get`, `Download` |

### Webhooks (unified handler)
All event types from UPI, AA, and BBPS in one `http.Handler`:
- `payment.initiated/pending/success/failed`
- `mandate.initiated/live/rejected/paused/revoked/updated`
- `mandate_operation.create/update/revoke/execute/notify.*`
- `refund.pending/successful`
- `dispute_created/open/closed/in_review/won/lost`
- `CONSENT_STATUS_UPDATE`, `SESSION_STATUS_UPDATE`
- `BILL_SETTLEMENT_STATUS`

---

## Installation

```bash
go get github.com/iamkanishka/setu-client-go
```

**Requires Go 1.25.0+.**

---

## Quick Start

```go
package main

import (
    "context"
    "log"
    "github.com/iamkanishka/setu-client-go"
    "github.com/iamkanishka/setu-client-go/data/kyc/pan"
)

func main() {
    client, err := setu.New(
        setu.WithClientID("your-client-id"),
        setu.WithClientSecret("your-client-secret"),
        setu.WithEnvironment(setu.Sandbox),
        setu.WithProductInstanceID("your-product-instance-id"),
    )
    if err != nil { log.Fatal(err) }

    res, err := client.Data.KYC.PAN.Verify(context.Background(), &pan.VerifyRequest{
        PAN:     "ABCDE1234A",
        Consent: "Y",
        Reason:  "Customer identity verification during onboarding",
    })
    if err != nil { log.Fatal(err) }
    log.Printf("valid=%v name=%q", res.IsValid(), res.Data.FullName)
}
```

---

## Configuration

```go
client, err := setu.New(
    // Required
    setu.WithClientID("..."),
    setu.WithClientSecret("..."),
    // Recommended
    setu.WithProductInstanceID("..."),      // For KYC / AA APIs
    setu.WithEnvironment(setu.Production),  // Default: Sandbox
    // Tuning
    setu.WithTimeout(30 * time.Second),     // Default: 30s
    setu.WithMaxAttempts(4),                // 1 + 3 retries
    setu.WithRetryWait(500*time.Millisecond, 10*time.Second),
    setu.WithRateLimit(100, 20),            // 100 RPS, burst 20
    // Advanced
    setu.WithHTTPTransport(myTransport),
    setu.WithUserAgent("my-app/1.0"),
)
```

---

## UPI Flash — Dynamic & Static QR

```go
// Dynamic QR (single-use, amount fixed or flexible)
dqr, _ := client.Payments.UPI.CreateDQR(ctx, merchantID, &upi.CreateDQRRequest{
    Amount:      10000,   // ₹100 in paise
    MerchantVPA: "merchant@pineaxis",
    ReferenceID: "ORDER-001",
})
// dqr.IntentLink → render as QR code image

// Static QR (permanent, multi-use, for in-store display)
sqr, _ := client.Payments.UPI.CreateSQR(ctx, merchantID, &upi.CreateSQRRequest{
    MerchantVPA: "shop@pineaxis",
    ReferenceID: "STORE-001",
})

// Check last payment on any link
payment, _ := client.Payments.UPI.GetLastPayment(ctx, merchantID, dqr.ID)
history, _ := client.Payments.UPI.GetPaymentHistory(ctx, merchantID, sqr.ID)
```

---

## UPI TPV (SEBI-mandated for capital markets)

```go
tpv, _ := client.Payments.UPI.CreateTPV(ctx, merchantID, &upi.CreateTPVRequest{
    Amount:      50000,       // ₹500
    MerchantVPA: "mf@pineaxis",
    CustomerAccount: upi.CustomerAccount{
        IFSC:          "HDFC0001234",
        AccountNumber: "9876543210",
    },
    ReferenceID: "MF-INV-001",
})
// Payment is accepted only from the registered account
```

---

## UPI Mandates

```go
// Recurring mandate
mandate, _ := client.Payments.UPI.CreateMandate(ctx, merchantID, &upi.CreateMandateRequest{
    Frequency: "monthly", Purpose: "14", BlockFunds: false,
    AllowMultipleDebit: false, /* … */
})

// One-time mandate (e.g. IPO reservation)
mandate, _ := client.Payments.UPI.CreateMandate(ctx, merchantID, &upi.CreateMandateRequest{
    Frequency: "one time", RecurrenceValue: 0, /* … */
})

// Single Block Multi-Debit (cash-on-delivery / try-and-buy)
mandate, _ := client.Payments.UPI.CreateMandate(ctx, merchantID, &upi.CreateMandateRequest{
    Frequency: "as presented", Purpose: "76",
    BlockFunds: true, AllowMultipleDebit: true, /* … */
})

// Pre-debit notification (required 48–72h before execution)
notif, _ := client.Payments.UPI.PreDebitNotify(ctx, merchantID, mandateID, &upi.PreDebitNotifyRequest{
    Amount: 1000, ExecutionDate: "15042025", UMN: mandate.UMN,
    SequenceNumber: 1, MerchantReferenceID: "EXEC-001",
})

// Execute after notification succeeds
exec, _ := client.Payments.UPI.ExecuteMandate(ctx, merchantID, mandateID, &upi.ExecuteMandateRequest{
    Amount: 1000, UMN: mandate.UMN,
    SequenceNumber: 1, MerchantReferenceID: "EXEC-001",
})

// Update / Revoke
client.Payments.UPI.UpdateMandate(ctx, merchantID, mandateID, &upi.UpdateMandateRequest{
    AmountLimit: 2000, EndDate: "01012028",
})
client.Payments.UPI.RevokeMandate(ctx, merchantID, mandateID, &upi.RevokeMandateRequest{
    MerchantReferenceID: "REVOKE-001",
})
```

---

## Disputes

```go
dispute, _ := client.Payments.UPI.GetDispute(ctx, merchantID, disputeID)

// Accept → automatic refund
client.Payments.UPI.AcceptDispute(ctx, merchantID, disputeID)

// Reject → provide evidence to NPCI
client.Payments.UPI.RejectDispute(ctx, merchantID, disputeID, &upi.RejectDisputeRequest{
    Evidence:     base64PDF,
    EvidenceType: "invoice",
    Remarks:      "Service rendered as agreed",
})
```

---

## Account Aggregator

```go
// Create consent (redirect customer to consent.URL)
consent, _ := client.Data.AA.CreateConsent(ctx, &aa.CreateConsentRequest{
    VUA: "9999999999",
    FetchType: aa.FetchTypePeriodic,
    ConsentDuration: &aa.Duration{Unit: aa.DurationUnitMonth, Value: 4},
    ConsentMode: aa.ConsentModeStore,
    ConsentTypes: []aa.ConsentType{aa.ConsentTypeTransactions, aa.ConsentTypeProfile},
    FITypes: []aa.FIType{aa.FITypeDeposit, aa.FITypeMutualFunds},
    Purpose: aa.PurposeLoanUnderwriting,
    DataRange: &aa.DateRange{From: from, To: to},
    DataLife: &aa.Duration{Unit: aa.DurationUnitMonth, Value: 1},
    Frequency: &aa.Frequency{Unit: aa.FrequencyUnitMonthly, Value: 1},
    RedirectURL: "https://yourapp.com/callback",
    Context: []aa.ContextParam{
        aa.WithAccountType("SAVINGS"),
        aa.WithAccountSelectionMode("multi"),
    },
    Tags: []string{"loan_q2"},
})

// After consent is ACTIVE, create data session
session, _ := client.Data.AA.CreateDataSession(ctx, &aa.CreateDataSessionRequest{
    ConsentID: consent.ID,
    Format:    aa.DataFormatJSON,
})

// After session is COMPLETED, fetch FI data
data, _ := client.Data.AA.FetchFIData(ctx, session.ID)
```

---

## Webhook Handler

```go
h := webhook.NewHandler(webhook.Config{
    OnPaymentUpdate: func(n *webhook.PaymentNotification) {
        log.Printf("payment %s → %s (type: %s)", n.ID, n.Status, n.ProductInstanceType)
    },
    OnMandateUpdate: func(n *webhook.MandateNotification) {
        log.Printf("mandate event: %s", n.EventType)
    },
    OnRefundUpdate: func(n *webhook.RefundNotification) {
        log.Printf("refund %s → %s", n.Data.ID, n.Data.Status)
    },
    OnDisputeUpdate: func(n *webhook.DisputeNotification) {
        log.Printf("dispute %s → %s", n.Data.ID, n.Data.Status)
    },
    OnConsentUpdate: func(n *aa.ConsentNotification) {
        log.Printf("consent %s → %s", n.ConsentID, n.Data.Status)
        if n.Data.Status == aa.ConsentStatusActive {
            // start data session
        }
    },
    OnSessionUpdate: func(n *aa.SessionNotification) {
        if n.Data.Status == aa.SessionStatusCompleted {
            // fetch FI data
        }
    },
    OnBBPSSettlement: func(n *bbps.SettlementNotification) {
        log.Printf("BBPS settlement: %d events", len(n.Events))
    },
})
http.Handle("/webhooks/setu", h)
```

---

## Error Handling

```go
result, err := client.Data.KYC.PAN.Verify(ctx, req)
if err != nil {
    if setuerrors.IsNotFound(err)     { /* PAN not in NSDL */ }
    if setuerrors.IsRateLimit(err)    { /* back off */ }
    if setuerrors.IsUnauthorized(err) { /* check credentials */ }

    traceID := setuerrors.GetTraceID(err) // for Setu support ticket

    var setuErr setuerrors.Error
    if errors.As(err, &setuErr) {
        log.Printf("status=%d code=%q retryable=%v trace=%s",
            setuErr.HTTPStatus(), setuErr.Code(),
            setuErr.Retryable(), setuErr.TraceID())
    }
}
```

---

## Environments

| Constant | Notes |
|----------|-------|
| `setu.Sandbox` | Default. No real money. Sandbox credentials only. |
| `setu.Production` | Real transactions. Production credentials required. |

Credentials are **environment-specific** — never swap sandbox and production keys.

---

## Project Structure

```
setu-client-go/
├── setu.go                          Root client: New(), Client, sub-client wiring
├── options.go                       WithClientID, WithEnvironment, WithRateLimit, …
├── go.mod                           module github.com/iamkanishka/setu-client-go · go 1.25
│
├── internal/
│   ├── auth/tokenmanager.go         Token cache, auto-refresh, singleflight dedup
│   ├── transport/client.go          HTTP: retry, rate-limit, body replay, error decode
│   ├── retry/retry.go               Exponential backoff + full-jitter (math/rand/v2)
│   └── ratelimit/ratelimit.go       Token-bucket (x/time/rate)
│
├── pkg/
│   ├── setuerrors/errors.go         APIError, AuthError, RateLimitError, NetworkError, ValidationError
│   └── types/types.go               Currency, Environment, FailureReason, DateRange
│
├── payments/
│   ├── upi/client.go                Flash/DQR/SQR, Collect, TPV, Mandates (3 types),
│   │                                Mandate ops, Pre-debit, Execution, Refunds, Disputes,
│   │                                Merchant onboarding
│   ├── bbps/client.go               BillCollect: biller types, settlement, transaction lookup
│   ├── billpay/client.go            BillPay agent: FetchBill, PayBill
│   └── whatsapp/client.go           SendReminder, GetReminderStatus
│
├── data/
│   ├── aa/client.go                 AA FIU: full consent + data session flows
│   │                                + AA webhook notification types
│   ├── esign/client.go              Aadhaar eSign: Create, Get, Download
│   └── kyc/
│       ├── pan/client.go            PAN verify (NSDL)
│       ├── bav/client.go            Bank account verify (sync + async)
│       ├── gst/client.go            GSTIN verify
│       ├── digilocker/client.go     DigiLocker: session + document fetch
│       ├── ekyc/client.go           Aadhaar eKYC
│       └── namematch/client.go      Name match (optimistic + pessimistic)
│
├── webhook/handler.go               Unified dispatcher (UPI + AA + BBPS)
└── examples/main.go                 Full runnable demo
```

---

## License

MIT © 2026 Kanishka Naik
