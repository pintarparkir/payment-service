// Package redis provides a simple Redis client wrapper with rate limiting support.
package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Client wraps redis commands needed by payment-service.
type Client interface {
	Ping(ctx context.Context) error
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error
	Get(ctx context.Context, key string) (string, error)
	Del(ctx context.Context, keys ...string) error
}

// Store implements the rate.Store interface for rate limiting.
type Store interface {
	Incr(ctx context.Context, key string) (int64, error)
	Expire(ctx context.Context, key string, ttl time.Duration) error
	TTL(ctx context.Context, key string) (time.Duration, error)
}

// Connection represents a Redis connection pool implementing both interfaces.
type Connection struct {
	client *redis.Client
}

// NewConnection returns a new Redis client configured with standard connection pooling.
func NewConnection(redisHost, redisPort, redisPassword string) *Connection {
	return &Connection{
		client: redis.NewClient(&redis.Options{
			Addr:     fmt.Sprintf("%s:%s", redisHost, redisPort),
			Password: redisPassword,
			DB:       0, // Default DB
		}),
	}
}

// InitConnection creates and returns a Redis connection for the given database (Store only).
func InitConnection(dbName int, redisHost, redisPort, redisPassword string, appConfig string) Store {
	c := NewConnection(redisHost, redisPort, redisPassword)
	return c.WithDB(dbName)
}

// WithDB switches to a specific database and returns a Connection instance.
func (c *Connection) WithDB(db int) *Connection {
	addr := "localhost:6379"
	if c.client != nil && c.client.Options().Addr != "" {
		addr = c.client.Options().Addr
	}
	pwd := ""
	if c.client != nil {
		pwd = c.client.Options().Password
	}
	return &Connection{
		client: redis.NewClient(&redis.Options{
			Addr:     addr,
			Password: pwd,
			DB:       db,
		}),
	}
}

// Ping checks the Redis connection health.
func (c *Connection) Ping(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}

// Set stores a key-value pair with optional expiration.
func (c *Connection) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	return c.client.Set(ctx, key, value, expiration).Err()
}

// Get retrieves a value by key.
func (c *Connection) Get(ctx context.Context, key string) (string, error) {
	return c.client.Get(ctx, key).Result()
}

// Del deletes one or more keys.
func (c *Connection) Del(ctx context.Context, keys ...string) error {
	return c.client.Del(ctx, keys...).Err()
}

// Incr increments a counter atomically.
func (c *Connection) Incr(ctx context.Context, key string) (int64, error) {
	return c.client.Incr(ctx, key).Result()
}

// Expire sets the TTL for a key in seconds.
func (c *Connection) Expire(ctx context.Context, key string, ttl time.Duration) error {
	return c.client.Expire(ctx, key, ttl).Err()
}

// TTL gets the remaining TTL of a key.
func (c *Connection) TTL(ctx context.Context, key string) (time.Duration, error) {
	return c.client.TTL(ctx, key).Result()
}
