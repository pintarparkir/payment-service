package midtrans_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/farid/payment-service/pkg/midtrans"
	"github.com/sony/gobreaker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCircuitBreakerOpensAfter5Failures(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"status_code":"500","status_message":"internal error"}`))
	}))
	defer srv.Close()

	client := midtrans.NewHTTPClient(srv.URL, "test-key")
	ctx := context.Background()

	// First 5 calls should hit the server and fail
	for i := 0; i < 5; i++ {
		_, err := client.Charge(ctx, "order-"+string(rune('0'+i)), 10000)
		require.Error(t, err)
	}
	assert.Equal(t, int32(5), calls.Load())

	// 6th call should be rejected by breaker without hitting server
	_, err := client.Charge(ctx, "order-6", 10000)
	require.Error(t, err)
	assert.ErrorIs(t, err, gobreaker.ErrOpenState)
	assert.Equal(t, int32(5), calls.Load())
}

func TestCircuitBreakerAllowsWhenServerHealthy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{
			"status_code":"201",
			"status_message":"Success, QRIS transaction is created",
			"transaction_id":"txn-123",
			"order_id":"order-1",
			"gross_amount":"10000",
			"qr_string":"00020101021226680022",
			"expiry_time":"2026-05-21 10:15:00"
		}`))
	}))
	defer srv.Close()

	client := midtrans.NewHTTPClient(srv.URL, "test-key")
	ctx := context.Background()

	result, err := client.Charge(ctx, "order-1", 10000)
	require.NoError(t, err)
	assert.Equal(t, "txn-123", result.TransactionID)
	assert.Equal(t, "00020101021226680022", result.QrisPayload)
}
