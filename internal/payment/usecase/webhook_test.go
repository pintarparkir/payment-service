package usecase_test

import (
	"encoding/json"
	"testing"

	"github.com/farid/payment-service/internal/payment/model"
	apperror "github.com/farid/payment-service/pkg/error"
	"github.com/farid/payment-service/pkg/midtrans"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func webhookNotification(txn, orderID, status, fraud string) *midtrans.Notification {
	return &midtrans.Notification{
		TransactionID:     txn,
		OrderID:           orderID,
		TransactionStatus: status,
		FraudStatus:       fraud,
		GrossAmount:       "10000.00",
	}
}

func TestHandleWebhook_TerminalSuccess(t *testing.T) {
	deps := newUsecaseDeps()
	ctx := deps.ctx
	repo := deps.repo
	n := webhookNotification("txn-1", "ord-1", "settlement", "accept")

	paid := mkPayment("pay-1", "ord-1", model.PaymentPaid)
	repo.On("MarkSettled", ctx, "txn-1", model.PaymentPaid, model.EvtPaymentPaid,
		mock.MatchedBy(func(b []byte) bool {
			var m map[string]any
			return json.Unmarshal(b, &m) == nil &&
				m["pg_reference"] == "txn-1" &&
				m["order_id"] == "ord-1" &&
				m["gross_amount"] == "10000.00"
		}),
	).Return(paid, nil)

	err := deps.uc.HandleWebhook(ctx, n)

	require.NoError(t, err)
	repo.AssertExpectations(t)
}

func TestHandleWebhook_TerminalFailure(t *testing.T) {
	deps := newUsecaseDeps()
	ctx := deps.ctx
	repo := deps.repo
	n := webhookNotification("txn-2", "ord-2", "deny", "accept")

	failed := mkPayment("pay-2", "ord-2", model.PaymentFailed)
	repo.On("MarkSettled", ctx, "txn-2", model.PaymentFailed, model.EvtPaymentFailed,
		mock.MatchedBy(func(b []byte) bool {
			var m map[string]any
			return json.Unmarshal(b, &m) == nil &&
				m["pg_reference"] == "txn-2" &&
				m["order_id"] == "ord-2" &&
				m["transaction_status"] == "deny"
		}),
	).Return(failed, nil)

	err := deps.uc.HandleWebhook(ctx, n)

	require.NoError(t, err)
	repo.AssertExpectations(t)
}

func TestHandleWebhook_NonTerminal(t *testing.T) {
	deps := newUsecaseDeps()
	n := &midtrans.Notification{
		TransactionID:     "txn-3",
		TransactionStatus: "pending",
		FraudStatus:       "accept",
	}

	err := deps.uc.HandleWebhook(deps.ctx, n)

	require.NoError(t, err)
	deps.repo.AssertNotCalled(t, "MarkSettled")
}

func TestHandleWebhook_MarkSettledError(t *testing.T) {
	deps := newUsecaseDeps()
	ctx := deps.ctx
	repo := deps.repo
	n := webhookNotification("txn-4", "ord-4", "settlement", "accept")

	repo.On("MarkSettled", ctx, "txn-4", model.PaymentPaid, model.EvtPaymentPaid, mock.Anything).
		Return(nil, apperror.ErrConflict)

	err := deps.uc.HandleWebhook(ctx, n)

	require.True(t, apperror.Is(err, apperror.ErrConflict))
	repo.AssertExpectations(t)
}

func TestHandleWebhook_FraudBlockedSettlement(t *testing.T) {
	deps := newUsecaseDeps()
	n := webhookNotification("txn-fraud", "ord-fraud", "settlement", "deny")

	err := deps.uc.HandleWebhook(deps.ctx, n)

	require.NoError(t, err)
	deps.repo.AssertNotCalled(t, "MarkSettled")
}

func TestHandleWebhook_IdempotentReplay(t *testing.T) {
	deps := newUsecaseDeps()
	ctx := deps.ctx
	repo := deps.repo
	n := webhookNotification("txn-replay", "ord-replay", "settlement", "accept")

	existing := mkPayment("pay-replay", "ord-replay", model.PaymentPaid)
	existing.PgReference = "txn-replay"
	repo.On("MarkSettled", ctx, "txn-replay", model.PaymentPaid, model.EvtPaymentPaid, mock.Anything).
		Return(existing, nil)

	// First call
	err := deps.uc.HandleWebhook(ctx, n)
	require.NoError(t, err)

	// Second call (replay) — should still succeed, MarkSettled called again but returns same row
	err = deps.uc.HandleWebhook(ctx, n)
	require.NoError(t, err)

	// Verify MarkSettled was called twice (repo layer handles idempotency)
	repo.AssertNumberOfCalls(t, "MarkSettled", 2)
}

func TestHandleWebhook_AlreadyTerminal(t *testing.T) {
	deps := newUsecaseDeps()
	ctx := deps.ctx
	repo := deps.repo
	n := webhookNotification("txn-already-paid", "ord-already-paid", "settlement", "accept")

	existing := mkPayment("pay-already", "ord-already-paid", model.PaymentPaid)
	existing.PgReference = "txn-already-paid"
	repo.On("MarkSettled", ctx, "txn-already-paid", model.PaymentPaid, model.EvtPaymentPaid, mock.Anything).
		Return(existing, nil)

	err := deps.uc.HandleWebhook(ctx, n)

	require.NoError(t, err)
	repo.AssertExpectations(t)
}
