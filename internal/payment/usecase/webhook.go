package usecase

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/farid/payment-service/internal/payment/model"
	"github.com/farid/payment-service/pkg/midtrans"
)

// HandleWebhook is invoked by the REST handler after midtrans.ParseAndVerify
// has confirmed the signature. We don't trust transaction_status alone — the
// Notification's IsTerminalSuccess/IsTerminalFailure helpers encode the
// settlement-vs-fraud-status logic.
func (u *paymentUsecase) HandleWebhook(ctx context.Context, n *midtrans.Notification) error {
	switch {
	case n.IsTerminalSuccess():
		payload, err := json.Marshal(map[string]any{
			"pg_reference":       n.TransactionID,
			"order_id":           n.OrderID,
			"transaction_status": n.TransactionStatus,
			"gross_amount":       n.GrossAmount,
		})
		if err != nil {
			return fmt.Errorf("payment: marshal webhook payload: %w", err)
		}
		_, err = u.repo.MarkSettled(ctx, n.TransactionID, model.PaymentPaid, model.EvtPaymentPaid, payload)
		return err

	case n.IsTerminalFailure():
		payload, err := json.Marshal(map[string]any{
			"pg_reference":       n.TransactionID,
			"order_id":           n.OrderID,
			"transaction_status": n.TransactionStatus,
		})
		if err != nil {
			return fmt.Errorf("payment: marshal webhook payload: %w", err)
		}
		_, err = u.repo.MarkSettled(ctx, n.TransactionID, model.PaymentFailed, model.EvtPaymentFailed, payload)
		return err
	}
	// "pending" or fraud-blocked "settlement" → leave row PENDING. Midtrans
	// will deliver the eventual terminal status in a later webhook (or the
	// reconciliation cron picks it up).
	return nil
}
