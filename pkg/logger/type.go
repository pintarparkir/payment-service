package logger

import "go.uber.org/zap"

// LoggerInterface defines the structured logging contract used across services.
// Implementations must be goroutine-safe.
type LoggerInterface interface {
	Info(ctx interface{}, msg string, fields map[string]interface{})
	Warn(ctx interface{}, msg string, fields map[string]interface{})
	Error(ctx interface{}, msg string, fields map[string]interface{})
	Fatal(ctx interface{}, msg string, fields map[string]interface{})
	Sync() error
}

// ErrorKey is the canonical map key used to attach an error to a log entry.
const ErrorKey = "error"

// instance holds the package-level zap logger; populated by NewLogger.
var instance *zap.Logger
