// Package idempotency provides a Postgres-backed store + a gRPC interceptor
// that enforces idempotency on annotated RPCs.
//
// Per soal: CreateReservation, OpenInvoice/Checkout MUST be idempotent.
// Clients pass `Idempotency-Key: <uuid>` in gRPC metadata.
package idempotency

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
)

type postgresStore struct {
	db *sqlx.DB
}

// NewPostgresStore returns an idempotency store backed by PostgreSQL.
func NewPostgresStore(db *sqlx.DB) StoreInterface { return &postgresStore{db: db} }

func (s *postgresStore) Get(ctx context.Context, scope, key string) ([]byte, bool, error) {
	if key == "" {
		return nil, false, nil
	}
	var payload []byte
	err := s.db.QueryRowxContext(ctx,
		`SELECT response_payload FROM idempotency_key WHERE scope = $1 AND key = $2`,
		scope, key,
	).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return payload, true, nil
}

func (s *postgresStore) Put(ctx context.Context, scope, key string, payload []byte, ttl time.Duration) error {
	if key == "" {
		return nil
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO idempotency_key (scope, key, response_payload, created_at, expires_at)
		 VALUES ($1, $2, $3, now(), now() + $4::interval)
		 ON CONFLICT (scope, key) DO NOTHING`,
		scope, key, payload, formatInterval(ttl),
	)
	return err
}

func formatInterval(d time.Duration) string {
	// Postgres interval literal — `<seconds> seconds`.
	return time.Duration(d.Seconds()).String() + "s"
}
