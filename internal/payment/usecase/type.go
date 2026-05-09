// Package usecase orchestrates payment-domain business logic.
package usecase

import (
	"context"
	"time"

	"github.com/farid/payment-service/internal/payment/model"
	"github.com/farid/payment-service/internal/payment/repository"
	"github.com/farid/payment-service/pkg/midtrans"
)

type PaymentUsecase interface {
	// CreateQrisIntent calls Midtrans /charge for the given invoice and
	// persists a PENDING payment row. Idempotent on `(invoice_id, status=PENDING)`:
	// re-issuing for the same invoice while a non-expired pending intent exists
	// returns the existing intent unchanged.
	CreateQrisIntent(ctx context.Context, invoiceID string, amountIDR int64) (*QrisIntent, error)

	// HandleWebhook processes a verified Midtrans notification: flips the
	// payment row to PAID/FAILED and emits the corresponding outbox event.
	HandleWebhook(ctx context.Context, n *midtrans.Notification) error

	GetPayment(ctx context.Context, id string) (*model.Payment, error)
}

// QrisIntent is the response shape for CreateQrisIntent (REST + gRPC translate).
type QrisIntent struct {
	PaymentID   string
	QrisPayload string
	PgReference string
	ExpiresAt   time.Time
}

type paymentUsecase struct {
	repo     repository.PaymentRepository
	midtrans midtrans.Client
}

func NewPaymentUsecase(repo repository.PaymentRepository, mt midtrans.Client) PaymentUsecase {
	return &paymentUsecase{repo: repo, midtrans: mt}
}
