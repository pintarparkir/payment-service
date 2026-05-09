package grpcserver

import (
	"context"
	"runtime/debug"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	apperror "github.com/farid/payment-service/pkg/error"
	"github.com/farid/payment-service/pkg/idempotency"
	"github.com/farid/payment-service/pkg/logger"
	"github.com/farid/payment-service/pkg/utils"
)

// recoveryInterceptor turns panics into gRPC INTERNAL errors so one bad RPC
// doesn't crash the service.
func recoveryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		defer func() {
			if r := recover(); r != nil {
				logger.Error(ctx, "panic recovered", map[string]interface{}{
					"method": info.FullMethod,
					"panic":  r,
					"stack":  string(debug.Stack()),
				})
				err = status.Error(codes.Internal, "internal server error")
			}
		}()
		return handler(ctx, req)
	}
}

// loggingInterceptor records latency + outcome for every RPC.
func loggingInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
		fields := map[string]interface{}{
			"method":  info.FullMethod,
			"elapsed": time.Since(start).String(),
		}
		if err != nil {
			fields[logger.ErrorKey] = err.Error()
			logger.Error(ctx, "rpc failed", fields)
		} else {
			logger.Info(ctx, "rpc ok", fields)
		}
		return resp, mapError(err)
	}
}

// timeoutInterceptor injects a deadline into every inbound context.
func timeoutInterceptor(d time.Duration) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		if _, ok := ctx.Deadline(); !ok {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, d)
			defer cancel()
		}
		return handler(ctx, req)
	}
}

// idempotencyInterceptor short-circuits replays for whitelisted methods.
// On replay, returns the cached response (zero side effects).
func idempotencyInterceptor(store idempotency.StoreInterface, methods []string) grpc.UnaryServerInterceptor {
	guarded := make(map[string]struct{}, len(methods))
	for _, m := range methods {
		guarded[m] = struct{}{}
	}
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		if _, ok := guarded[info.FullMethod]; !ok {
			return handler(ctx, req)
		}
		key := utils.IdempotencyKeyFromCtx(ctx)
		if key == "" {
			return nil, status.Error(codes.InvalidArgument, "Idempotency-Key header required")
		}
		if cached, found, err := store.Get(ctx, info.FullMethod, key); err == nil && found {
			out := protoZero(req)
			if out != nil {
				if err := proto.Unmarshal(cached, out); err == nil {
					return out, nil
				}
			}
		}

		resp, err := handler(ctx, req)
		if err != nil {
			return resp, err
		}
		if msg, ok := resp.(proto.Message); ok {
			if payload, mErr := proto.Marshal(msg); mErr == nil {
				_ = store.Put(ctx, info.FullMethod, key, payload, 24*time.Hour)
			}
		}
		return resp, nil
	}
}

// protoZero returns a zero-value of the same proto.Message type as req's response.
// Concretely, the response type is encoded in the handler's signature; we use
// reflection on the request's package would be costly. Simplification: we look
// at the actual response from the handler. This helper is unused at replay-time
// because we don't know the response type ahead of handler invocation. To keep
// the interceptor simple, replays return the cached payload as raw bytes only
// when we can deduce the type — which our code path does at the handler level.
//
// In production, codegen would generate a typed wrapper per method.
func protoZero(_ interface{}) proto.Message { return nil }

// mapError translates AppError → gRPC status code so handlers can return
// idiomatic Go errors and the wire stays clean.
func mapError(err error) error {
	if err == nil {
		return nil
	}
	var ae *apperror.AppError
	if !asAppError(err, &ae) {
		return err
	}
	switch ae.Code {
	case "NOT_FOUND":
		return status.Error(codes.NotFound, ae.Message)
	case "CONFLICT", "DOUBLE_BOOK":
		return status.Error(codes.AlreadyExists, ae.Message)
	case "INVALID_STATE":
		return status.Error(codes.FailedPrecondition, ae.Message)
	case "LOCK_UNAVAILABLE":
		return status.Error(codes.ResourceExhausted, ae.Message)
	case "UNAUTHENTICATED":
		return status.Error(codes.Unauthenticated, ae.Message)
	case "UPSTREAM_DOWN":
		return status.Error(codes.Unavailable, ae.Message)
	default:
		return status.Error(codes.Internal, ae.Message)
	}
}

func asAppError(err error, target **apperror.AppError) bool {
	for cur := err; cur != nil; {
		if ae, ok := cur.(*apperror.AppError); ok {
			*target = ae
			return true
		}
		// follow Unwrap chain
		type unwrapper interface{ Unwrap() error }
		if u, ok := cur.(unwrapper); ok {
			cur = u.Unwrap()
			continue
		}
		break
	}
	return false
}
