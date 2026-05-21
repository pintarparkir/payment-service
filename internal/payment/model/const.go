// Package model defines payment domain models and constants.
package model

// gRPC method full names (idempotency interceptor whitelist).
const (
	ScopeCreateQrisIntent = "/parkirpintar.payment.v1.PaymentService/CreateQrisIntent"
)

// Routing keys we publish on parkirpintar.events.
const (
	EvtPaymentPaid   = "payment.paid.v1"
	EvtPaymentFailed = "payment.failed.v1"
)
