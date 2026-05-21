package utils

import (
	"context"

	"google.golang.org/grpc/metadata"
)

// IdempotencyKeyFromCtx extracts the Idempotency-Key from gRPC metadata.
// Returns empty string if absent.
func IdempotencyKeyFromCtx(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	if v := md.Get(HeaderIdempotencyKey); len(v) > 0 {
		return v[0]
	}
	return ""
}

// DriverIDFromCtx returns the driver id injected by the gateway after JWT verify.
func DriverIDFromCtx(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	if v := md.Get(HeaderDriverID); len(v) > 0 {
		return v[0]
	}
	return ""
}

// CtxWithIdempotencyKey appends the key to outgoing metadata; useful for client-side calls.
func CtxWithIdempotencyKey(ctx context.Context, key string) context.Context {
	return metadata.AppendToOutgoingContext(ctx, HeaderIdempotencyKey, key)
}
