package otel

import (
	"go.opentelemetry.io/contrib/instrumentation/runtime"
)

func (o *OpenTelemetry) RegisterRuntimeMetrics() error {
	if o == nil || o.mp == nil {
		return nil
	}
	return runtime.Start(runtime.WithMinimumReadMemStatsInterval(15))
}
