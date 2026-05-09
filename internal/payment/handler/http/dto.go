package http

import (
	"time"

	"github.com/farid/payment-service/internal/payment/model"
	"github.com/farid/payment-service/internal/payment/usecase"
)

type createIntentReq struct {
	InvoiceID string `json:"invoice_id" binding:"required"`
	AmountIDR int64  `json:"amount_idr" binding:"required"`
}

type qrisIntentDTO struct {
	PaymentID   string    `json:"payment_id"`
	QrisPayload string    `json:"qris_payload,omitempty"`
	PgReference string    `json:"pg_reference"`
	ExpiresAt   time.Time `json:"expires_at,omitempty"`
}

func toIntentDTO(r *usecase.QrisIntent) *qrisIntentDTO {
	return &qrisIntentDTO{
		PaymentID:   r.PaymentID,
		QrisPayload: r.QrisPayload,
		PgReference: r.PgReference,
		ExpiresAt:   r.ExpiresAt,
	}
}

type paymentDTO struct {
	ID          string     `json:"id"`
	InvoiceID   string     `json:"invoice_id"`
	Method      string     `json:"method"`
	Status      string     `json:"status"`
	PgReference string     `json:"pg_reference"`
	AmountIDR   int64      `json:"amount_idr"`
	CreatedAt   time.Time  `json:"created_at"`
	PaidAt      *time.Time `json:"paid_at,omitempty"`
}

func toPaymentDTO(p *model.Payment) *paymentDTO {
	return &paymentDTO{
		ID: p.ID, InvoiceID: p.InvoiceID, Method: p.Method,
		Status: string(p.Status), PgReference: p.PgReference,
		AmountIDR: p.AmountIDR, CreatedAt: p.CreatedAt, PaidAt: p.PaidAt,
	}
}
