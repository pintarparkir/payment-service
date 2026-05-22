# payment-service

> **Purpose:** QRIS payment integration — owns payment intent creation, Midtrans webhook processing, and payment events.  
> **Author:** Farid Triwicaksono · **Last Updated:** 2026-05-21

## Project Overview

**ParkirPintar** is a backend mini-app for smart parking within a super-app. It handles:
- Availability queries (spots per floor, per vehicle type)
- Reservation creation (system-assigned or user-selected spots)
- Reservation state transitions (confirm, cancel, check-in, check-out)
- Geofence validation (GPS-based check-in)
- No-show expiration (automatic after 1 hour hold)
- Event publishing (outbox pattern → RabbitMQ)

Five services: **user**, **reservation**, **billing**, **payment** (this service), **notification**.

## Service Scope

**Owns:**
- QRIS payment intent creation
- Midtrans sandbox/production integration
- Webhook signature verification
- Payment state machine (PENDING → PAID/FAILED/EXPIRED)
- Payment outbox events (`payment.paid.v1`, `payment.failed.v1`)
- Circuit breaker around Midtrans HTTP calls

**Does NOT own:**
- Invoice amount calculation (billing-service owns)
- Reservation lifecycle (reservation-service owns)
- Driver identity (user-service owns)
- SMS dispatch (notification-service owns)

**Key invariants:**
- One payment intent per invoice unless explicitly retried
- Webhook signature verified before business logic
- Terminal payment states are immutable
- Webhook replay is idempotent on `pg_reference`

## At a Glance

| Aspect | Details |
|--------|---------|
| **REST Port** | 8083 (mini-app + Midtrans webhook) |
| **gRPC Port** | 9092 (s2s) |
| **Database** | PostgreSQL 16 (payment, outbox_event, idempotency_key) |
| **Cache** | Optional Redis for rate limiting (if enabled) |
| **Message Queue** | RabbitMQ 3.13 (payment event publishing) |
| **External APIs** | Midtrans QRIS (HTTP) |

## Tech Stack

- **Language:** Go 1.22
- **Web Framework:** Gin (REST) + gRPC
- **Database:** PostgreSQL 16 + sqlx
- **Message Queue:** RabbitMQ 3.13 (amqp091-go)
- **External HTTP:** Midtrans QRIS API
- **Resilience:** sony/gobreaker circuit breaker
- **Logging:** Zap + Lumberjack
- **Observability:** OpenTelemetry (OTLP/gRPC)
- **Testing:** testify/mock, table-driven tests

## Architecture

### High-Level Design
See [`../docs/architecture/high-level-design/04-payment-service.md`](../docs/architecture/high-level-design/04-payment-service.md) for:
- Service responsibilities and boundaries
- Midtrans integration flow
- Webhook processing and replay handling

### Low-Level Design
See [`../docs/architecture/low-level-design/04-payment-service-lld.md`](../docs/architecture/low-level-design/04-payment-service-lld.md) for:
- Layer cake (model → usecase → repository → handler)
- Webhook verification sequence
- Outbox event transaction boundaries

### Entity Relationship Diagram
See [`../docs/architecture/erd/04-payment-service.md`](../docs/architecture/erd/04-payment-service.md) for:
- Table schema (payment, outbox_event, idempotency_key)
- Unique constraints (`invoice_id`, `pg_reference`)
- Critical indexes

![ParkirPintar ERD](../user-service/ERD.jpg)

## API Reference

### REST Endpoints

| Method | Path | Description | Auth |
|--------|------|-------------|------|
| POST | `/v1/payments/qris/intent` | Create QRIS payment intent | Driver JWT |
| GET | `/v1/payments/{id}` | Get payment status | Driver JWT |
| POST | `/v1/payments/webhook/midtrans` | Midtrans webhook callback | HMAC signature |

### gRPC Services (s2s, internal only)

| RPC | Input | Output | Purpose |
|-----|-------|--------|---------|
| CreateQrisIntent | CreateQrisIntentRequest | Payment | Create QRIS payment intent |
| GetPayment | GetPaymentRequest | Payment | Lookup payment by ID/invoice_id |
| HandleWebhook | HandleWebhookRequest | HandleWebhookResponse | Process Midtrans callback |

### RabbitMQ Events (published via outbox)

| Event | Trigger | Payload |
|-------|---------|---------|
| `payment.paid.v1` | Midtrans settlement/capture | payment_id, invoice_id, driver_id, amount_idr, paid_at |
| `payment.failed.v1` | Midtrans deny/cancel/expire | payment_id, invoice_id, driver_id, failure_reason, failed_at |

## Sample Environment

```bash
# ── App ─────────────────────────────────────────────────────────────────────
APP_NAME=payment-service
APP_ENV=local
APP_PORT=8083        # REST port (mini app + webhook)
GRPC_PORT=9092       # gRPC port (s2s)

# ── Postgres ────────────────────────────────────────────────────────────────
DB_HOST=localhost
DB_PORT=5432
DB_USERNAME=postgres
DB_PASSWORD=postgres
DB_NAME=payment_service

# ── RabbitMQ (event publishing via outbox) ──────────────────────────────────
RABBIT_URL=amqp://guest:guest@localhost:5672/
RABBIT_EXCHANGE=parkirpintar.events

# ── Observability ────────────────────────────────────────────────────────────
OTLP_ENDPOINT=localhost:4317

# ── Midtrans QRIS sandbox ───────────────────────────────────────────────────
MIDTRANS_STUB_MODE=true
MIDTRANS_SERVER_KEY=SB-Mid-server-XXXXXXXX
MIDTRANS_BASE_URL=https://api.sandbox.midtrans.com/v2
MIDTRANS_WEBHOOK_SECRET=

# ── JWT verification ─────────────────────────────────────────────────────────
SUPER_APP_JWT_PUBLIC_KEY_PEM=
```

See `configs/.env.example` for full reference.

## Getting Started

### Prerequisites
- Docker 24+ & Docker Compose v2
- Go 1.22+ (for local development)
- `buf` CLI (for proto regeneration)
- Midtrans sandbox credentials (optional; stub mode works without credentials)

### Local Development

```bash
# 1. Clone and setup
git clone <repo> && cd <repo>
cd payment-service
cp configs/.env.example configs/.env

# 2. Start shared infra (see https://github.com/pintarparkir/infra)
cd ../infra && podman compose up -d

# 3. Run migrations
cd ../payment-service
make migrate-up

# 4. Run the service
make run

# 5. Verify health
curl http://localhost:8083/healthz
```

## Testing

### Unit Tests (no infra)
```bash
make test-unit
```
Covers: webhook signature verification, payment state transitions, idempotent replay, Midtrans client stub.

### All Tests
```bash
make test
```

### Coverage
```bash
go test -coverprofile=cov.out ./...
go tool cover -html=cov.out
```
Target: usecase ≥80%, repository ≥60%.

## Debugging

### Logs
```bash
LOG_LEVEL=debug make run
```
Logs are JSON-formatted with trace_id, span_id, request_id.

### Database
```bash
psql postgresql://postgres:postgres@localhost:5432/payment_service

# View schema
\dt

# Check payment status
SELECT id, invoice_id, pg_reference, status, amount_idr FROM payment ORDER BY created_at DESC LIMIT 10;

# Check outbox events
SELECT event_type, published_at, payload FROM outbox_event ORDER BY created_at DESC LIMIT 10;
```

### Midtrans Stub Mode
```bash
# Stub mode returns synthetic QRIS payload without calling Midtrans
MIDTRANS_STUB_MODE=true make run
```

### Webhook Replay
```bash
# Replay webhook body against local service
curl -X POST http://localhost:8083/v1/payments/webhook/midtrans \
  -H "Content-Type: application/json" \
  -H "X-Signature: <computed-signature>" \
  -d @fixtures/midtrans-paid.json
```

### RabbitMQ
- **Management UI:** http://localhost:15672 (guest/guest)
- **View exchange:** parkirpintar.events
- **View queues:** payment.* queues

## Operations

### Health Checks
```bash
# REST
curl http://localhost:8083/healthz

# gRPC
grpcurl -plaintext localhost:9092 grpc.health.v1.Health/Check
```

### Migrations
```bash
make migrate-up      # Apply all pending migrations
make migrate-down    # Rollback one migration
```

### Outbox Publisher
Background worker publishes unsent outbox events to RabbitMQ every 5 seconds. Check logs for `outbox: published` messages.

### Circuit Breaker
Midtrans calls are protected by a circuit breaker. When Midtrans is down, QRIS intent creation fails fast until the breaker half-opens.

## Security Notes

- **Secrets:** Never commit `.env` files. Use Secret Manager in production.
- **Webhook:** Verify signature with constant-time compare against raw body before business logic.
- **JWT:** Mini-app endpoints require driver JWT; webhook endpoint uses signature auth.
- **SQL:** All queries parameterized (sqlx prevents injection).
- **Rate limiting:** QRIS intent endpoint should be rate-limited per driver_id.

## Related Documentation

- **Architecture Overview:** [`../docs/README.md`](../docs/README.md)
- **Shared Infra Docs:** [`infra`](https://github.com/pintarparkir/infra)
- **API Documentation:** [`../docs/api-documentation/00-overview.md`](../docs/api-documentation/00-overview.md)
- **Implementation Backlog:** [`../docs/implementation-todo/00-backlog.md`](../docs/implementation-todo/00-backlog.md)

---

_For questions or issues, refer to the troubleshooting section in the main README or open an issue on the repo._
