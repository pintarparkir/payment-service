package usecase_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/farid/payment-service/internal/payment/model"
	mockrepo "github.com/farid/payment-service/internal/payment/repository/mock"
	apperror "github.com/farid/payment-service/pkg/error"
	"github.com/farid/payment-service/pkg/midtrans"
	mockmidtrans "github.com/farid/payment-service/pkg/midtrans/mock"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestHandleWebhook_TerminalSuccess(t *testing.T) {
	ctx := context.Background()
	repo := new(mockrepo.MockPaymentRepository)
	mt := new(mockmidtrans.MockMidtransClient)

	n := &midtrans.Notification{
		TransactionID:     "txn-1",
		OrderID:           "ord-1",
		TransactionStatus: "settlement",
		FraudStatus:       "accept",
		GrossAmount:       "10000.00",
	}

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

	uc := newUC(repo, mt)
	err := uc.HandleWebhook(ctx, n)

	require.NoError(t, err)
	repo.AssertExpectations(t)
}

func TestHandleWebhook_TerminalFailure(t *testing.T) {
	ctx := context.Background()
	repo := new(mockrepo.MockPaymentRepository)
	mt := new(mockmidtrans.MockMidtransClient)

	n := &midtrans.Notification{
		TransactionID:     "txn-2",
		OrderID:           "ord-2",
		TransactionStatus: "deny",
		FraudStatus:       "accept",
		GrossAmount:       "10000.00",
	}

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

	uc := newUC(repo, mt)
	err := uc.HandleWebhook(ctx, n)

	require.NoError(t, err)
	repo.AssertExpectations(t)
}

func TestHandleWebhook_NonTerminal(t *testing.T) {
	ctx := context.Background()
	repo := new(mockrepo.MockPaymentRepository)
	mt := new(mockmidtrans.MockMidtransClient)

	// "pending" means the QRIS code hasn't been scanned yet. The usecase does
	// nothing and waits for the eventual terminal webhook.
	n := &midtrans.Notification{
		TransactionID:     "txn-3",
		TransactionStatus: "pending",
		FraudStatus:       "accept",
	}

	uc := newUC(repo, mt)
	err := uc.HandleWebhook(ctx, n)

	require.NoError(t, err)
	repo.AssertNotCalled(t, "MarkSettled")
}

func TestHandleWebhook_MarkSettledError(t *testing.T) {
	ctx := context.Background()
	repo := new(mockrepo.MockPaymentRepository)
	mt := new(mockmidtrans.MockMidtransClient)

	n := &midtrans.Notification{
		TransactionID:     "txn-4",
		OrderID:           "ord-4",
		TransactionStatus: "settlement",
		FraudStatus:       "accept",
	}

	repo.On("MarkSettled", ctx, "txn-4", model.PaymentPaid, model.EvtPaymentPaid, mock.Anything).
		Return(nil, apperror.ErrConflict)

	uc := newUC(repo, mt)
	err := uc.HandleWebhook(ctx, n)

	require.True(t, apperror.Is(err, apperror.ErrConflict))
	repo.AssertExpectations(t)
}
