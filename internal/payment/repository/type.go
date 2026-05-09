// Package repository defines the persistence contract for payment-service.
package repository

import (
	"context"

	"github.com/farid/payment-service/internal/payment/model"
)

type PaymentRepository interface {
	// Insert creates a PENDING payment. Caller passes an outbox event payload
	// for `payment.intent.created.v1`. Returns ErrConflict if pg_reference is
	// already taken (UNIQUE).
	Insert(ctx context.Context, p *model.Payment, eventPayload []byte) (*model.Payment, error)

	GetByID(ctx context.Context, id string) (*model.Payment, error)
	GetByPgReference(ctx context.Context, ref string) (*model.Payment, error)
	// GetByInvoiceID returns the most recent payment for an invoice.
	// Used by CreateQrisIntent to short-circuit when an active intent exists.
	GetByInvoiceID(ctx context.Context, invoiceID string) (*model.Payment, error)

	// MarkSettled atomically flips PENDING → PAID/FAILED and writes the outbox
	// event. Idempotent on already-terminal rows: returns the existing row
	// without writing a duplicate event.
	MarkSettled(ctx context.Context, pgReference string, status model.PaymentStatus, eventType string, eventPayload []byte) (*model.Payment, error)
}

// OutboxRepository drives the publisher worker.
type OutboxRepository interface {
	FetchUnpublished(ctx context.Context, limit int) ([]OutboxRow, error)
	MarkPublished(ctx context.Context, ids []int64) error
}

type OutboxRow struct {
	ID        int64  `db:"id"`
	EventType string `db:"event_type"`
	Payload   []byte `db:"payload"`
}
