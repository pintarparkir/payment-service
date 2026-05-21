package usecase_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/farid/payment-service/internal/payment/model"
	mockrepo "github.com/farid/payment-service/internal/payment/repository/mock"
	"github.com/farid/payment-service/internal/payment/usecase"
	apperror "github.com/farid/payment-service/pkg/error"
	"github.com/farid/payment-service/pkg/midtrans"
	mockmidtrans "github.com/farid/payment-service/pkg/midtrans/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// newUC wires a PaymentUsecase with the given mocks.
func newUC(repo *mockrepo.MockPaymentRepository, mt *mockmidtrans.MockMidtransClient) usecase.PaymentUsecase {
	return usecase.NewPaymentUsecase(repo, mt)
}

// mkPayment builds a minimal Payment for use as mock return values.
func mkPayment(id, invoiceID string, status model.PaymentStatus) *model.Payment {
	return &model.Payment{
		ID:          id,
		InvoiceID:   invoiceID,
		Method:      "QRIS",
		Status:      status,
		PgReference: "txn-" + id,
		AmountIDR:   10000,
		CreatedAt:   time.Now().UTC(),
	}
}

type usecaseDeps struct {
	ctx  context.Context
	repo *mockrepo.MockPaymentRepository
	mt   *mockmidtrans.MockMidtransClient
	uc   usecase.PaymentUsecase
}

func newUsecaseDeps() usecaseDeps {
	repo := new(mockrepo.MockPaymentRepository)
	mt := new(mockmidtrans.MockMidtransClient)
	return usecaseDeps{
		ctx:  context.Background(),
		repo: repo,
		mt:   mt,
		uc:   newUC(repo, mt),
	}
}

// ── Validation ───────────────────────────────────────────────────────────────

func TestCreateQrisIntent_MissingInvoiceID(t *testing.T) {
	ctx := context.Background()
	repo := new(mockrepo.MockPaymentRepository)
	mt := new(mockmidtrans.MockMidtransClient)

	uc := newUC(repo, mt)
	_, err := uc.CreateQrisIntent(ctx, "", 10000)

	require.Error(t, err)
	var ae *apperror.AppError
	require.ErrorAs(t, err, &ae)
	assert.Equal(t, "VALIDATION", ae.Code)
	repo.AssertNotCalled(t, "GetByInvoiceID")
	mt.AssertNotCalled(t, "Charge")
}

func TestCreateQrisIntent_ZeroAmount(t *testing.T) {
	ctx := context.Background()
	repo := new(mockrepo.MockPaymentRepository)
	mt := new(mockmidtrans.MockMidtransClient)

	uc := newUC(repo, mt)
	_, err := uc.CreateQrisIntent(ctx, "inv-1", 0)

	require.Error(t, err)
	var ae *apperror.AppError
	require.ErrorAs(t, err, &ae)
	assert.Equal(t, "VALIDATION", ae.Code)
	repo.AssertNotCalled(t, "GetByInvoiceID")
}

func TestCreateQrisIntent_NegativeAmount(t *testing.T) {
	ctx := context.Background()
	repo := new(mockrepo.MockPaymentRepository)
	mt := new(mockmidtrans.MockMidtransClient)

	uc := newUC(repo, mt)
	_, err := uc.CreateQrisIntent(ctx, "inv-1", -1)

	require.Error(t, err)
	var ae *apperror.AppError
	require.ErrorAs(t, err, &ae)
	assert.Equal(t, "VALIDATION", ae.Code)
}

// ── Idempotency replay ───────────────────────────────────────────────────────

func TestCreateQrisIntent_IdempotentReplay(t *testing.T) {
	ctx := context.Background()
	repo := new(mockrepo.MockPaymentRepository)
	mt := new(mockmidtrans.MockMidtransClient)

	// An active PENDING intent exists → reuse it without calling Midtrans.
	existing := mkPayment("pay-1", "inv-1", model.PaymentPending)
	repo.On("GetByInvoiceID", ctx, "inv-1").Return(existing, nil)

	uc := newUC(repo, mt)
	got, err := uc.CreateQrisIntent(ctx, "inv-1", 10000)

	require.NoError(t, err)
	assert.Equal(t, existing.ID, got.PaymentID)
	assert.Equal(t, existing.PgReference, got.PgReference)
	assert.Empty(t, got.QrisPayload)
	mt.AssertNotCalled(t, "Charge")
	repo.AssertNotCalled(t, "Insert")
}

// ── Happy path ───────────────────────────────────────────────────────────────

func TestCreateQrisIntent_HappyPath(t *testing.T) {
	ctx := context.Background()
	repo := new(mockrepo.MockPaymentRepository)
	mt := new(mockmidtrans.MockMidtransClient)

	repo.On("GetByInvoiceID", ctx, "inv-1").Return(nil, apperror.ErrNotFound)

	charge := &midtrans.ChargeResult{
		TransactionID: "txn-123",
		QrisPayload:   "00020101...",
		ExpiresAt:     time.Now().Add(15 * time.Minute),
	}
	mt.On("Charge", ctx, "inv-1", int64(10000)).Return(charge, nil)

	created := mkPayment("pay-new", "inv-1", model.PaymentPending)
	created.PgReference = "txn-123"
	repo.On("Insert", ctx,
		mock.MatchedBy(func(p *model.Payment) bool {
			return p.InvoiceID == "inv-1" && p.PgReference == "txn-123" && p.AmountIDR == 10000
		}),
		mock.Anything,
	).Return(created, nil)

	uc := newUC(repo, mt)
	got, err := uc.CreateQrisIntent(ctx, "inv-1", 10000)

	require.NoError(t, err)
	assert.Equal(t, created.ID, got.PaymentID)
	assert.Equal(t, "00020101...", got.QrisPayload)
	assert.Equal(t, "txn-123", got.PgReference)
	repo.AssertExpectations(t)
	mt.AssertExpectations(t)
}

// ── Midtrans error ───────────────────────────────────────────────────────────

func TestCreateQrisIntent_MidtransError(t *testing.T) {
	ctx := context.Background()
	repo := new(mockrepo.MockPaymentRepository)
	mt := new(mockmidtrans.MockMidtransClient)

	repo.On("GetByInvoiceID", ctx, "inv-1").Return(nil, apperror.ErrNotFound)
	mt.On("Charge", ctx, "inv-1", int64(10000)).Return(nil, fmt.Errorf("network timeout"))

	uc := newUC(repo, mt)
	_, err := uc.CreateQrisIntent(ctx, "inv-1", 10000)

	require.Error(t, err)
	assert.True(t, apperror.Is(err, apperror.ErrUpstreamDown))
	repo.AssertNotCalled(t, "Insert")
}

// ── ErrConflict race recovery ────────────────────────────────────────────────

func TestCreateQrisIntent_InsertConflict_Recovery(t *testing.T) {
	ctx := context.Background()
	repo := new(mockrepo.MockPaymentRepository)
	mt := new(mockmidtrans.MockMidtransClient)

	// Two concurrent goroutines both pass the idempotency check and both call
	// Midtrans. The second Insert hits the UNIQUE constraint on pg_reference.
	// The usecase falls back to GetByPgReference and returns the existing row.
	repo.On("GetByInvoiceID", ctx, "inv-1").Return(nil, apperror.ErrNotFound)

	charge := &midtrans.ChargeResult{
		TransactionID: "txn-race",
		QrisPayload:   "00020101...",
		ExpiresAt:     time.Now().Add(15 * time.Minute),
	}
	mt.On("Charge", ctx, "inv-1", int64(10000)).Return(charge, nil)
	repo.On("Insert", ctx, mock.Anything, mock.Anything).Return(nil, apperror.ErrConflict)

	existing := mkPayment("pay-existing", "inv-1", model.PaymentPending)
	existing.PgReference = "txn-race"
	repo.On("GetByPgReference", ctx, "txn-race").Return(existing, nil)

	uc := newUC(repo, mt)
	got, err := uc.CreateQrisIntent(ctx, "inv-1", 10000)

	require.NoError(t, err)
	assert.Equal(t, existing.ID, got.PaymentID)
	assert.Equal(t, "txn-race", got.PgReference)
	assert.Equal(t, "00020101...", got.QrisPayload)
	repo.AssertExpectations(t)
}

// ── Existing non-PENDING payment ─────────────────────────────────────────────

func TestCreateQrisIntent_ExistingNonPending(t *testing.T) {
	ctx := context.Background()
	repo := new(mockrepo.MockPaymentRepository)
	mt := new(mockmidtrans.MockMidtransClient)

	// A PAID payment exists for this invoice — the usecase does NOT replay it;
	// it falls through and creates a new payment attempt via Midtrans.
	paid := mkPayment("pay-paid", "inv-1", model.PaymentPaid)
	repo.On("GetByInvoiceID", ctx, "inv-1").Return(paid, nil)

	charge := &midtrans.ChargeResult{
		TransactionID: "txn-new",
		QrisPayload:   "00020101...",
		ExpiresAt:     time.Now().Add(15 * time.Minute),
	}
	mt.On("Charge", ctx, "inv-1", int64(10000)).Return(charge, nil)

	newPay := mkPayment("pay-new", "inv-1", model.PaymentPending)
	newPay.PgReference = "txn-new"
	repo.On("Insert", ctx, mock.Anything, mock.Anything).Return(newPay, nil)

	uc := newUC(repo, mt)
	got, err := uc.CreateQrisIntent(ctx, "inv-1", 10000)

	require.NoError(t, err)
	assert.Equal(t, newPay.ID, got.PaymentID)
	mt.AssertExpectations(t)
	repo.AssertExpectations(t)
}

// ── ErrConflict + GetByPgReference failure ──────────────────────────────────────

func TestCreateQrisIntent_InsertConflict_GetByPgRefFails(t *testing.T) {
	ctx := context.Background()
	repo := new(mockrepo.MockPaymentRepository)
	mt := new(mockmidtrans.MockMidtransClient)

	repo.On("GetByInvoiceID", ctx, "inv-1").Return(nil, apperror.ErrNotFound)
	charge := &midtrans.ChargeResult{
		TransactionID: "txn-fail",
		QrisPayload:   "...",
		ExpiresAt:     time.Now().Add(15 * time.Minute),
	}
	mt.On("Charge", ctx, "inv-1", int64(10000)).Return(charge, nil)
	repo.On("Insert", ctx, mock.Anything, mock.Anything).Return(nil, apperror.ErrConflict)
	repo.On("GetByPgReference", ctx, "txn-fail").Return(nil, apperror.ErrNotFound)

	uc := newUC(repo, mt)
	_, err := uc.CreateQrisIntent(ctx, "inv-1", 10000)

	require.Error(t, err)
	assert.True(t, apperror.Is(err, apperror.ErrConflict))
	repo.AssertExpectations(t)
}

// ── GetPayment ───────────────────────────────────────────────────────────────

func TestGetPayment_OK(t *testing.T) {
	ctx := context.Background()
	repo := new(mockrepo.MockPaymentRepository)
	mt := new(mockmidtrans.MockMidtransClient)

	pay := mkPayment("pay-get-1", "inv-1", model.PaymentPaid)
	repo.On("GetByID", ctx, "pay-get-1").Return(pay, nil)

	uc := newUC(repo, mt)
	got, err := uc.GetPayment(ctx, "pay-get-1")

	require.NoError(t, err)
	assert.Equal(t, pay, got)
	repo.AssertExpectations(t)
}

func TestGetPayment_NotFound(t *testing.T) {
	ctx := context.Background()
	repo := new(mockrepo.MockPaymentRepository)
	mt := new(mockmidtrans.MockMidtransClient)

	repo.On("GetByID", ctx, "missing").Return(nil, apperror.ErrNotFound)

	uc := newUC(repo, mt)
	_, err := uc.GetPayment(ctx, "missing")

	require.Error(t, err)
	assert.True(t, apperror.Is(err, apperror.ErrNotFound))
}
