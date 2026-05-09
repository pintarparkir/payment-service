package otel

import (
	"context"
	"time"
)

// EndAPM flushes pending spans and shuts down the provider gracefully.
func (o *OpenTelemetry) EndAPM() error {
	if o == nil || o.tp == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return o.tp.Shutdown(ctx)
}
