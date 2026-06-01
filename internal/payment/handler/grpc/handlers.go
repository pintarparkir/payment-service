package grpc

import (
	"context"

	"google.golang.org/protobuf/types/known/timestamppb"

	paymentv1 "github.com/farid/payment-service/api/proto/payment/v1"
	"github.com/farid/payment-service/internal/payment/model"
)

func (s *Server) CreateQrisIntent(ctx context.Context, req *paymentv1.CreateQrisIntentRequest) (*paymentv1.QrisIntent, error) {
	// Caller supplies amount_idr (mini app or another s2s caller that already
	// loaded the invoice). Future work: dial billing.GetInvoice and resolve
	// automatically when amount_idr is unset.
	out, err := s.uc.CreateQrisIntent(ctx, req.GetInvoiceId(), req.GetAmountIdr())
	if err != nil {
		return nil, err
	}
	return &paymentv1.QrisIntent{
		PaymentId:   out.PaymentID,
		QrisPayload: out.QrisPayload,
		RedirectUrl: out.RedirectURL, // Include SNAP redirect URL
		PgReference: out.PgReference,
		ExpiresAt:   timestamppb.New(out.ExpiresAt),
	}, nil
}

func (s *Server) GetPayment(ctx context.Context, req *paymentv1.GetPaymentRequest) (*paymentv1.Payment, error) {
	p, err := s.uc.GetPayment(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	return paymentToProto(p), nil
}

func paymentToProto(p *model.Payment) *paymentv1.Payment {
	if p == nil {
		return nil
	}
	out := &paymentv1.Payment{
		Id:          p.ID,
		InvoiceId:   p.InvoiceID,
		Method:      p.Method,
		Status:      statusToProto(p.Status),
		PgReference: p.PgReference,
		AmountIdr:   p.AmountIDR,
		CreatedAt:   timestamppb.New(p.CreatedAt),
	}
	if p.PaidAt != nil {
		out.PaidAt = timestamppb.New(*p.PaidAt)
	}
	return out
}

func statusToProto(s model.PaymentStatus) paymentv1.PaymentStatus {
	switch s {
	case model.PaymentPending:
		return paymentv1.PaymentStatus_PENDING
	case model.PaymentPaid:
		return paymentv1.PaymentStatus_PAID
	case model.PaymentFailed:
		return paymentv1.PaymentStatus_FAILED
	case model.PaymentRefunded:
		return paymentv1.PaymentStatus_REFUNDED
	}
	return paymentv1.PaymentStatus_PAYMENT_STATUS_UNSPECIFIED
}
