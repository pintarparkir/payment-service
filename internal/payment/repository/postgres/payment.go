package postgres

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"

	"github.com/farid/payment-service/internal/payment/model"
	"github.com/farid/payment-service/internal/payment/repository"
	apperror "github.com/farid/payment-service/pkg/error"
)

const codeUniqueViolation = "23505"

type paymentRepo struct{ db *sqlx.DB }

func NewPaymentRepository(db *sqlx.DB) repository.PaymentRepository {
	return &paymentRepo{db: db}
}

const insertPaymentSQL = `
INSERT INTO payment (invoice_id, method, status, pg_reference, amount_idr)
VALUES ($1, $2, 'PENDING', $3, $4)
RETURNING id, created_at
`

const insertOutboxSQL = `
INSERT INTO outbox_event (aggregate_type, aggregate_id, event_type, payload)
VALUES ('payment', $1, $2, $3)
`

// Insert creates a PENDING payment. Returns ErrConflict if pg_reference is
// already used (UNIQUE) — caller can fetch via GetByPgReference instead.
func (r *paymentRepo) Insert(ctx context.Context, p *model.Payment, eventPayload []byte) (*model.Payment, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	out := *p
	err = tx.QueryRowxContext(ctx, insertPaymentSQL,
		p.InvoiceID, "QRIS", p.PgReference, p.AmountIDR,
	).Scan(&out.ID, &out.CreatedAt)
	if err != nil {
		var pgErr *pq.Error
		if errors.As(err, &pgErr) && string(pgErr.Code) == codeUniqueViolation {
			return nil, apperror.ErrConflict
		}
		return nil, err
	}
	if eventPayload != nil {
		if _, err := tx.ExecContext(ctx, insertOutboxSQL, out.ID, "payment.intent.created.v1", eventPayload); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &out, nil
}

const getByIDSQL = `
SELECT id, invoice_id, method, status, pg_reference, amount_idr, created_at, paid_at
FROM payment WHERE id = $1
`

const getByPgRefSQL = `
SELECT id, invoice_id, method, status, pg_reference, amount_idr, created_at, paid_at
FROM payment WHERE pg_reference = $1
`

const getByInvoiceSQL = `
SELECT id, invoice_id, method, status, pg_reference, amount_idr, created_at, paid_at
FROM payment WHERE invoice_id = $1 ORDER BY created_at DESC
`

func (r *paymentRepo) GetByID(ctx context.Context, id string) (*model.Payment, error) {
	return r.scanOne(ctx, getByIDSQL, id)
}

func (r *paymentRepo) GetByPgReference(ctx context.Context, ref string) (*model.Payment, error) {
	return r.scanOne(ctx, getByPgRefSQL, ref)
}

func (r *paymentRepo) GetByInvoiceID(ctx context.Context, invoiceID string) (*model.Payment, error) {
	return r.scanOne(ctx, getByInvoiceSQL, invoiceID)
}

func (r *paymentRepo) scanOne(ctx context.Context, query string, args ...interface{}) (*model.Payment, error) {
	var row paymentRow
	if err := r.db.QueryRowxContext(ctx, query, args...).StructScan(&row); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, apperror.ErrNotFound
		}
		return nil, err
	}
	return row.toModel(), nil
}

// MarkSettled flips PENDING → PAID (or PENDING → FAILED) and writes the
// outbox event in the same tx. Idempotent: a no-op if status is already terminal.
func (r *paymentRepo) MarkSettled(ctx context.Context, pgReference string, status model.PaymentStatus, eventType string, eventPayload []byte) (*model.Payment, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var id string
	var current string
	err = tx.QueryRowxContext(ctx,
		`SELECT id, status FROM payment WHERE pg_reference = $1 FOR UPDATE`, pgReference,
	).Scan(&id, &current)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperror.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if current != string(model.PaymentPending) {
		// Already terminal → idempotent no-op (no second event publish either).
		_ = tx.Rollback()
		return r.GetByID(ctx, id)
	}

	// Pass status as text and cast to the enum inside the query so lib/pq
	// doesn't have to guess the parameter type from a CASE expression.
	statusStr := string(status)
	if _, err := tx.ExecContext(ctx,
		`UPDATE payment
		    SET status   = $1::payment_status,
		        paid_at  = CASE WHEN $1 = 'PAID' THEN now() ELSE paid_at END
		  WHERE id = $2`,
		statusStr, id,
	); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, insertOutboxSQL, id, eventType, eventPayload); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return r.GetByID(ctx, id)
}

// ── row mapping ──────────────────────────────────────────────────────────────

type paymentRow struct {
	ID          string      `db:"id"`
	InvoiceID   string      `db:"invoice_id"`
	Method      string      `db:"method"`
	Status      string      `db:"status"`
	PgReference string      `db:"pg_reference"`
	AmountIDR   int64       `db:"amount_idr"`
	CreatedAt   pq.NullTime `db:"created_at"`
	PaidAt      pq.NullTime `db:"paid_at"`
}

func (r paymentRow) toModel() *model.Payment {
	out := &model.Payment{
		ID:          r.ID,
		InvoiceID:   r.InvoiceID,
		Method:      r.Method,
		Status:      model.PaymentStatus(r.Status),
		PgReference: r.PgReference,
		AmountIDR:   r.AmountIDR,
	}
	if r.CreatedAt.Valid {
		out.CreatedAt = r.CreatedAt.Time
	}
	if r.PaidAt.Valid {
		t := r.PaidAt.Time
		out.PaidAt = &t
	}
	return out
}
