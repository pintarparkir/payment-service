// Package worker holds background loops. Just outbox publisher for now;
// the reconciliation cron lives behind a separate cobra subcommand (feature 03).
package worker

import (
	"context"
	"time"

	"github.com/farid/payment-service/internal/payment/repository"
	"github.com/farid/payment-service/pkg/logger"
	"github.com/farid/payment-service/pkg/rabbit"
)

type OutboxPublisher struct {
	repo      repository.OutboxRepository
	publisher *rabbit.Publisher
	interval  time.Duration
	batch     int
}

func NewOutboxPublisher(repo repository.OutboxRepository, p *rabbit.Publisher) *OutboxPublisher {
	return &OutboxPublisher{repo: repo, publisher: p, interval: time.Second, batch: 200}
}

// Run continuously drains outbox_event into RabbitMQ. Same protocol as
// reservation-service / billing-service: at-least-once delivery, consumers
// must be idempotent.
func (w *OutboxPublisher) Run(ctx context.Context) {
	t := time.NewTicker(w.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			w.tick(ctx)
		}
	}
}

func (w *OutboxPublisher) tick(ctx context.Context) {
	rows, err := w.repo.FetchUnpublished(ctx, w.batch)
	if err != nil {
		logger.Error(ctx, "payment outbox: fetch failed",
			map[string]interface{}{logger.ErrorKey: err.Error()})
		return
	}
	if len(rows) == 0 {
		return
	}
	var published []int64
	for _, r := range rows {
		if err := w.publisher.Publish(ctx, r.EventType, r.Payload); err != nil {
			logger.Error(ctx, "payment outbox: publish failed",
				map[string]interface{}{
					"id":         r.ID,
					"event_type": r.EventType,
					logger.ErrorKey: err.Error(),
				})
			break
		}
		published = append(published, r.ID)
	}
	if err := w.repo.MarkPublished(ctx, published); err != nil {
		logger.Error(ctx, "payment outbox: mark failed",
			map[string]interface{}{logger.ErrorKey: err.Error()})
	}
}
