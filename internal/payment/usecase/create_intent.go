package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/farid/payment-service/internal/payment/model"
	apperror "github.com/farid/payment-service/pkg/error"
)

func (u *paymentUsecase) CreateQrisIntent(ctx context.Context, invoiceID string, amountIDR int64) (*QrisIntent, error) {
	if strings.TrimSpace(invoiceID) == "" {
		return nil, &apperror.AppError{Code: "VALIDATION", Message: "invoice_id required"}
	}
	if amountIDR <= 0 {
		return nil, &apperror.AppError{Code: "VALIDATION", Message: "amount_idr must be positive"}
	}

	// Idempotency at the row level: if a recent PENDING intent for this invoice
	// exists, reuse it instead of opening a second Midtrans transaction.
	if existing, err := u.repo.GetByInvoiceID(ctx, invoiceID); err == nil && existing.Status == model.PaymentPending {
		return &QrisIntent{
			PaymentID:   existing.ID,
			PgReference: existing.PgReference,
			// Note: we don't persist the QRIS payload string, so an idempotent
			// replay returns just the pg_reference. Mini-app fetches the QR
			// payload via a separate /payments/{id} call when needed.
		}, nil
	} else if err != nil && !errors.Is(err, apperror.ErrNotFound) {
		return nil, err
	}

	// Call Midtrans /charge.
	res, err := u.midtrans.Charge(ctx, invoiceID, amountIDR)
	if err != nil {
		return nil, &apperror.AppError{Code: "UPSTREAM_DOWN", Message: "midtrans charge failed", Cause: err}
	}

	payload, err := json.Marshal(map[string]any{
		"invoice_id":   invoiceID,
		"pg_reference": res.TransactionID,
		"amount_idr":   amountIDR,
	})
	if err != nil {
		return nil, fmt.Errorf("payment: marshal intent payload: %w", err)
	}
	created, err := u.repo.Insert(ctx, &model.Payment{
		InvoiceID:   invoiceID,
		Method:      "QRIS",
		Status:      model.PaymentPending,
		PgReference: res.TransactionID,
		AmountIDR:   amountIDR,
	}, payload)
	if err != nil {
		// If the unique constraint on pg_reference fired, Midtrans returned the
		// same transaction_id we already have — treat as replay.
		if errors.Is(err, apperror.ErrConflict) {
			if found, gerr := u.repo.GetByPgReference(ctx, res.TransactionID); gerr == nil {
				return &QrisIntent{
					PaymentID:   found.ID,
					QrisPayload: res.QrisPayload,
					PgReference: found.PgReference,
					ExpiresAt:   res.ExpiresAt,
				}, nil
			}
		}
		return nil, err
	}

	return &QrisIntent{
		PaymentID:   created.ID,
		QrisPayload: res.QrisPayload,
		PgReference: created.PgReference,
		ExpiresAt:   res.ExpiresAt,
	}, nil
}

func (u *paymentUsecase) GetPayment(ctx context.Context, id string) (*model.Payment, error) {
	return u.repo.GetByID(ctx, id)
}
