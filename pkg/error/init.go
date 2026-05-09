// Package error provides domain-level error types that map cleanly to gRPC
// status codes via the grpcserver package's error interceptor.
package error

import "errors"

// AppError is a typed business error.
type AppError struct {
	Code    string // application code: DOUBLE_BOOK, INVALID_STATE, NOT_FOUND, ...
	Message string
	Cause   error
}

func (e *AppError) Error() string {
	if e.Cause != nil {
		return e.Code + ": " + e.Message + ": " + e.Cause.Error()
	}
	return e.Code + ": " + e.Message
}

func (e *AppError) Unwrap() error { return e.Cause }

// Sentinel errors used across the codebase.
var (
	ErrNotFound          = &AppError{Code: "NOT_FOUND", Message: "resource not found"}
	ErrConflict          = &AppError{Code: "CONFLICT", Message: "concurrent modification"}
	ErrDoubleBook        = &AppError{Code: "DOUBLE_BOOK", Message: "spot already reserved for overlapping window"}
	ErrInvalidState      = &AppError{Code: "INVALID_STATE", Message: "invalid state transition"}
	ErrLockUnavailable   = &AppError{Code: "LOCK_UNAVAILABLE", Message: "spot temporarily locked"}
	ErrIdempotencyReplay = &AppError{Code: "IDEMPOTENCY_REPLAY", Message: "request replayed"}
	ErrUnauthenticated   = &AppError{Code: "UNAUTHENTICATED", Message: "missing or invalid credentials"}
	ErrUpstreamDown      = &AppError{Code: "UPSTREAM_DOWN", Message: "dependent service unavailable"}
)

// Is helper for errors.Is comparisons by Code.
func Is(err, target error) bool {
	var a, b *AppError
	if !errors.As(err, &a) || !errors.As(target, &b) {
		return errors.Is(err, target)
	}
	return a.Code == b.Code
}
