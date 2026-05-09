package utils

// Header keys — consumed by interceptors and middleware.
const (
	HEADER_IDEMPOTENCY_KEY = "Idempotency-Key"
	HEADER_AUTHORIZATION   = "Authorization"
	HEADER_TRACE_ID        = "X-Trace-Id"
	HEADER_DRIVER_ID       = "X-Driver-Id"
)

// Default RPC budgets.
const (
	DEFAULT_RPC_TIMEOUT_MS = 800
	INBOUND_RPC_TIMEOUT_MS = 2000
)
