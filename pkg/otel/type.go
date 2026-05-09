package otel

import "go.opentelemetry.io/otel/sdk/trace"

// OpenTelemetry holds the SDK provider lifecycle so callers can shutdown cleanly.
type OpenTelemetry struct {
	tp *trace.TracerProvider
}
