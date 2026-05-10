// payment-service entry point.
//
// Wires:
//   - configs → logger → otel
//   - postgres + rabbitmq publisher
//   - Midtrans HTTP client (real or stub based on MIDTRANS_STUB_MODE)
//   - repository, usecase
//   - REST server: /v1/payments/qris/intent (mini app)
//                  /v1/payments/webhook/midtrans (Midtrans callback)
//                  /v1/payments/{id} (mini app)
//   - background outbox publisher
//
// gRPC server registration is conditional on the proto being generated; the
// .pb.go files are committed in api/proto/payment/v1/ so it just works.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"

	pmtgrpc "github.com/farid/payment-service/internal/payment/handler/grpc"
	pmthttp "github.com/farid/payment-service/internal/payment/handler/http"
	"github.com/farid/payment-service/internal/payment/model"
	pmtrepo "github.com/farid/payment-service/internal/payment/repository/postgres"
	pmtuc "github.com/farid/payment-service/internal/payment/usecase"
	"github.com/farid/payment-service/internal/payment/worker"

	"github.com/farid/payment-service/pkg/configs"
	pgdb "github.com/farid/payment-service/pkg/db/postgres"
	"github.com/farid/payment-service/pkg/grpcserver"
	"github.com/farid/payment-service/pkg/idempotency"
	"github.com/farid/payment-service/pkg/logger"
	"github.com/farid/payment-service/pkg/midtrans"
	pkgOtel "github.com/farid/payment-service/pkg/otel"
	"github.com/farid/payment-service/pkg/rabbit"
)

func main() {
	cfg := configs.NewConfig(configs.ConfigLoader{Env: os.Getenv("PROJECT_ENV")})
	if err := logger.NewLogger(cfg.AppName, cfg.AppEnv); err != nil {
		panic(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	otel := pkgOtel.NewOpenTelemetry(cfg.OTLPEndpoint, "payment", cfg.AppEnv)
	defer func() { _ = otel.EndAPM() }()

	// ── Infra ────────────────────────────────────────────────────────────────
	db, err := pgdb.NewPostgresDB(pgdb.PostgresDsn{
		Host: cfg.DbHost, Port: cfg.DbPort, User: cfg.DbUsername, Password: cfg.DbPassword, Db: cfg.DbName,
		MaxOpen: cfg.DbMaxOpen, MaxIdle: cfg.DbMaxIdle,
	})
	if err != nil {
		logger.Fatal(ctx, "postgres init failed", map[string]interface{}{logger.ErrorKey: err.Error()})
	}
	defer db.Close()

	pub, err := rabbit.NewPublisher(cfg.RabbitURL, cfg.RabbitExchange)
	if err != nil {
		logger.Fatal(ctx, "rabbitmq publisher init failed", map[string]interface{}{logger.ErrorKey: err.Error()})
	}
	defer pub.Close()

	// ── Midtrans client ──────────────────────────────────────────────────────
	var mt midtrans.Client
	if cfg.MidtransStubMode {
		mt = midtrans.NewStubClient()
		logger.Info(ctx, "midtrans: STUB mode (no real API calls)", nil)
	} else {
		mt = midtrans.NewHTTPClient(cfg.MidtransBaseURL, cfg.MidtransServerKey)
		logger.Info(ctx, "midtrans: HTTP client", map[string]interface{}{"base_url": cfg.MidtransBaseURL})
	}

	// ── Domain wiring ────────────────────────────────────────────────────────
	repo := pmtrepo.NewPaymentRepository(db)
	obRepo := pmtrepo.NewOutboxRepository(db)
	uc := pmtuc.NewPaymentUsecase(repo, mt)

	// ── gRPC server (s2s — CreateQrisIntent / GetPayment) ───────────────────
	grpcSrv, err := grpcserver.NewGrpcServer(cfg.GrpcPort, grpcserver.Options{
		IdempotencyStore:  idempotency.NewPostgresStore(db),
		IdempotentMethods: []string{model.SCOPE_CREATE_QRIS_INTENT},
	})
	if err != nil {
		logger.Fatal(ctx, "grpc server init failed", map[string]interface{}{logger.ErrorKey: err.Error()})
	}
	pmtgrpc.Register(grpcSrv.Server, uc)
	go func() {
		if err := grpcSrv.Start(); err != nil {
			logger.Fatal(ctx, "grpc serve failed", map[string]interface{}{logger.ErrorKey: err.Error()})
		}
	}()

	// ── Background workers ───────────────────────────────────────────────────
	go worker.NewOutboxPublisher(obRepo, pub).Run(ctx)

	// ── HTTP server ──────────────────────────────────────────────────────────
	if cfg.AppEnv == "local" {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}
	router := gin.New()
	router.Use(gin.Recovery(), cors.Default())
	router.GET("/healthz", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })
	pmthttp.RegisterPaymentHandler(router.Group("/v1"), uc, cfg.SuperAppJWTPubKey, cfg.MidtransWebhookSecret)

	httpSrv := &http.Server{
		Addr:              ":" + cfg.AppPort,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		logger.Info(ctx, fmt.Sprintf("payment HTTP listening on :%s", cfg.AppPort), nil)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal(ctx, "http listen failed", map[string]interface{}{logger.ErrorKey: err.Error()})
		}
	}()

	// ── Graceful shutdown ────────────────────────────────────────────────────
	<-ctx.Done()
	logger.Info(context.Background(), "shutdown signal received", nil)
	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(shutCtx)
	grpcSrv.Shutdown()
	_ = logger.Sync()
}
