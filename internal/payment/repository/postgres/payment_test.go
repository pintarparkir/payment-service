package postgres_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/farid/payment-service/internal/payment/model"
	"github.com/farid/payment-service/internal/payment/repository/postgres"
	apperror "github.com/farid/payment-service/pkg/error"
)

func newMockDB(t *testing.T) (*sqlx.DB, sqlmock.Sqlmock) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	return sqlx.NewDb(db, "postgres"), mock
}

func paymentRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "invoice_id", "method", "status", "pg_reference", "amount_idr", "created_at", "paid_at"}).
		AddRow("pay-1", "inv-1", "QRIS", "PENDING", "txn-1", int64(10000), time.Now().UTC(), nil)
}

func TestPaymentRepo_Insert_HappyPath(t *testing.T) {
	db, mock := newMockDB(t)
	defer db.Close()

	ctx := context.Background()
	repo := postgres.NewPaymentRepository(db)

	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO payment`).
		WithArgs("inv-1", "QRIS", "txn-1", int64(10000)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow("pay-1", time.Now().UTC()))
	mock.ExpectExec(`INSERT INTO outbox_event`).
		WithArgs("pay-1", "payment.intent.created.v1", []byte(`{}`)).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	got, err := repo.Insert(ctx, &model.Payment{InvoiceID: "inv-1", PgReference: "txn-1", AmountIDR: 10000}, []byte(`{}`))

	require.NoError(t, err)
	assert.Equal(t, "pay-1", got.ID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestPaymentRepo_Insert_UniqueViolation(t *testing.T) {
	db, mock := newMockDB(t)
	defer db.Close()

	ctx := context.Background()
	repo := postgres.NewPaymentRepository(db)

	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO payment`).
		WillReturnError(&pq.Error{Code: "23505"})
	mock.ExpectRollback()

	_, err := repo.Insert(ctx, &model.Payment{InvoiceID: "inv-1", PgReference: "txn-1", AmountIDR: 10000}, nil)

	require.Error(t, err)
	assert.True(t, apperror.Is(err, apperror.ErrConflict))
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestPaymentRepo_GetByID_Found(t *testing.T) {
	db, mock := newMockDB(t)
	defer db.Close()

	ctx := context.Background()
	repo := postgres.NewPaymentRepository(db)

	mock.ExpectQuery(`FROM payment WHERE id`).WithArgs("pay-1").WillReturnRows(paymentRows())

	got, err := repo.GetByID(ctx, "pay-1")

	require.NoError(t, err)
	assert.Equal(t, "pay-1", got.ID)
	assert.Equal(t, model.PaymentPending, got.Status)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestPaymentRepo_GetByID_NotFound(t *testing.T) {
	db, mock := newMockDB(t)
	defer db.Close()

	ctx := context.Background()
	repo := postgres.NewPaymentRepository(db)

	mock.ExpectQuery(`FROM payment WHERE id`).WithArgs("missing").WillReturnError(sql.ErrNoRows)

	_, err := repo.GetByID(ctx, "missing")

	require.Error(t, err)
	assert.True(t, apperror.Is(err, apperror.ErrNotFound))
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestPaymentRepo_GetByPgReference_Found(t *testing.T) {
	db, mock := newMockDB(t)
	defer db.Close()

	ctx := context.Background()
	repo := postgres.NewPaymentRepository(db)

	mock.ExpectQuery(`FROM payment WHERE pg_reference`).WithArgs("txn-1").WillReturnRows(paymentRows())

	got, err := repo.GetByPgReference(ctx, "txn-1")

	require.NoError(t, err)
	assert.Equal(t, "txn-1", got.PgReference)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestPaymentRepo_GetByInvoiceID_Found(t *testing.T) {
	db, mock := newMockDB(t)
	defer db.Close()

	ctx := context.Background()
	repo := postgres.NewPaymentRepository(db)

	mock.ExpectQuery(`FROM payment WHERE invoice_id`).WithArgs("inv-1").WillReturnRows(paymentRows())

	got, err := repo.GetByInvoiceID(ctx, "inv-1")

	require.NoError(t, err)
	assert.Equal(t, "inv-1", got.InvoiceID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestPaymentRepo_MarkSettled_HappyPath(t *testing.T) {
	db, mock := newMockDB(t)
	defer db.Close()

	ctx := context.Background()
	repo := postgres.NewPaymentRepository(db)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id, status FROM payment`).
		WithArgs("txn-1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "status"}).AddRow("pay-1", "PENDING"))
	mock.ExpectExec(`UPDATE payment`).
		WithArgs("PAID", "pay-1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO outbox_event`).
		WithArgs("pay-1", "payment.paid.v1", []byte(`{}`)).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	mock.ExpectQuery(`FROM payment WHERE id`).WithArgs("pay-1").WillReturnRows(paymentRows())

	got, err := repo.MarkSettled(ctx, "txn-1", model.PaymentPaid, "payment.paid.v1", []byte(`{}`))

	require.NoError(t, err)
	assert.Equal(t, "pay-1", got.ID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestPaymentRepo_MarkSettled_AlreadyTerminal(t *testing.T) {
	db, mock := newMockDB(t)
	defer db.Close()

	ctx := context.Background()
	repo := postgres.NewPaymentRepository(db)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id, status FROM payment`).
		WithArgs("txn-1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "status"}).AddRow("pay-1", "PAID"))
	mock.ExpectRollback()
	mock.ExpectQuery(`FROM payment WHERE id`).WithArgs("pay-1").WillReturnRows(paymentRows())

	got, err := repo.MarkSettled(ctx, "txn-1", model.PaymentPaid, "payment.paid.v1", nil)

	require.NoError(t, err)
	assert.Equal(t, "pay-1", got.ID)
	assert.NoError(t, mock.ExpectationsWereMet())
}
