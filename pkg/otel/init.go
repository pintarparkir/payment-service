package otel

import (
	"context"
	"log"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// NewOpenTelemetry initialises the global OTel TracerProvider with an OTLP/gRPC exporter.
// In local/dev where collector is absent, exporter errors are non-fatal (log only).
func NewOpenTelemetry(endpoint, appName, appEnv string) *OpenTelemetry {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := grpc.NewClient(endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Printf("otel: dial collector failed: %v (continuing without traces)", err)
		return &OpenTelemetry{}
	}
	exp, err := otlptracegrpc.New(ctx, otlptracegrpc.WithGRPCConn(conn))
	if err != nil {
		log.Printf("otel: exporter init failed: %v (continuing without traces)", err)
		return &OpenTelemetry{}
	}

	res, _ := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(appName),
			semconv.DeploymentEnvironment(appEnv),
		),
	)

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(0.1))),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	return &OpenTelemetry{tp: tp}
}
