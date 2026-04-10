// Package setu provides a production-grade Go SDK for the Setu APIs.
//
// This SDK supports the full Setu product suite including:
//
//   - UPI Setu (UMAP)
//     Flash/DQR/SQR payment links
//     Collect (VPA push payments)
//     TPV (Third Party Validation)
//     Refunds and Disputes
//     UPI Mandates (Recurring, One-time, Single Block Multi-Debit)
//
//   - BBPS
//     BillCollect (biller-side APIs)
//     BillPay (agent-side bill payment)
//
//   - WhatsApp Collect
//     Send payment reminders via WhatsApp.
//
//   - Account Aggregator (AA)
//     FIU consent flow
//     Data fetch
//     Multi-consent management
//
//   - KYC APIs
//     PAN Verification
//     Bank Account Verification (BAV)
//     GST / GSTIN Verification
//     DigiLocker
//     Aadhaar eKYC
//     Name Match
//
//   - Aadhaar eSign
//     Create signing session
//     Retrieve signed documents
//
// # Quick Start
//
//	client, err := setu.New(
//	    setu.WithClientID("your-client-id"),
//	    setu.WithClientSecret("your-client-secret"),
//	    setu.WithEnvironment(setu.Sandbox),
//	)
//
//	if err != nil {
//	    panic(err)
//	}
//
//	resp, err := client.Payments.UPI.CreatePaymentLink(ctx, req)
//
// # Error Handling
//
// All errors implement [setuerrors.Error] and support:
//
//	errors.As()
//	setuerrors.IsNotFound()
//	setuerrors.IsRateLimit()
//	setuerrors.GetTraceID()
//
// # Architecture
//
// The SDK is organized into two main product groups:
//
//   - Payments
//   - Data
//
// Each product group exposes specialized sub-clients.
//
//	client.Payments.UPI
//	client.Payments.BBPS
//	client.Data.KYC.PAN
//	client.Data.AA
//
// This design keeps the API strongly typed and scalable.
package setu
