// Package aa provides the Setu Account Aggregator FIU API client.
//
// Three flows:
//  1. Consent — Create, Get, Revoke, Multi-Consent.
//  2. Data Fetch — Create Data Session, Get Status, Fetch FI Data.
//  3. Notifications — parse CONSENT_STATUS_UPDATE and SESSION_STATUS_UPDATE webhooks.
//
// Setu docs: https://docs.setu.co/data/account-aggregator
package aa

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/iamkanishka/setu-client-go/internal/transport"
	"github.com/iamkanishka/setu-client-go/pkg/setuerrors"
)

// Client is the Account Aggregator FIU API client.
type Client struct {
	tc      *transport.Client
	baseURL string
	headers map[string]string
}

// New creates a [*Client].
func New(tc *transport.Client, baseURL string, headers map[string]string) *Client {
	return &Client{tc: tc, baseURL: baseURL, headers: headers}
}

// ── Enumerations ──────────────────────────────────────────────────────────

// ConsentStatus is the lifecycle state of an AA consent request.
type ConsentStatus string

const (
	ConsentStatusPending   ConsentStatus = "PENDING"
	ConsentStatusInitiated ConsentStatus = "INITIATED" // FIP-selector multi-AA flow
	ConsentStatusActive    ConsentStatus = "ACTIVE"
	ConsentStatusRejected  ConsentStatus = "REJECTED"
	ConsentStatusRevoked   ConsentStatus = "REVOKED"
	ConsentStatusPaused    ConsentStatus = "PAUSED"
	ConsentStatusExpired   ConsentStatus = "EXPIRED"
)

// SessionStatus is the combined status of an AA data session.
type SessionStatus string

const (
	SessionStatusPending   SessionStatus = "PENDING"
	SessionStatusPartial   SessionStatus = "PARTIAL"
	SessionStatusCompleted SessionStatus = "COMPLETED"
	SessionStatusExpired   SessionStatus = "EXPIRED"
	SessionStatusFailed    SessionStatus = "FAILED"
)

// ConsentMode defines what the FIU will do with fetched data.
type ConsentMode string

const (
	ConsentModeView   ConsentMode = "VIEW"
	ConsentModeStore  ConsentMode = "STORE"
	ConsentModeQuery  ConsentMode = "QUERY"
	ConsentModeStream ConsentMode = "STREAM"
)

// FetchType specifies whether data fetches are one-time or periodic.
type FetchType string

const (
	FetchTypeOnetime  FetchType = "ONETIME"
	FetchTypePeriodic FetchType = "PERIODIC"
)

// FIType is a financial information data type.
type FIType string

const (
	FITypeDeposit          FIType = "DEPOSIT"
	FITypeTermDeposit      FIType = "TERM_DEPOSIT"
	FITypeRecurringDeposit FIType = "RECURRING_DEPOSIT"
	FITypeMutualFunds      FIType = "MUTUAL_FUNDS"
	FITypeInsurance        FIType = "INSURANCE_POLICIES"
	FITypeETF              FIType = "ETF"
	FITypeEquities         FIType = "EQUITIES"
	FITypeBonds            FIType = "BONDS"
	FITypeDebentures       FIType = "DEBENTURES"
	FITypeNPS              FIType = "NPS"
	FITypeEPF              FIType = "EPF"
	FITypePPF              FIType = "PPF"
	FITypeSIP              FIType = "SIP"
	FITypeGovtSecurities   FIType = "GOVT_SECURITIES"
	FITypeREIT             FIType = "REIT"
	FITypeINVIT            FIType = "INVIT"
	FITypeAIF              FIType = "AIF"
	FITypeCIS              FIType = "CIS"
	FITypeGSTR             FIType = "GSTR1_3B"
	FITypeULIP             FIType = "ULIP"
	FITypeCD               FIType = "CD"
	FITypeIDR              FIType = "IDR"
	FITypeCP               FIType = "CP"
)

// ConsentType specifies the category of FI data requested.
type ConsentType string

const (
	ConsentTypeProfile      ConsentType = "PROFILE"
	ConsentTypeSummary      ConsentType = "SUMMARY"
	ConsentTypeTransactions ConsentType = "TRANSACTIONS"
)

// PurposeCode is the regulatory purpose code for data access.
type PurposeCode string

const (
	PurposeLoanUnderwriting         PurposeCode = "101"
	PurposeSpendingPatterns         PurposeCode = "102"
	PurposeAggregatedStatement      PurposeCode = "103"
	PurposeMonitorAccounts          PurposeCode = "104"
	PurposeOneTimeConsentDataAccess PurposeCode = "105"
)

// DurationUnit is the time unit for consent/data-life durations.
type DurationUnit string

const (
	DurationUnitDay   DurationUnit = "DAY"
	DurationUnitMonth DurationUnit = "MONTH"
	DurationUnitYear  DurationUnit = "YEAR"
	DurationUnitInf   DurationUnit = "INF"
)

// FrequencyUnit is the time unit for data-fetch frequency.
type FrequencyUnit string

const (
	FrequencyUnitHourly  FrequencyUnit = "HOURLY"
	FrequencyUnitDaily   FrequencyUnit = "DAILY"
	FrequencyUnitMonthly FrequencyUnit = "MONTHLY"
	FrequencyUnitYearly  FrequencyUnit = "YEARLY"
)

// DataFormat controls the format of FI data returned.
type DataFormat string

const (
	DataFormatJSON DataFormat = "json"
	DataFormatXML  DataFormat = "xml"
)

// ── Shared structures ─────────────────────────────────────────────────────

// Duration is a time span with a unit and numeric value.
// Note: Setu serialises Value as a JSON string; MarshalJSON handles this.
type Duration struct {
	Unit  DurationUnit `json:"unit"`
	Value int          `json:"value,string"`
}

// DateRange is a start/end datetime window for consent validity or data fetch.
type DateRange struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
}

// Frequency specifies the maximum data-fetch cadence.
// Maximum: 1 per HOUR (24 per DAY).
type Frequency struct {
	Unit  FrequencyUnit `json:"unit"`
	Value int           `json:"value"`
}

// ContextParam is a key-value pair customising the consent UI flow.
type ContextParam struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// DataFilter restricts which records the FIP returns.
type DataFilter struct {
	Type     string `json:"type"`
	Operator string `json:"operator"` // ">", "<", ">=", "<="
	Value    string `json:"value"`
}

// LinkedAccount is a financial account linked during the consent flow.
type LinkedAccount struct {
	MaskedAccNumber string `json:"maskedAccNumber"`
	AccType         string `json:"accType"`
	FIPId           string `json:"fipId"`
	FIType          string `json:"fiType"`
	LinkRefNumber   string `json:"linkRefNumber"`
}

// ConsentUsage records how many times data has been fetched.
type ConsentUsage struct {
	Count    string     `json:"count"`
	LastUsed *time.Time `json:"lastUsed,omitempty"`
}

// ── Consent Flow ──────────────────────────────────────────────────────────

// createConsentBody is the internal JSON shape sent to the API.
type createConsentBody struct {
	VUA              string         `json:"vua"`
	ConsentDuration  *Duration      `json:"consentDuration,omitempty"`
	ConsentDateRange *DateRange     `json:"consentDateRange,omitempty"`
	ConsentMode      ConsentMode    `json:"consentMode,omitempty"`
	FetchType        FetchType      `json:"fetchType"`
	ConsentTypes     []ConsentType  `json:"consentTypes"`
	FITypes          []FIType       `json:"fiTypes"`
	Purpose          PurposeCode    `json:"purpose,omitempty"`
	DataRange        *DateRange     `json:"dataRange,omitempty"`
	DataLife         *Duration      `json:"dataLife,omitempty"`
	Frequency        *Frequency     `json:"frequency,omitempty"`
	DataFilter       []DataFilter   `json:"dataFilter,omitempty"`
	RedirectURL      string         `json:"redirectUrl,omitempty"`
	Context          []ContextParam `json:"context,omitempty"`
	AdditionalParams map[string]any `json:"additionalParams,omitempty"`
}

// CreateConsentRequest is the input for [Client.CreateConsent].
type CreateConsentRequest struct {
	// VUA: mobile number "9999999999" or handle "9999999999@onemoney".
	// Bare mobile → Setu selects best-performing AA (FIP Selector flow).
	VUA string
	// ConsentDuration or ConsentDateRange must be set (not both).
	ConsentDuration  *Duration
	ConsentDateRange *DateRange
	ConsentMode      ConsentMode
	FetchType        FetchType
	ConsentTypes     []ConsentType
	FITypes          []FIType
	Purpose          PurposeCode
	// DataRange is mandatory when ConsentTypes includes TRANSACTIONS.
	DataRange   *DateRange
	DataLife    *Duration
	Frequency   *Frequency
	DataFilter  []DataFilter
	RedirectURL string
	Context     []ContextParam
	// Tags for analytics segmentation. Must be pre-created at product-instance level.
	Tags []string
}

// CreateConsentResponse is returned by [Client.CreateConsent].
type CreateConsentResponse struct {
	ID          string         `json:"id"`
	URL         string         `json:"url"`
	Status      ConsentStatus  `json:"status"`
	Detail      map[string]any `json:"detail,omitempty"`
	RedirectURL string         `json:"redirectUrl,omitempty"`
	Usage       ConsentUsage   `json:"usage,omitempty"`
	Tags        []string       `json:"tags,omitempty"`
	TraceID     string         `json:"traceId"`
}

// GetConsentResponse is returned by [Client.GetConsent].
type GetConsentResponse struct {
	ID             string          `json:"id"`
	URL            string          `json:"url"`
	Status         ConsentStatus   `json:"status"`
	Detail         map[string]any  `json:"detail,omitempty"`
	AccountsLinked []LinkedAccount `json:"accountsLinked,omitempty"`
	Usage          ConsentUsage    `json:"usage,omitempty"`
	Tags           []string        `json:"tags,omitempty"`
	TraceID        string          `json:"traceId"`
}

// CreateConsent creates a new AA consent request.
// Redirect the customer to the returned URL for approval.
//
//	POST /consents
func (c *Client) CreateConsent(ctx context.Context, req *CreateConsentRequest) (*CreateConsentResponse, error) {
	if err := validateCreateConsent(req); err != nil {
		return nil, err
	}
	body := createConsentBody{
		VUA:              req.VUA,
		ConsentDuration:  req.ConsentDuration,
		ConsentDateRange: req.ConsentDateRange,
		ConsentMode:      req.ConsentMode,
		FetchType:        req.FetchType,
		ConsentTypes:     req.ConsentTypes,
		FITypes:          req.FITypes,
		Purpose:          req.Purpose,
		DataRange:        req.DataRange,
		DataLife:         req.DataLife,
		Frequency:        req.Frequency,
		DataFilter:       req.DataFilter,
		RedirectURL:      req.RedirectURL,
		Context:          req.Context,
	}
	if len(req.Tags) > 0 {
		body.AdditionalParams = map[string]any{"tags": req.Tags}
	}
	httpReq, err := c.newReq(ctx, http.MethodPost, "/consents", body)
	if err != nil {
		return nil, err
	}
	var out CreateConsentResponse
	return &out, c.tc.DoJSON(httpReq, &out)
}

// GetConsent retrieves the status and details of a consent request.
// Pass expanded=true to receive the full consent configuration.
//
//	GET /consents/:id[?expanded=true]
func (c *Client) GetConsent(ctx context.Context, id string, expanded bool) (*GetConsentResponse, error) {
	if id == "" {
		return nil, setuerrors.NewValidationError("id", "consent ID is required")
	}
	path := "/consents/" + id
	if expanded {
		path += "?expanded=true"
	}
	httpReq, err := c.newReq(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var out GetConsentResponse
	return &out, c.tc.DoJSON(httpReq, &out)
}

// RevokeConsentResponse is returned by [Client.RevokeConsent].
type RevokeConsentResponse struct {
	Status  ConsentStatus `json:"status"`
	TraceID string        `json:"traceId"`
}

// RevokeConsent revokes an active consent on behalf of the customer.
//
//	POST /v2/consents/:id/revoke
func (c *Client) RevokeConsent(ctx context.Context, id string) (*RevokeConsentResponse, error) {
	if id == "" {
		return nil, setuerrors.NewValidationError("id", "consent ID is required")
	}
	httpReq, err := c.newReq(ctx, http.MethodPost, "/v2/consents/"+id+"/revoke", nil)
	if err != nil {
		return nil, err
	}
	var out RevokeConsentResponse
	return &out, c.tc.DoJSON(httpReq, &out)
}

// MultiConsentRequest is the input for [Client.CreateMultiConsent].
type MultiConsentRequest struct {
	// MandatoryConsents must have at least one entry.
	MandatoryConsents []string `json:"mandatoryConsents"`
	OptionalConsents  []string `json:"optionalConsents,omitempty"`
}

// MultiConsentResponse is returned by [Client.CreateMultiConsent].
type MultiConsentResponse struct {
	ConsentCollectionID string `json:"consentCollectionId"`
	URL                 string `json:"url"`
	TxnID               string `json:"txnid"`
	TraceID             string `json:"traceId"`
}

// CreateMultiConsent merges two existing consent requests into a single
// approval flow with unified OTP authentication.
//
//	POST /v2/consents/collection
func (c *Client) CreateMultiConsent(ctx context.Context, req *MultiConsentRequest) (*MultiConsentResponse, error) {
	if req == nil || len(req.MandatoryConsents) == 0 {
		return nil, setuerrors.NewValidationError("mandatoryConsents", "at least one mandatory consent ID is required")
	}
	httpReq, err := c.newReq(ctx, http.MethodPost, "/v2/consents/collection", req)
	if err != nil {
		return nil, err
	}
	var out MultiConsentResponse
	return &out, c.tc.DoJSON(httpReq, &out)
}

// LastFetchStatus is returned by [Client.GetLastFetchStatus].
type LastFetchStatus struct {
	LastFetchedAt   time.Time `json:"lastFetchedAt"`
	DataRange       DateRange `json:"dataRange"`
	LastFetchedFips []string  `json:"lastFetchedFips"`
	TraceID         string    `json:"traceId"`
}

// GetLastFetchStatus retrieves the timestamp and FIP list of the most recent
// data fetch for a consent.
//
//	GET /v2/consents/:id/fetch/status
func (c *Client) GetLastFetchStatus(ctx context.Context, consentID string) (*LastFetchStatus, error) {
	if consentID == "" {
		return nil, setuerrors.NewValidationError("consentID", "consent ID is required")
	}
	httpReq, err := c.newReq(ctx, http.MethodGet, "/v2/consents/"+consentID+"/fetch/status", nil)
	if err != nil {
		return nil, err
	}
	var out LastFetchStatus
	return &out, c.tc.DoJSON(httpReq, &out)
}

// DataSessionSummary is one entry in [ListDataSessionsResponse].
type DataSessionSummary struct {
	SessionID string        `json:"sessionId"`
	Status    SessionStatus `json:"status"`
	CreatedAt time.Time     `json:"created_at"`
}

// ListDataSessionsResponse is returned by [Client.ListDataSessions].
type ListDataSessionsResponse struct {
	ConsentID    string               `json:"consentId"`
	DataSessions []DataSessionSummary `json:"dataSessions"`
	TraceID      string               `json:"traceId"`
}

// ListDataSessions lists all non-expired data sessions for a consent.
//
//	GET /v2/consents/:id/data-sessions
func (c *Client) ListDataSessions(ctx context.Context, consentID string) (*ListDataSessionsResponse, error) {
	if consentID == "" {
		return nil, setuerrors.NewValidationError("consentID", "consent ID is required")
	}
	httpReq, err := c.newReq(ctx, http.MethodGet, "/v2/consents/"+consentID+"/data-sessions", nil)
	if err != nil {
		return nil, err
	}
	var out ListDataSessionsResponse
	return &out, c.tc.DoJSON(httpReq, &out)
}

// ── Data Fetch Flow ───────────────────────────────────────────────────────

// CreateDataSessionRequest is the input for [Client.CreateDataSession].
type CreateDataSessionRequest struct {
	ConsentID    string     `json:"consentId"`
	Format       DataFormat `json:"format,omitempty"`
	FIPDataRange *DateRange `json:"DataRange,omitempty"`
}

// CreateDataSessionResponse is returned by [Client.CreateDataSession].
type CreateDataSessionResponse struct {
	ID     string        `json:"id"`
	Status SessionStatus `json:"status"`
}

// CreateDataSession initiates a data fetch against an ACTIVE consent.
// Poll [Client.GetDataSession] or receive SESSION_STATUS_UPDATE webhook
// before calling [Client.FetchFIData].
//
//	POST /v2/sessions
func (c *Client) CreateDataSession(ctx context.Context, req *CreateDataSessionRequest) (*CreateDataSessionResponse, error) {
	if req == nil || req.ConsentID == "" {
		return nil, setuerrors.NewValidationError("consentId", "consentId is required")
	}
	if req.Format == "" {
		req.Format = DataFormatJSON
	}
	httpReq, err := c.newReq(ctx, http.MethodPost, "/v2/sessions", req)
	if err != nil {
		return nil, err
	}
	var out CreateDataSessionResponse
	return &out, c.tc.DoJSON(httpReq, &out)
}

// AccountFIStatus is the per-account data-readiness within a session.
type AccountFIStatus struct {
	FIStatus      string `json:"FIStatus"` // PENDING, READY, DELIVERED, TIMEOUT, DENIED
	Description   string `json:"description"`
	LinkRefNumber string `json:"linkRefNumber"`
}

// FIPStatus describes per-FIP readiness in a data session.
type FIPStatus struct {
	FIPID    string            `json:"fipID"`
	Accounts []AccountFIStatus `json:"accounts"`
}

// GetDataSessionResponse is returned by [Client.GetDataSession].
type GetDataSessionResponse struct {
	ID     string        `json:"id"`
	Status SessionStatus `json:"status"`
	FIPs   []FIPStatus   `json:"fips,omitempty"`
	Format DataFormat    `json:"format,omitempty"`
}

// GetDataSession retrieves the current status of a data session.
//
//	GET /v2/sessions/:id
func (c *Client) GetDataSession(ctx context.Context, sessionID string) (*GetDataSessionResponse, error) {
	if sessionID == "" {
		return nil, setuerrors.NewValidationError("sessionID", "session ID is required")
	}
	httpReq, err := c.newReq(ctx, http.MethodGet, "/v2/sessions/"+sessionID, nil)
	if err != nil {
		return nil, err
	}
	var out GetDataSessionResponse
	return &out, c.tc.DoJSON(httpReq, &out)
}

// FIDataResponse holds the raw FI data for a completed session.
type FIDataResponse struct {
	ConsentID     string           `json:"consentId"`
	DataSessionID string           `json:"dataSessionId"`
	Payload       []map[string]any `json:"payload"`
}

// FetchFIData retrieves FI data for a COMPLETED or PARTIAL data session.
//
//	GET /v2/sessions/:id/fetch
func (c *Client) FetchFIData(ctx context.Context, sessionID string) (*FIDataResponse, error) {
	if sessionID == "" {
		return nil, setuerrors.NewValidationError("sessionID", "session ID is required")
	}
	httpReq, err := c.newReq(ctx, http.MethodGet, "/v2/sessions/"+sessionID+"/fetch", nil)
	if err != nil {
		return nil, err
	}
	var out FIDataResponse
	return &out, c.tc.DoJSON(httpReq, &out)
}

// ── Webhook notification types ────────────────────────────────────────────

// NotificationType classifies AA webhook payloads.
type NotificationType string

const (
	NotificationTypeConsent NotificationType = "CONSENT_STATUS_UPDATE"
	NotificationTypeSession NotificationType = "SESSION_STATUS_UPDATE"
)

// ConsentErrorCode is a machine-readable reason for consent rejection.
type ConsentErrorCode string

const (
	ErrorCodeUserCancelled           ConsentErrorCode = "UserCancelled"
	ErrorCodeUserRejected            ConsentErrorCode = "UserRejected"
	ErrorCodeNoFIPAccountsDiscovered ConsentErrorCode = "NoFIPAccountsDiscovered"
	ErrorCodeFIPDenied               ConsentErrorCode = "FIPDenied"
)

// NotificationError carries structured error details in a failed notification.
type NotificationError struct {
	Code    ConsentErrorCode `json:"code"`
	Message string           `json:"message"`
}

// BaseNotification is the common AA webhook envelope.
// Unmarshal this first to determine Type, then decode the full payload.
type BaseNotification struct {
	Type           NotificationType   `json:"type"`
	ConsentID      string             `json:"consentId"`
	NotificationID string             `json:"notificationId,omitempty"`
	Timestamp      time.Time          `json:"timestamp"`
	Success        bool               `json:"success"`
	Error          *NotificationError `json:"error,omitempty"`
	Raw            json.RawMessage    `json:"-"`
}

// ConsentNotification is the full payload for CONSENT_STATUS_UPDATE.
type ConsentNotification struct {
	BaseNotification
	Data ConsentNotificationData `json:"data"`
}

// ConsentNotificationData holds consent-specific status and account details.
type ConsentNotificationData struct {
	Status ConsentStatus `json:"status"`
	// VUA may be updated with the AA handle in FIP-selector flows.
	VUA    string               `json:"vua,omitempty"`
	Detail *ConsentActiveDetail `json:"detail,omitempty"`
}

// ConsentActiveDetail holds linked accounts (present when Status is ACTIVE).
type ConsentActiveDetail struct {
	Accounts []LinkedAccount `json:"accounts"`
}

// SessionNotification is the full payload for SESSION_STATUS_UPDATE.
type SessionNotification struct {
	BaseNotification
	DataSessionID string                  `json:"dataSessionId"`
	Data          SessionNotificationData `json:"data"`
}

// SessionNotificationData holds session status and per-FIP readiness.
type SessionNotificationData struct {
	Status SessionStatus           `json:"status"`
	FIPs   []FIPNotificationStatus `json:"fips,omitempty"`
	Format DataFormat              `json:"format,omitempty"`
}

// FIPNotificationStatus is per-FIP readiness in a session notification.
type FIPNotificationStatus struct {
	FIPID    string                        `json:"fipID"`
	Accounts []AccountFINotificationStatus `json:"accounts"`
}

// AccountFINotificationStatus is per-account readiness in a notification.
type AccountFINotificationStatus struct {
	FIStatus      string `json:"FIStatus"`
	Description   string `json:"description"`
	LinkRefNumber string `json:"linkRefNumber"`
}

// ParseBaseNotification decodes the common AA notification envelope.
func ParseBaseNotification(body []byte) (*BaseNotification, error) {
	var n BaseNotification
	if err := json.Unmarshal(body, &n); err != nil {
		return nil, err
	}
	n.Raw = body
	return &n, nil
}

// ParseConsentNotification decodes a CONSENT_STATUS_UPDATE payload.
func ParseConsentNotification(body []byte) (*ConsentNotification, error) {
	var n ConsentNotification
	return &n, json.Unmarshal(body, &n)
}

// ParseSessionNotification decodes a SESSION_STATUS_UPDATE payload.
func ParseSessionNotification(body []byte) (*SessionNotification, error) {
	var n SessionNotification
	return &n, json.Unmarshal(body, &n)
}

// ── Context builder helpers ───────────────────────────────────────────────

// WithAccountType returns a ContextParam filtering by account type ("SAVINGS" or "CURRENT").
func WithAccountType(t string) ContextParam {
	return ContextParam{Key: "accounttype", Value: t}
}

// WithFIPFilter returns a ContextParam restricting consent to specific FIP IDs
// (comma-separated).
func WithFIPFilter(fipIDs string) ContextParam {
	return ContextParam{Key: "fipId", Value: fipIDs}
}

// WithExcludeFIPs returns a ContextParam excluding specific FIP IDs.
func WithExcludeFIPs(fipIDs string) ContextParam {
	return ContextParam{Key: "excludeFipIds", Value: fipIDs}
}

// WithAccountSelectionMode returns a ContextParam for account selection mode.
// Values: "single", "multi", "multi-opt-out".
func WithAccountSelectionMode(mode string) ContextParam {
	return ContextParam{Key: "accountSelectionMode", Value: mode}
}

// WithTransactionType returns a ContextParam for transaction type filter
// ("debit" or "credit").
func WithTransactionType(t string) ContextParam {
	return ContextParam{Key: "transactionType", Value: t}
}

// WithPurposeDescription returns a ContextParam with a custom consent purpose description.
func WithPurposeDescription(desc string) ContextParam {
	return ContextParam{Key: "purposeDescription", Value: desc}
}

// ── Helpers ───────────────────────────────────────────────────────────────

func (c *Client) newReq(ctx context.Context, method, path string, body any) (*http.Request, error) {
	req, err := transport.NewJSONRequest(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, err
	}
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}
	return req, nil
}

func validateCreateConsent(req *CreateConsentRequest) error {
	if req == nil {
		return setuerrors.NewValidationError("", "request is required")
	}
	if req.VUA == "" {
		return setuerrors.NewValidationError("vua", "VUA (mobile or handle) is required")
	}
	if req.ConsentDuration == nil && req.ConsentDateRange == nil {
		return setuerrors.NewValidationError("consentDuration", "consentDuration or consentDateRange is required")
	}
	if len(req.FITypes) == 0 {
		return setuerrors.NewValidationError("fiTypes", "at least one FI type is required")
	}
	if len(req.ConsentTypes) == 0 {
		return setuerrors.NewValidationError("consentTypes", "at least one consent type is required")
	}
	for _, ct := range req.ConsentTypes {
		if ct == ConsentTypeTransactions && req.DataRange == nil {
			return setuerrors.NewValidationError("dataRange",
				"dataRange is required when ConsentTypes includes TRANSACTIONS")
		}
	}
	return nil
}
