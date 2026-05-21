// Package postgres implements payment repository using PostgreSQL.
package postgres

import (
	"context"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"

	"github.com/farid/payment-service/internal/payment/repository"
)

type outboxRepo struct{ db *sqlx.DB }

func NewOutboxRepository(db *sqlx.DB) repository.OutboxRepository { return &outboxRepo{db: db} }

const fetchUnpublishedSQL = `
SELECT id, event_type, payload
FROM outbox_event
WHERE published_at IS NULL
ORDER BY id
FOR UPDATE SKIP LOCKED
LIMIT $1
`

const markPublishedSQL = `UPDATE outbox_event SET published_at = now() WHERE id = ANY($1)`

func (r *outboxRepo) FetchUnpublished(ctx context.Context, limit int) ([]repository.OutboxRow, error) {
	rows, err := r.db.QueryxContext(ctx, fetchUnpublishedSQL, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []repository.OutboxRow
	for rows.Next() {
		var row repository.OutboxRow
		if err := rows.StructScan(&row); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *outboxRepo) MarkPublished(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := r.db.ExecContext(ctx, markPublishedSQL, pq.Array(ids))
	return err
}
