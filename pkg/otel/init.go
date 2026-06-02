package otel

import (
	"context"
	"log"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func NewOpenTelemetry(endpoint, appName, appEnv string) *OpenTelemetry {
	if endpoint == "" {
		log.Println("otel: OTLP_ENDPOINT not set, telemetry disabled")
		return &OpenTelemetry{}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := grpc.NewClient(endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Printf("otel: dial collector failed: %v (continuing without telemetry)", err)
		return &OpenTelemetry{}
	}

	res, _ := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(appName),
			semconv.DeploymentEnvironment(appEnv),
		),
	)

	// Traces
	traceExp, err := otlptracegrpc.New(ctx, otlptracegrpc.WithGRPCConn(conn))
	if err != nil {
		log.Printf("otel: trace exporter init failed: %v (continuing without traces)", err)
		return &OpenTelemetry{}
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExp),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(0.1))),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))

	// Metrics
	metricExp, err := otlpmetricgrpc.New(ctx, otlpmetricgrpc.WithGRPCConn(conn))
	if err != nil {
		log.Printf("otel: metric exporter init failed: %v (continuing without metrics)", err)
		return &OpenTelemetry{tp: tp}
	}

	mp := metric.NewMeterProvider(
		metric.WithResource(res),
		metric.WithReader(metric.NewPeriodicReader(metricExp, metric.WithInterval(15*time.Second))),
	)
	otel.SetMeterProvider(mp)

	return &OpenTelemetry{tp: tp, mp: mp}
}
