package otel

import (
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/trace"
)

type OpenTelemetry struct {
	tp *trace.TracerProvider
	mp *metric.MeterProvider
}
