package mockrepo

import (
	"context"

	"github.com/farid/payment-service/internal/payment/model"
	"github.com/farid/payment-service/internal/payment/repository"
	"github.com/stretchr/testify/mock"
)

var _ repository.PaymentRepository = (*MockPaymentRepository)(nil)

// MockPaymentRepository implements repository.PaymentRepository.
type MockPaymentRepository struct {
	mock.Mock
}

func (m *MockPaymentRepository) Insert(
	ctx context.Context,
	p *model.Payment,
	eventPayload []byte,
) (*model.Payment, error) {
	args := m.Called(ctx, p, eventPayload)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Payment), args.Error(1)
}

func (m *MockPaymentRepository) GetByID(ctx context.Context, id string) (*model.Payment, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Payment), args.Error(1)
}

func (m *MockPaymentRepository) GetByPgReference(ctx context.Context, ref string) (*model.Payment, error) {
	args := m.Called(ctx, ref)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Payment), args.Error(1)
}

func (m *MockPaymentRepository) GetByInvoiceID(ctx context.Context, invoiceID string) (*model.Payment, error) {
	args := m.Called(ctx, invoiceID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Payment), args.Error(1)
}

func (m *MockPaymentRepository) MarkSettled(
	ctx context.Context,
	pgReference string,
	status model.PaymentStatus,
	eventType string,
	eventPayload []byte,
) (*model.Payment, error) {
	args := m.Called(ctx, pgReference, status, eventType, eventPayload)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Payment), args.Error(1)
}
