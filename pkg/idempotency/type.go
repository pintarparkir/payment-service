package idempotency

import (
	"context"
	"time"
)

// StoreInterface persists request→response pairs keyed by (scope, key).
type StoreInterface interface {
	// Get returns (payload, found, err). If found, callers replay the cached response.
	Get(ctx context.Context, scope, key string) ([]byte, bool, error)
	// Put writes payload with a TTL.
	Put(ctx context.Context, scope, key string, payload []byte, ttl time.Duration) error
}
