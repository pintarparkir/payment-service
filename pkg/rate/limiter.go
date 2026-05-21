// Package rate provides rate limiting utilities.
package rate

import (
	"context"
	"time"
)

type Store interface {
	Incr(ctx context.Context, key string) (int64, error)
	Expire(ctx context.Context, key string, ttl time.Duration) error
	TTL(ctx context.Context, key string) (time.Duration, error)
}

type Limiter interface {
	Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, time.Duration, error)
}

type limiter struct {
	store Store
}

func New(store Store) Limiter {
	return &limiter{store: store}
}

func (l *limiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, time.Duration, error) {
	n, err := l.store.Incr(ctx, key)
	if err != nil {
		return false, 0, err
	}
	if n == 1 {
		if expireErr := l.store.Expire(ctx, key, window); expireErr != nil {
			return false, 0, expireErr
		}
	}
	if int(n) <= limit {
		return true, 0, nil
	}
	ttl, err := l.store.TTL(ctx, key)
	if err != nil || ttl <= 0 {
		ttl = window
	}
	return false, ttl, nil
}
