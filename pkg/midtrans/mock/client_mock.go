package mockmidtrans

import (
	"context"

	"github.com/farid/payment-service/pkg/midtrans"
	"github.com/stretchr/testify/mock"
)

var _ midtrans.Client = (*MockMidtransClient)(nil)

// MockMidtransClient implements midtrans.Client.
type MockMidtransClient struct {
	mock.Mock
}

func (m *MockMidtransClient) Charge(
	ctx context.Context,
	orderID string,
	amountIDR int64,
) (*midtrans.ChargeResult, error) {
	args := m.Called(ctx, orderID, amountIDR)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*midtrans.ChargeResult), args.Error(1)
}
