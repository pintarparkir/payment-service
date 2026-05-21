// Package logger provides structured logging helpers.
package logger

import (
	"context"
	"os"

	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// consoleStdout returns a writer pointing to os.Stdout (kept as helper to ease testing).
func consoleStdout() zapcore.WriteSyncer { return zapcore.AddSync(os.Stdout) }

// fieldsFromMap converts a generic map into zap fields and adds trace_id/span_id
// when the context carries an OpenTelemetry span.
func fieldsFromMap(ctx interface{}, m map[string]interface{}) []zap.Field {
	out := make([]zap.Field, 0, len(m)+2)
	if c, ok := ctx.(context.Context); ok {
		if span := trace.SpanFromContext(c); span.SpanContext().IsValid() {
			out = append(out,
				zap.String("trace_id", span.SpanContext().TraceID().String()),
				zap.String("span_id", span.SpanContext().SpanID().String()),
			)
		}
	}
	for k, v := range m {
		out = append(out, zap.Any(k, v))
	}
	return out
}
