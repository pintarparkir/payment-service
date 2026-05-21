// Package http exposes payment-service over REST/JSON for two clients:
//   - Tencent Mini Program (JWT-authenticated) — POST /v1/payments/qris/intent
//   - Midtrans webhook callback (HMAC-verified) — POST /v1/payments/webhook/midtrans
package http

import (
	"time"

	"github.com/gin-gonic/gin"

	"github.com/farid/payment-service/internal/payment/usecase"
	"github.com/farid/payment-service/pkg/rate"
)

const ctxDriverID = "driver_id"

type paymentHandler struct {
	uc            usecase.PaymentUsecase
	jwtKey        string
	webhookSecret string
}

// RegisterPaymentHandler mounts mini-app + Midtrans-webhook routes under rg.
// jwtPubKeyPEM = super-app RS256 public key (empty skips check in dev).
// midtransSecret = MIDTRANS_WEBHOOK_SECRET (empty skips signature check in dev).
// lim may be nil; rate limiting is skipped when nil or on Redis errors (fail-open).
func RegisterPaymentHandler(rg *gin.RouterGroup, uc usecase.PaymentUsecase, jwtPubKeyPEM, midtransSecret string, lim rate.Limiter) {
	h := &paymentHandler{uc: uc, jwtKey: jwtPubKeyPEM, webhookSecret: midtransSecret}

	// Mini-app (JWT required).
	authed := rg.Group("/payments")
	authed.Use(h.jwtMiddleware())
	authed.POST("/qris/intent", rateLimitDriver(lim, "payments:qris-intent", 5, time.Minute), h.createQrisIntent)
	authed.GET("/:id", h.getPayment)

	// Midtrans webhook is public — secured by HMAC signature, not JWT.
	rg.POST("/payments/webhook/midtrans", h.midtransWebhook)
}
