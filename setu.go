// Package setu is the root of the Setu Go SDK.
//
// # Products covered
//
//   - UPI Setu (UMAP): Flash/DQR/SQR payment links, Collect (VPA-push),
//     TPV (Third Party Validation), Refunds, Disputes, UPI Mandates
//     (Recurring, One-time, Single Block Multi-Debit), Mandate operations
//     (Update, Revoke, Pause, Unpause), Merchant onboarding for aggregators.
//   - BBPS BillCollect: biller-side bill-fetch/receipt APIs, settlement.
//   - BBPS BillPay: agent-side bill-fetch and bill-payment.
//   - WhatsApp Collect: send payment reminders via WhatsApp.
//   - Account Aggregator: full FIU consent + data-fetch flow, multi-consent,
//     revoke, last fetch status, data session management.
//   - PAN Verification (NSDL).
//   - Bank Account Verification — penny drop, sync and async.
//   - GST / GSTIN Verification.
//   - DigiLocker — consent session + document fetch.
//   - Aadhaar eKYC.
//   - Name Match — optimistic and pessimistic scoring.
//   - Aadhaar eSign — create, get, download signed PDF.
//
// # Quick start
//
//	client, err := setu.New(
//	    setu.WithClientID("your-client-id"),
//	    setu.WithClientSecret("your-client-secret"),
//	    setu.WithEnvironment(setu.Sandbox),
//	)
//
// # Error handling
//
// All errors implement [setuerrors.Error] and are compatible with [errors.As].
// Use [setuerrors.IsNotFound], [setuerrors.IsRateLimit], [setuerrors.GetTraceID], etc.
package setu

import (
	"fmt"
	"net/http"
	"time"

	"github.com/iamkanishka/setu-client-go/data/aa"
	"github.com/iamkanishka/setu-client-go/data/esign"
	"github.com/iamkanishka/setu-client-go/data/kyc/bav"
	"github.com/iamkanishka/setu-client-go/data/kyc/digilocker"
	"github.com/iamkanishka/setu-client-go/data/kyc/ekyc"
	"github.com/iamkanishka/setu-client-go/data/kyc/gst"
	"github.com/iamkanishka/setu-client-go/data/kyc/namematch"
	"github.com/iamkanishka/setu-client-go/data/kyc/pan"
	"github.com/iamkanishka/setu-client-go/internal/auth"
	"github.com/iamkanishka/setu-client-go/internal/ratelimit"
	"github.com/iamkanishka/setu-client-go/internal/retry"
	"github.com/iamkanishka/setu-client-go/internal/transport"
	"github.com/iamkanishka/setu-client-go/payments/bbps"
	"github.com/iamkanishka/setu-client-go/payments/billpay"
	"github.com/iamkanishka/setu-client-go/payments/upi"
	"github.com/iamkanishka/setu-client-go/payments/whatsapp"
	"github.com/iamkanishka/setu-client-go/pkg/setuerrors"
	"github.com/iamkanishka/setu-client-go/pkg/types"
)

// SDKVersion is the current semantic version of this SDK.
const SDKVersion = "1.0.2"

// Environment aliases — re-exported for convenience.
const (
	Sandbox    = types.Sandbox
	Production = types.Production
)

// envURLs holds the base-URL set for one deployment environment.
type envURLs struct {
	accountService string // UPI / BBPS auth endpoint
	umapAPI        string // UPI Setu (UMAP) REST host
	dataGateway    string // KYC / eSign host
	fiu            string // Account Aggregator FIU host
}

var environmentURLs = map[types.Environment]envURLs{
	types.Sandbox: {
		accountService: "https://accountservice.setu.co",
		umapAPI:        "https://umap.setu.co/api",
		dataGateway:    "https://dg-sandbox.setu.co",
		fiu:            "https://fiu-sandbox.setu.co",
	},
	types.Production: {
		accountService: "https://accountservice.setu.co",
		umapAPI:        "https://umap.setu.co/api",
		dataGateway:    "https://dg.setu.co",
		fiu:            "https://fiu.setu.co",
	},
}

// KYCClient groups identity and verification sub-clients.
type KYCClient struct {
	PAN        *pan.Client
	BAV        *bav.Client
	GST        *gst.Client
	DigiLocker *digilocker.Client
	EKYC       *ekyc.Client
	NameMatch  *namematch.Client
}

// DataClient groups all data-product sub-clients.
type DataClient struct {
	// AA is the Account Aggregator FIU client.
	AA *aa.Client
	// KYC groups all identity verification clients.
	KYC *KYCClient
	// ESign is the Aadhaar eSign client.
	ESign *esign.Client
}

// PaymentsClient groups all payments-product sub-clients.
type PaymentsClient struct {
	// UPI covers Flash/DQR/SQR, Collect, TPV, Mandates, Refunds, Disputes.
	UPI *upi.Client
	// BBPS covers BillCollect biller-side APIs.
	BBPS *bbps.Client
	// BillPay covers BBPS agent-side bill-payment APIs.
	BillPay *billpay.Client
	// WhatsApp covers WhatsApp Collect reminder APIs.
	WhatsApp *whatsapp.Client
}

// Client is the root Setu SDK client. Construct with [New].
// All sub-clients share the underlying transport, rate limiter, and token manager.
type Client struct {
	// Payments exposes all payments-product clients.
	Payments *PaymentsClient
	// Data exposes all data-product clients.
	Data *DataClient

	cfg config
}

type config struct {
	clientID          string
	clientSecret      string
	productInstanceID string
	environment       types.Environment
	timeout           time.Duration
	maxAttempts       int
	retryWaitBase     time.Duration
	retryWaitMax      time.Duration
	rateLimitRPS      float64
	rateLimitBurst    int
	httpTransport     http.RoundTripper
	userAgent         string
}

// New constructs a [*Client] using the provided options.
//
// Required: [WithClientID], [WithClientSecret].
//
//	client, err := setu.New(
//	    setu.WithClientID("your-id"),
//	    setu.WithClientSecret("your-secret"),
//	    setu.WithEnvironment(setu.Production),
//	    setu.WithProductInstanceID("your-pid"), // for KYC / AA
//	)
func New(opts ...Option) (*Client, error) {
	cfg := config{
		environment:    types.Sandbox,
		timeout:        30 * time.Second,
		maxAttempts:    4,
		retryWaitBase:  500 * time.Millisecond,
		retryWaitMax:   10 * time.Second,
		rateLimitRPS:   100,
		rateLimitBurst: 20,
		userAgent:      fmt.Sprintf("setu-client-go/%s", SDKVersion),
	}
	for _, o := range opts {
		o(&cfg)
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	urls, ok := environmentURLs[cfg.environment]
	if !ok {
		return nil, fmt.Errorf("setu: unknown environment %q", cfg.environment)
	}

	rl := ratelimit.New(cfg.rateLimitRPS, cfg.rateLimitBurst)
	rp := retry.New(retry.Config{
		MaxAttempts: cfg.maxAttempts,
		WaitBase:    cfg.retryWaitBase,
		WaitMax:     cfg.retryWaitMax,
	})

	var hc *http.Client
	if cfg.httpTransport != nil {
		hc = &http.Client{Transport: cfg.httpTransport, Timeout: cfg.timeout}
	}
	tc := transport.New(transport.Config{
		HTTPClient:  hc,
		RateLimiter: rl,
		RetryPolicy: rp,
		UserAgent:   cfg.userAgent,
	})

	tm := auth.NewTokenManager(auth.TokenManagerConfig{
		ClientID:     cfg.clientID,
		ClientSecret: cfg.clientSecret,
		BaseURL:      urls.accountService,
		HTTPClient:   tc,
	})

	kycHdrs := map[string]string{
		"x-client-id":           cfg.clientID,
		"x-client-secret":       cfg.clientSecret,
		"x-product-instance-id": cfg.productInstanceID,
	}

	return &Client{
		cfg: cfg,
		Payments: &PaymentsClient{
			UPI:      upi.New(tc, urls.umapAPI, tm),
			BBPS:     bbps.New(tc, urls.umapAPI, tm),
			BillPay:  billpay.New(tc, urls.umapAPI, tm),
			WhatsApp: whatsapp.New(tc, urls.umapAPI, tm),
		},
		Data: &DataClient{
			AA:    aa.New(tc, urls.fiu, kycHdrs),
			ESign: esign.New(tc, urls.dataGateway, kycHdrs),
			KYC: &KYCClient{
				PAN:        pan.New(tc, urls.dataGateway, kycHdrs),
				BAV:        bav.New(tc, urls.dataGateway, kycHdrs),
				GST:        gst.New(tc, urls.dataGateway, kycHdrs),
				DigiLocker: digilocker.New(tc, urls.dataGateway, kycHdrs),
				EKYC:       ekyc.New(tc, urls.dataGateway, kycHdrs),
				NameMatch:  namematch.New(tc, urls.dataGateway, kycHdrs),
			},
		},
	}, nil
}

// Environment returns the environment the client targets.
func (c *Client) Environment() types.Environment { return c.cfg.environment }

func (cfg *config) validate() error {
	if cfg.clientID == "" {
		return setuerrors.NewValidationError("clientID", "clientID is required (use WithClientID)")
	}
	if cfg.clientSecret == "" {
		return setuerrors.NewValidationError("clientSecret", "clientSecret is required (use WithClientSecret)")
	}
	if cfg.timeout <= 0 {
		return setuerrors.NewValidationError("timeout", "timeout must be positive")
	}
	return nil
}
