// Package main demonstrates the complete Setu Go SDK.
//
// Run with:
//
//	export CLIENT_ID=your-id CLIENT_SECRET=your-secret PRODUCT_INSTANCE_ID=your-pid
//	export MERCHANT_ID=your-merchant-id   # optional for UPI demo
//	go run ./examples/
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/iamkanishka/setu-client-go"
	"github.com/iamkanishka/setu-client-go/data/aa"
	"github.com/iamkanishka/setu-client-go/data/kyc/bav"
	"github.com/iamkanishka/setu-client-go/data/kyc/gst"
	"github.com/iamkanishka/setu-client-go/data/kyc/namematch"
	"github.com/iamkanishka/setu-client-go/data/kyc/pan"
	"github.com/iamkanishka/setu-client-go/payments/upi"
	"github.com/iamkanishka/setu-client-go/payments/whatsapp"
	"github.com/iamkanishka/setu-client-go/pkg/setuerrors"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout,
		&slog.HandlerOptions{Level: slog.LevelInfo})))
	ctx := context.Background()

	// ── Build client ──────────────────────────────────────────────────────
	client, err := setu.New(
		setu.WithClientID(mustEnv("CLIENT_ID")),
		setu.WithClientSecret(mustEnv("CLIENT_SECRET")),
		setu.WithProductInstanceID(os.Getenv("PRODUCT_INSTANCE_ID")),
		setu.WithEnvironment(setu.Sandbox),
		setu.WithTimeout(30*time.Second),
		setu.WithMaxAttempts(4),
		setu.WithRetryWait(500*time.Millisecond, 10*time.Second),
		setu.WithRateLimit(50, 10),
	)
	must(err, "create client")
	fmt.Printf("✓  Setu client ready — env: %s\n\n", client.Environment())

	merchantID := os.Getenv("MERCHANT_ID")
	if merchantID == "" {
		merchantID = "sandbox-merchant-id"
	}

	// ── 1. PAN Verification ───────────────────────────────────────────────
	section("PAN Verification")
	panRes, panErr := client.Data.KYC.PAN.Verify(ctx, &pan.VerifyRequest{
		PAN:     "ABCDE1234A",
		Consent: "Y",
		Reason:  "Customer KYC for loan onboarding process",
	})
	if ok(panErr) {
		fmt.Printf("  valid=%-5v  name=%-22q  category=%q  aadhaar=%q\n",
			panRes.IsValid(), panRes.Data.FullName,
			panRes.Data.Category, panRes.Data.AadhaarSeedingStatus)
	}

	// ── 2. GST Verification ───────────────────────────────────────────────
	section("GST Verification")
	gstRes, gstErr := client.Data.KYC.GST.Verify(ctx, &gst.VerifyRequest{
		GSTIN: "27AAICB3918J1CT",
	})
	if ok(gstErr) {
		fmt.Printf("  gstin=%-18q  company=%-28q  active=%v\n",
			gstRes.Data.GST.ID, gstRes.Data.Company.Name, gstRes.IsActive())
	}

	// ── 3. Bank Account Verification (sync) ───────────────────────────────
	section("Bank Account Verification — Sync")
	bavRes, bavErr := client.Data.KYC.BAV.VerifySync(ctx, &bav.VerifySyncRequest{
		AccountNumber: "1234567890",
		IFSC:          "HDFC0001234",
		Name:          "Rahul Sharma",
	})
	if ok(bavErr) {
		fmt.Printf("  account_exists=%-5v  name_at_bank=%-20q  status=%q\n",
			bavRes.AccountExists, bavRes.NameAtBank, bavRes.Status)
	}

	// ── 4. Name Match ─────────────────────────────────────────────────────
	section("Name Match")
	nmRes, nmErr := client.Data.KYC.NameMatch.Match(ctx, &namematch.MatchRequest{
		Name1: "Rakesh Kumar Singh",
		Name2: "Rakesh K. Singh",
	})
	if ok(nmErr) {
		fmt.Printf("  optimistic=%.1f%% (%s)   pessimistic=%.1f%% (%s)   match@75%%=%v\n",
			nmRes.OptimisticMatchOutput.MatchPercentage,
			nmRes.OptimisticMatchOutput.MatchType,
			nmRes.PessimisticMatchOutput.MatchPercentage,
			nmRes.PessimisticMatchOutput.MatchType,
			nmRes.IsMatch(75.0))
	}

	// ── 5. Verify VPA ─────────────────────────────────────────────────────
	section("UPI — Verify VPA")
	valid, vpaErr := client.Payments.UPI.VerifyVPA(ctx, merchantID, "customer@okhdfcbank")
	if ok(vpaErr) {
		fmt.Printf("  vpa=customer@okhdfcbank  valid=%v\n", valid)
	}

	// ── 6. UPI Flash — Create Dynamic QR ─────────────────────────────────
	section("UPI Flash — Create Dynamic QR")
	dqr, dqrErr := client.Payments.UPI.CreateDQR(ctx, merchantID, &upi.CreateDQRRequest{
		Amount:          10000, // ₹100 in paise
		MerchantVPA:     "merchant@pineaxis",
		ReferenceID:     fmt.Sprintf("ORDER-%d", time.Now().Unix()),
		TransactionNote: "Payment for order #1001",
		Metadata:        map[string]any{"invoiceId": "INV-2025-001"},
	})
	if ok(dqrErr) {
		fmt.Printf("  dqr_id=%-28q  status=%q\n", dqr.ID, dqr.Status)
		fmt.Printf("  short_link: %s\n", dqr.ShortLink)

		// Poll last payment
		pay, payErr := client.Payments.UPI.GetLastPayment(ctx, merchantID, dqr.ID)
		if ok(payErr) {
			fmt.Printf("  last payment: status=%q  txnId=%q\n", pay.Status, pay.TxnID)
		}
	}

	// ── 7. UPI Flash — Create Static QR ──────────────────────────────────
	section("UPI Flash — Create Static QR")
	sqr, sqrErr := client.Payments.UPI.CreateSQR(ctx, merchantID, &upi.CreateSQRRequest{
		MerchantVPA:     "shop@pineaxis",
		ReferenceID:     "STORE-001",
		TransactionNote: "In-store payment",
	})
	if ok(sqrErr) {
		fmt.Printf("  sqr_id=%-28q  status=%q\n", sqr.ID, sqr.Status)
	}

	// ── 8. UPI TPV ────────────────────────────────────────────────────────
	section("UPI TPV — Third Party Validation")
	tpv, tpvErr := client.Payments.UPI.CreateTPV(ctx, merchantID, &upi.CreateTPVRequest{
		Amount:      50000, // ₹500
		MerchantVPA: "mf@pineaxis",
		CustomerAccount: upi.CustomerAccount{
			IFSC:          "HDFC0001234",
			AccountNumber: "9876543210",
		},
		ReferenceID:     fmt.Sprintf("MF-%d", time.Now().Unix()),
		TransactionNote: "Mutual fund investment",
	})
	if ok(tpvErr) {
		fmt.Printf("  tpv_id=%-28q  status=%q\n", tpv.ID, tpv.Status)
	}

	// ── 9. UPI Collect ────────────────────────────────────────────────────
	section("UPI Collect (VPA push) — ⚠️ NPCI deprecation pending")
	collect, colErr := client.Payments.UPI.CreateCollect(ctx, merchantID, &upi.CreateCollectRequest{
		Amount:              5000, // ₹50
		Currency:            "INR",
		CustomerVPA:         "customer@okhdfcbank",
		MerchantVPA:         "merchant@pineaxis",
		MerchantReferenceID: fmt.Sprintf("COLLECT-%d", time.Now().Unix()),
		ExpireAfter:         10,
		TransactionNote:     "Test collect",
	})
	if ok(colErr) {
		fmt.Printf("  collect_id=%-28q  status=%q\n", collect.ID, collect.Status)
	}

	// ── 10. UPI Mandate — Recurring ───────────────────────────────────────
	section("UPI Mandate — Recurring (Intent)")
	mandate, mErr := client.Payments.UPI.CreateMandate(ctx, merchantID, &upi.CreateMandateRequest{
		Amount:               1000,
		AmountRule:           "max",
		AllowMultipleDebit:   false,
		AutoExecute:          false,
		AutoPreNotify:        false,
		BlockFunds:           false,
		CreationMode:         "intent",
		Currency:             "INR",
		CustomerRevocable:    true,
		EndDate:              "01012030",
		ExpireAfter:          120,
		FirstExecutionAmount: 0,
		Frequency:            "monthly",
		InitiationMode:       "04",
		MaxAmountLimit:       1000,
		MerchantReferenceID:  fmt.Sprintf("MANDATE-%d", time.Now().Unix()),
		MerchantVPA:          "merchant@pineaxis",
		Purpose:              "14",
		RecurrenceRule:       "on",
		RecurrenceValue:      1,
		ShareToPayee:         false,
		StartDate:            "01012025",
		TransactionNote:      "Monthly subscription",
	})
	if ok(mErr) {
		fmt.Printf("  mandate_id=%-28q  status=%q\n", mandate.ID, mandate.Status)
		if mandate.IntentLink != "" {
			fmt.Printf("  intent_link (show as QR): %s…\n", mandate.IntentLink[:60])
		}
	}

	// ── 11. UPI Mandate — One-time ────────────────────────────────────────
	section("UPI Mandate — One-time")
	onetimeMandate, omErr := client.Payments.UPI.CreateMandate(ctx, merchantID, &upi.CreateMandateRequest{
		Amount:               2000,
		AmountRule:           "max",
		AllowMultipleDebit:   false,
		AutoExecute:          false,
		AutoPreNotify:        false,
		BlockFunds:           false,
		CreationMode:         "qr",
		Currency:             "INR",
		CustomerRevocable:    false,
		EndDate:              "01012026",
		ExpireAfter:          60,
		FirstExecutionAmount: 0,
		Frequency:            "one time",
		InitiationMode:       "01",
		MaxAmountLimit:       2000,
		MerchantReferenceID:  fmt.Sprintf("ONETIME-%d", time.Now().Unix()),
		MerchantVPA:          "merchant@pineaxis",
		Purpose:              "14",
		RecurrenceRule:       "on",
		RecurrenceValue:      0,
		ShareToPayee:         false,
		StartDate:            "01012025",
		TransactionNote:      "IPO reservation",
	})
	if ok(omErr) {
		fmt.Printf("  one_time_mandate_id=%q  status=%q\n", onetimeMandate.ID, onetimeMandate.Status)
	}

	// ── 12. UPI Mandate — SBMD (Single Block Multi-Debit) ─────────────────
	section("UPI Mandate — SBMD (Single Block Multi-Debit)")
	sbmd, sErr := client.Payments.UPI.CreateMandate(ctx, merchantID, &upi.CreateMandateRequest{
		Amount:               5000,
		AmountRule:           "max",
		AllowMultipleDebit:   true,
		AutoExecute:          false,
		AutoPreNotify:        false,
		BlockFunds:           true,
		CreationMode:         "intent",
		Currency:             "INR",
		CustomerRevocable:    false,
		EndDate:              "01012026",
		ExpireAfter:          120,
		FirstExecutionAmount: 0,
		Frequency:            "as presented",
		InitiationMode:       "04",
		MaxAmountLimit:       5000,
		MerchantReferenceID:  fmt.Sprintf("SBMD-%d", time.Now().Unix()),
		MerchantVPA:          "merchant@pineaxis",
		Purpose:              "76", // block purpose
		RecurrenceRule:       "on",
		RecurrenceValue:      0,
		ShareToPayee:         false,
		StartDate:            "01012025",
		TransactionNote:      "Cash on delivery reservation",
	})
	if ok(sErr) {
		fmt.Printf("  sbmd_id=%q  status=%q\n", sbmd.ID, sbmd.Status)
	}

	// ── 13. UPI Refund ────────────────────────────────────────────────────
	section("UPI Refund")
	refund, rErr := client.Payments.UPI.CreateRefund(ctx, merchantID, &upi.CreateRefundRequest{
		Amount:              5000,
		Currency:            "INR",
		MerchantReferenceID: fmt.Sprintf("REFUND-%d", time.Now().Unix()),
		PaymentID:           "01HKSEWQ509Z56CVQNQ2XHGJZ1", // from actual payment
		Remarks:             "Customer cancellation",
		Type:                "online",
	})
	if ok(rErr) {
		fmt.Printf("  refund_id=%q  status=%q\n", refund.ID, refund.Status)
	}

	// ── 14. Account Aggregator — Create Consent ───────────────────────────
	section("Account Aggregator — Create Consent")
	from := time.Now().AddDate(-1, 0, 0)
	to := time.Now()
	consent, cErr := client.Data.AA.CreateConsent(ctx, &aa.CreateConsentRequest{
		VUA:       "9999999999",
		FetchType: aa.FetchTypeOnetime,
		ConsentDuration: &aa.Duration{
			Unit:  aa.DurationUnitMonth,
			Value: 1,
		},
		ConsentMode:  aa.ConsentModeStore,
		ConsentTypes: []aa.ConsentType{aa.ConsentTypeTransactions, aa.ConsentTypeProfile},
		FITypes:      []aa.FIType{aa.FITypeDeposit, aa.FITypeMutualFunds},
		Purpose:      aa.PurposeLoanUnderwriting,
		DataRange:    &aa.DateRange{From: from, To: to},
		DataLife:     &aa.Duration{Unit: aa.DurationUnitMonth, Value: 1},
		Frequency:    &aa.Frequency{Unit: aa.FrequencyUnitMonthly, Value: 1},
		RedirectURL:  "https://yourapp.com/aa/callback",
		Context: []aa.ContextParam{
			aa.WithAccountType("SAVINGS"),
			aa.WithAccountSelectionMode("multi"),
			aa.WithPurposeDescription("Loan underwriting for personal credit product"),
		},
		Tags: []string{"loan_onboarding", "q2_2025"},
	})
	if ok(cErr) {
		fmt.Printf("  consent_id=%q  status=%q\n", consent.ID, consent.Status)
		fmt.Printf("  redirect customer to: %s\n", consent.URL)

		// Poll consent
		statusRes, sErr2 := client.Data.AA.GetConsent(ctx, consent.ID, false)
		if ok(sErr2) {
			fmt.Printf("  polled consent status: %q\n", statusRes.Status)
		}
	}

	// ── 15. AA — Multi-Consent ────────────────────────────────────────────
	section("Account Aggregator — Multi Consent (merged flow)")
	// Requires two previously created consent IDs
	fmt.Println("  (skipped: requires two pre-created consent IDs)")

	// ── 16. WhatsApp Collect Reminder ─────────────────────────────────────
	section("WhatsApp Collect — Send Bill Reminder")
	waRes, waErr := client.Payments.WhatsApp.SendReminder(ctx, &whatsapp.SendReminderRequest{
		CustomerMobile: "9999999998",
		CustomerName:   "Rahul Sharma",
		BillAmount:     150000, // ₹1500 in paise
		BillerBillID:   "BILL-2025-001",
		DueDate:        time.Now().Add(7 * 24 * time.Hour),
		LanguageCode:   "en",
	})
	if ok(waErr) {
		fmt.Printf("  reminder_id=%q  delivery=%q\n", waRes.ID, waRes.DeliveryStatus)
	}

	fmt.Println("\n✓  All examples complete.")
}

// ── helpers ───────────────────────────────────────────────────────────────

func section(title string) { fmt.Printf("\n── %s ──\n", title) }

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		fmt.Fprintf(os.Stderr, "required env var %q is not set\n", key)
		os.Exit(1)
	}
	return v
}

func must(err error, op string) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal %s: %v\n", op, err)
		os.Exit(1)
	}
}

func ok(err error) bool {
	if err == nil {
		return true
	}
	var ve *setuerrors.ValidationError
	if errors.As(err, &ve) {
		fmt.Printf("  [validation] field=%q: %s\n", ve.Field, ve.Message)
		return false
	}
	traceID := setuerrors.GetTraceID(err)
	if traceID != "" {
		fmt.Printf("  [error] %v (traceId: %s)\n", err, traceID)
	} else {
		fmt.Printf("  [error] %v\n", err)
	}
	return false
}
