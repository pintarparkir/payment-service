package otel

import (
	"context"
	"time"
)

func (o *OpenTelemetry) EndAPM() error {
	if o == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if o.mp != nil {
		if err := o.mp.Shutdown(ctx); err != nil {
			return err
		}
	}
	if o.tp != nil {
		return o.tp.Shutdown(ctx)
	}
	return nil
}
