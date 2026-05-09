package model

// gRPC method full names (idempotency interceptor whitelist).
const (
	SCOPE_CREATE_QRIS_INTENT = "/parkirpintar.payment.v1.PaymentService/CreateQrisIntent"
)

// Routing keys we publish on parkirpintar.events.
const (
	EvtPaymentPaid   = "payment.paid.v1"
	EvtPaymentFailed = "payment.failed.v1"
)
