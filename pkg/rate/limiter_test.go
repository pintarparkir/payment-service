package rate_test

import (
	"context"
	"testing"
	"time"

	"github.com/farid/payment-service/pkg/rate"
	"github.com/stretchr/testify/require"
)

type fakeStore struct {
	n   int64
	ttl time.Duration
	err error
}

func (f *fakeStore) Incr(ctx context.Context, key string) (int64, error) {
	if f.err != nil {
		return 0, f.err
	}
	f.n++
	return f.n, nil
}

func (f *fakeStore) Expire(ctx context.Context, key string, ttl time.Duration) error {
	f.ttl = ttl
	return f.err
}

func (f *fakeStore) TTL(ctx context.Context, key string) (time.Duration, error) {
	return f.ttl, f.err
}

func TestLimiterDeniesAfterLimit(t *testing.T) {
	store := &fakeStore{ttl: time.Minute}
	lim := rate.New(store)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		ok, _, err := lim.Allow(ctx, "rl:qris-intent:drv-1", 5, time.Minute)
		require.NoError(t, err)
		require.True(t, ok)
	}
	ok, retry, err := lim.Allow(ctx, "rl:qris-intent:drv-1", 5, time.Minute)
	require.NoError(t, err)
	require.False(t, ok)
	require.Greater(t, retry, time.Duration(0))
}
