// Package types defines shared domain types used across the Setu SDK.
package types

import "time"

// Environment is the Setu API deployment target.
type Environment string

const (
	// Sandbox is the testing environment. No real money movement.
	Sandbox Environment = "sandbox"
	// Production is the live environment.
	Production Environment = "production"
)

// Currency is an ISO 4217 currency code.
type Currency string

const (
	// INR is Indian National Rupee, the only currency supported by Setu.
	INR Currency = "INR"
)

// FailureReason carries NPCI error details from failed UPI transactions.
type FailureReason struct {
	Code         string `json:"code,omitempty"`
	Description  string `json:"desc,omitempty"`
	NPCIErrCode  string `json:"npciErrCode,omitempty"`
	NPCIErrDesc  string `json:"npciErrDesc,omitempty"`
	NPCIRespCode string `json:"npciRespCode,omitempty"`
	NPCIRespDesc string `json:"npciRespDesc,omitempty"`
}

// DateRange represents a start/end datetime window.
type DateRange struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
}

// Address is a structured postal address returned in KYC responses.
type Address struct {
	HouseNumber string `json:"house,omitempty"`
	Street      string `json:"street,omitempty"`
	Locality    string `json:"locality,omitempty"`
	District    string `json:"district,omitempty"`
	State       string `json:"state,omitempty"`
	PinCode     string `json:"pin,omitempty"`
	Country     string `json:"country,omitempty"`
}
