// Package redis wraps go-redis v8 and exposes a thin Collections facade
// to keep call-sites independent of go-redis types.
package redis

import (
	"context"
	"fmt"
	"time"

	redisotel "github.com/go-redis/redis/extra/redisotel/v8"
	"github.com/go-redis/redis/v8"
)

// Collections is the application-facing interface. Adapters expose just
// the verbs callers actually use (set/get/setnx/del/eval), nothing more.
type Collections interface {
	Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error
	Get(ctx context.Context, key string) (string, error)
	SetNX(ctx context.Context, key string, value interface{}, ttl time.Duration) (bool, error)
	Del(ctx context.Context, keys ...string) error
	Eval(ctx context.Context, script string, keys []string, args ...interface{}) (interface{}, error)
	Incr(ctx context.Context, key string) (int64, error)
	Expire(ctx context.Context, key string, ttl time.Duration) error
	TTL(ctx context.Context, key string) (time.Duration, error)
	Ping(ctx context.Context) error
	Raw() *redis.Client
}

type redisAdapter struct {
	client *redis.Client
	prefix string
}

// InitConnection builds a *redis.Client and wraps it in the Collections facade.
// `appConfig` becomes a key prefix so multiple apps can share a Redis db cleanly.
func InitConnection(db int, host, port, password, appConfig string) Collections {
	c := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%s", host, port),
		Password: password,
		DB:       db,
	})
	c.AddHook(redisotel.NewTracingHook())
	return &redisAdapter{client: c, prefix: appConfig + ":"}
}

func (r *redisAdapter) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	return r.client.Set(ctx, r.prefix+key, value, ttl).Err()
}

func (r *redisAdapter) Get(ctx context.Context, key string) (string, error) {
	return r.client.Get(ctx, r.prefix+key).Result()
}

func (r *redisAdapter) SetNX(ctx context.Context, key string, value interface{}, ttl time.Duration) (bool, error) {
	return r.client.SetNX(ctx, r.prefix+key, value, ttl).Result()
}

func (r *redisAdapter) Del(ctx context.Context, keys ...string) error {
	pks := make([]string, len(keys))
	for i, k := range keys {
		pks[i] = r.prefix + k
	}
	return r.client.Del(ctx, pks...).Err()
}

func (r *redisAdapter) Eval(ctx context.Context, script string, keys []string, args ...interface{}) (interface{}, error) {
	pks := make([]string, len(keys))
	for i, k := range keys {
		pks[i] = r.prefix + k
	}
	return r.client.Eval(ctx, script, pks, args...).Result()
}

func (r *redisAdapter) Incr(ctx context.Context, key string) (int64, error) {
	return r.client.Incr(ctx, r.prefix+key).Result()
}

func (r *redisAdapter) Expire(ctx context.Context, key string, ttl time.Duration) error {
	return r.client.Expire(ctx, r.prefix+key, ttl).Err()
}

func (r *redisAdapter) TTL(ctx context.Context, key string) (time.Duration, error) {
	return r.client.TTL(ctx, r.prefix+key).Result()
}

func (r *redisAdapter) Ping(ctx context.Context) error { return r.client.Ping(ctx).Err() }

func (r *redisAdapter) Raw() *redis.Client { return r.client }
