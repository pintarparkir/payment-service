package rabbit

import (
	"context"
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

type Publisher struct {
	conn     *amqp.Connection
	ch       *amqp.Channel
	exchange string
}

func NewPublisher(amqpURL, exchange string) (*Publisher, error) {
	conn, err := amqp.Dial(amqpURL)
	if err != nil {
		return nil, fmt.Errorf("amqp dial: %w", err)
	}
	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("amqp channel: %w", err)
	}
	if err := ch.ExchangeDeclare(exchange, "topic", true, false, false, false, nil); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return nil, fmt.Errorf("exchange declare: %w", err)
	}
	return &Publisher{conn: conn, ch: ch, exchange: exchange}, nil
}

func (p *Publisher) Publish(ctx context.Context, routingKey string, body []byte) error {
	ctx, span := otel.Tracer("rabbit").Start(ctx, "publish "+routingKey,
		trace.WithSpanKind(trace.SpanKindProducer),
	)
	defer span.End()

	headers := make(amqp.Table)
	otel.GetTextMapPropagator().Inject(ctx, &amqpHeaderCarrier{headers})

	return p.ch.PublishWithContext(ctx, p.exchange, routingKey, false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Headers:      headers,
		Body:         body,
	})
}

func (p *Publisher) Close() {
	if p.ch != nil {
		_ = p.ch.Close()
	}
	if p.conn != nil {
		_ = p.conn.Close()
	}
}
