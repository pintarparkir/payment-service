// Package utils provides shared utility constants and helpers.
package utils

// Header keys — consumed by interceptors and middleware.
const (
	HeaderIdempotencyKey = "Idempotency-Key"
	HeaderAuthorization  = "Authorization"
	HeaderTraceID        = "X-Trace-Id"
	HeaderDriverID       = "X-Driver-Id"
)

// Default RPC budgets.
const (
	DefaultRPCTimeoutMs = 800
	InboundRPCTimeoutMs = 2000
)
