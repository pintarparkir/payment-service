// Package grpcclient holds dial helpers for outbound s2s calls.
// Each client adds OTel instrumentation + per-call timeout.
package grpcclient

import (
	"fmt"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const serviceConfig = `{
  "methodConfig": [{
    "name": [{"service": ""}],
    "retryPolicy": {
      "MaxAttempts": 3,
      "InitialBackoff": "0.1s",
      "MaxBackoff": "1s",
      "BackoffMultiplier": 2,
      "RetryableStatusCodes": ["UNAVAILABLE", "DEADLINE_EXCEEDED"]
    },
    "timeout": "3s"
  }]
}`

// Dial returns a *grpc.ClientConn with OTel propagation + insecure transport
// (for dev). Connection is established lazily on first call.
// In production, swap insecure for mTLS.
func Dial(addr string) (*grpc.ClientConn, error) {
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
		grpc.WithDefaultServiceConfig(serviceConfig),
	)
	if err != nil {
		return nil, fmt.Errorf("grpc dial %s: %w", addr, err)
	}
	return conn, nil
}
