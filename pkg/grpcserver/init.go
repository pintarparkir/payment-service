// Package grpcserver bootstraps a gRPC server with the canonical interceptor stack:
// recovery → trace → log → idempotency. Every service uses NewGrpcServer to start.
package grpcserver

import (
	"context"
	"fmt"
	"net"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/keepalive"

	"github.com/farid/payment-service/pkg/idempotency"
	"github.com/farid/payment-service/pkg/logger"
)

// GrpcServer wraps the underlying *grpc.Server with lifecycle helpers.
type GrpcServer struct {
	Server   *grpc.Server
	listener net.Listener
	port     string
}

// Options control optional behaviour of the server.
type Options struct {
	IdempotencyStore idempotency.StoreInterface
	IdempotentMethods []string // FullMethod values to enforce idempotency on
	UnaryInterceptors []grpc.UnaryServerInterceptor
}

// NewGrpcServer constructs a *GrpcServer bound to :port with the standard
// interceptor stack. Add domain handlers via Server.RegisterService(...).
func NewGrpcServer(port string, opt Options) (*GrpcServer, error) {
	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return nil, fmt.Errorf("listen :%s: %w", port, err)
	}

	chain := []grpc.UnaryServerInterceptor{
		recoveryInterceptor(),
		loggingInterceptor(),
		timeoutInterceptor(2 * time.Second),
	}
	if opt.IdempotencyStore != nil {
		chain = append(chain, idempotencyInterceptor(opt.IdempotencyStore, opt.IdempotentMethods))
	}
	chain = append(chain, opt.UnaryInterceptors...)

	srv := grpc.NewServer(
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
		grpc.ChainUnaryInterceptor(chain...),
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle:     5 * time.Minute,
			MaxConnectionAge:      30 * time.Minute,
			MaxConnectionAgeGrace: 30 * time.Second,
			Time:                  10 * time.Second,
			Timeout:               5 * time.Second,
		}),
	)

	healthpb.RegisterHealthServer(srv, health.NewServer())

	return &GrpcServer{Server: srv, listener: listener, port: port}, nil
}

// Start serves requests until ctx is cancelled.
func (g *GrpcServer) Start() error {
	logger.Info(context.Background(), "gRPC server listening", map[string]interface{}{"port": g.port})
	return g.Server.Serve(g.listener)
}

// Shutdown gracefully drains in-flight RPCs.
func (g *GrpcServer) Shutdown() {
	g.Server.GracefulStop()
}
