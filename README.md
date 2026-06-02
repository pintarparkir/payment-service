# payment-service

[![Security](https://sonarcloud.io/api/project_badges/measure?project=pintarparkir_payment-service&metric=security_rating)](https://sonarcloud.io/summary/new_code?id=pintarparkir_payment-service)
[![Reliability](https://sonarcloud.io/api/project_badges/measure?project=pintarparkir_payment-service&metric=reliability_rating)](https://sonarcloud.io/summary/new_code?id=pintarparkir_payment-service)
[![Maintainability](https://sonarcloud.io/api/project_badges/measure?project=pintarparkir_payment-service&metric=sqale_rating)](https://sonarcloud.io/summary/new_code?id=pintarparkir_payment-service)
[![Duplications](https://sonarcloud.io/api/project_badges/measure?project=pintarparkir_payment-service&metric=duplicated_lines_density)](https://sonarcloud.io/summary/new_code?id=pintarparkir_payment-service)
[![Coverage](https://sonarcloud.io/api/project_badges/measure?project=pintarparkir_payment-service&metric=coverage)](https://sonarcloud.io/summary/new_code?id=pintarparkir_payment-service)

**Cloud Run:** `https://payment-service-725nddkmwq-as.a.run.app`

## Architecture Overview

![Architecture](docs/PintarParkir.architecture.svg)

## E2E Flow

![Flow Diagram](docs/flow.diagram.svg)

## Sequence Diagrams

### Payment Flow (QRIS)

```mermaid
sequenceDiagram
    autonumber
    actor 👤 as 👤 Driver
    participant 📱 as 📱 Mini-App
    participant 💾 as 💾 Payment Service
    participant 🌐 as 🌐 Midtrans API
    participant 💾_DB as 💾 Postgres DB
    participant 🔧_RMQ as 🔧 RabbitMQ
    participant 💾_Notif as 💾 Notification Service
    
    Note over 👤,💾_Notif: ──────────────────────────────────────────────
    FLOW A: Create QRIS Payment Intent
    Note over 👤,💾_Notif: ──────────────────────────────────────────────
    
    👤->>📱: Tap "Pay with QRIS"<br/>Invoice: INV-001 | Amount: IDR 14,500
    
    📱->>💾: POST /v1/payments/qris-intent<br/>{invoice_id: "INV-001"}
    
    activate 💾
    
    %% Step 1: Validate invoice via billing-service
    💾->>💾_Billing: gRPC GetInvoice(invoice_id)
    activate 💾_Billing
    
    alt Invoice not found or not CLOSED
        💾_Billing-->>💾: NOT_FOUND or status != CLOSED
        💾-->>📱: 400 Bad Request "Invalid invoice"
        deactivate 💾
        return
    end
    
    💾_Billing-->>💾: {total_idr: 14500, driver_id, status: 'CLOSED'}
    deactivate 💾_Billing
    
    %% Step 2: Check existing payment for this invoice (idempotent)
    💾->>💾_DB: BEGIN TRANSACTION
    activate 💾_DB
    
    💾->>💾_DB: SELECT * FROM payment WHERE invoice_id = ?
    
    alt Already has PENDING payment
        💾_DB-->>💾: {qr_code, payment_url}
        💾->>💾_DB: COMMIT
        💾-->>📱: 200 OK {<br/>payment_id, qr_code,<br/>payment_url, expires_at<br/>}
        Note right of 💾: Returning existing QR code
        deactivate 💾
        deactivate 💾_DB
        return
        
    else Already has PAID payment
        💾_DB-->>💾: {status: 'PAID'}
        💾->>💾_DB: COMMIT
        💾-->>📱: 409 Conflict "Already paid"
        deactivate 💾
        deactivate 💾_DB
        return
    end
    
    %% Step 3: Create payment record
    💾->>💾_DB: INSERT INTO payment (<br/>invoice_id, driver_id,<br/>amount_idr=14500,<br/>status='PENDING',<br/>idempotency_key=uuid<br/>)
    
    💾->>💾_DB: COMMIT
    deactivate 💾_DB
    
    %% Step 4: Call Midtrans QRIS API
    alt Stub Mode (development)
        Note right of 💾: MIDTRANS_STUB_MODE=true
        💾->>💾: Generate stub QRIS response
        💾->>💾: QRIS payload simulated
    else Production Mode
        💾->>🌐: POST https://api.midtrans.com/v2/charge<br/>{<br/>payment_type: "qris",<br/>transaction_id: payment_id,<br/>gross_amount: 14500,<br/>customer_details: {...}<br/>}
        activate 🌐
        
        💾->>💾: Circuit Breaker check
        Note right of 💾: sony/gobreaker protects Midtrans call
        
        alt Circuit closed (Midtrans healthy)
            🌐-->>💾: {<br/>status_code: "201",<br/>qr_code: "base64-encoded-qr",<br/>actions: [{url, method}]
            deactivate 🌐
        else Circuit open (Midtrans down)
            🌐-->>💾: Error: circuit breaker open
            💾-->>📱: 503 Service Unavailable "Payment gateway unavailable"
            deactivate 💾
            return
        end
    end
    
    %% Step 5: Store Midtrans response
    💾->>💾_DB: UPDATE payment SET<br/>pg_reference = ?,<br/>qr_payload = ?,<br/>expires_at = ?<br/>WHERE id = ?
    
    %% Step 6: Return QR code to mini-app
    💾-->>📱: 200 OK {<br/>payment_id,<br/>invoice_id,<br/>qris_image_url: "...",<br/>qr_code (base64),<br/>amount: 14500,<br/>expires_at: '2026-06-01T11:05Z'<br/>}
    deactivate 💾
    
    📱->>📱: Render QR code image on screen
    
    Note over 👤,💾_Notif: ⏳ User scans QRIS via mobile banking (BCA/Mandiri/GoPay)
    Note over 👤,💾_Notif: ──────────────────────────────────────────────
```
```mermaid
sequenceDiagram
    autonumber
    participant 🌐 as 🌐 Midtrans API
    participant 💾 as 💾 Payment Service
    participant 💾_DB as 💾 Postgres DB
    participant 🔧_RMQ as 🔧 RabbitMQ
    participant 💾_Notif as 💾 Notification Service
    
    Note over 🌐,💾_Notif: ──────────────────────────────────────────────
    FLOW B: Midtrans Webhook Processing
    Note over 🌐,💾_Notif: ──────────────────────────────────────────────
    
    🌐->>💾: POST /v1/payments/webhook/midtrans<br/>{<br/>transaction_id,<br/>transaction_status: "capture",<br/>gross_amount: "14500",<br/>signature_key: "...",<br/>order_id: payment_id<br/>}
    Note left of 🌐: Midtrans sends webhook after<br/>user completes QRIS scan
    
    activate 💾
    
    %% Step 1: Read raw body for signature verification
    %% ⚠ Critical: Must read io.ReadAll(request.Body) before JSON parse
    
    %% Step 2: HMAC-SHA512 Verification
    💾->>💾: Read X-Signature header
    💾->>💾: Compute HMAC-SHA512(SERVER_KEY, raw_body)
    💾->>💾: crypto/subtle.ConstantTimeCompare(computed, signature)
    
    Note right of 💾: ┌──────────────────────────────────────────┐
                       │ Webhook Security                        │
                       │                                          │
                       │ 1. signature = req.Header("X-Signature") │
                       │ 2. body = io.ReadAll(req.Body)           │
                       │ 3. computed = HMAC-SHA512(key, body)     │
                       │ 4. valid = ConstantTimeCompare(computed, │
                       │            signature)                    │
                       │ 5. If !valid → 401 + alert              │
                       └──────────────────────────────────────────┘
    
    alt Signature INVALID
        💾-->>🌐: 401 Unauthorized
        Note right of 💾: ⚠ Security alert logged
        deactivate 💾
        return
    end
    
    %% Step 3: Idempotency check on pg_reference
    💾->>💾_DB: BEGIN TRANSACTION
    activate 💾_DB
    
    💾->>💾_DB: SELECT * FROM payment WHERE pg_reference = ?
    
    alt Already processed (terminal status)
        💾_DB-->>💾: status = 'PAID'
        Note right of 💾: Webhook replay detected!<br/>Return 200 with same response for idempotency
        💾->>💾_DB: COMMIT
        💾-->>🌐: 200 OK {status: 'PAID'}
        deactivate 💾
        deactivate 💾_DB
        return
    end
    
    %% Step 4: Determine payment status from webhook
    alt Transaction captured/settled (PAID)
        💾->>💾_DB: UPDATE payment SET<br/>status = 'PAID',<br/>paid_at = NOW(),<br/>pg_reference = ?,<br/>raw_response = ?
        
        💾->>💾_DB: INSERT INTO outbox_event (<br/>topic='payment.paid.v1',<br/>payload={payment_id, invoice_id,<br/>driver_id, amount_idr: 14500,<br/>paid_at}<br/>)
        
        Note right of 💾: Webhook status: 'capture' or 'settlement'
        
    else Transaction denied/expired/failed (FAILED)
        💾->>💾_DB: UPDATE payment SET<br/>status = 'FAILED',<br/>failed_at = NOW(),<br/>failure_reason = ?
        
        💾->>💾_DB: INSERT INTO outbox_event (<br/>topic='payment.failed.v1',<br/>payload={payment_id, invoice_id,<br/>driver_id, reason}<br/>)
    end
    
    💾->>💾_DB: COMMIT
    deactivate 💾_DB
    
    %% Step 5: Acknowledge webhook
    💾-->>🌐: 200 OK {<br/>status_code: "200",<br/>message: "ok",<br/>transaction_status: "capture"<br/>}
    deactivate 💾
    
    %% Step 6: Async outbox publishing + notification
    par Outbox Publisher
        💾->>🔧_RMQ: PUBLISH payment.paid.v1
        🔧_RMQ-->💾_Notif: CONSUME payment.paid.v1
        activate 💾_Notif
        
        💾_Notif->>💾_Notif: Render SMS template:<br/>"Pembayaran berhasil Rp14.500. Terima kasih!"
        💾_Notif->>SMS Gateway: POST /send_sms
        
        deactivate 💾_Notif
    end
    
    Note over 🌐,💾_Notif: ──────────────────────────────────────────────
    Summary: ✅ Payment processed successfully
    Note over 🌐,💾_Notif: Amount: IDR 14,500 | Status: PAID
    Note over 🌐,💾_Notif: Receipt SMS sent to driver
    Note over 🌐,💾_Notif: ──────────────────────────────────────────────
```

<details>
<summary>More sequence diagrams</summary>

- [All Sequence Diagrams](docs/sequence-diagrams/)
</details>

---



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

## Business Flow Logic

### Payment Flow (QRIS Intent & Webhook)

Payment-service mengelola dua flow utama:
1. **QRIS Intent Creation** — Driver meminta QR code untuk pembayaran
2. **Webhook Processing** — Midtrans mengkonfirmasi status pembayaran

```mermaid
sequenceDiagram
    autonumber
    actor Driver as 👤 Driver
    participant MiniApp as 📱 Mini-App
    participant PaySvc as Payment Service
    participant DB as Postgres DB
    participant Midtrans as Midtrans API
    participant RMQ as RabbitMQ
    participant Notif as Notification Service
    
    Note over Driver,Notif: Flow 1: Create QRIS Intent
    
    Driver->>MiniApp: Tap "Pay Now"
    MiniApp->>PaySvc: POST /v1/payments/qris-intent<br/>{invoice_id, amount: 14500}
    
    activate PaySvc
    PaySvc->>PaySvc: Validate invoice exists (gRPC to billing)
    
    PaySvc->>DB: BEGIN
    PaySvc->>DB: SELECT * FROM payment WHERE invoice_id = ? AND status = 'PENDING'
    
    alt Payment exists (reuse)
        DB-->>PaySvc: Existing payment
        PaySvc->>DB: COMMIT
        PaySvc-->>MiniApp: 200 OK {qr_code, payment_id}
    else No payment
        PaySvc->>DB: INSERT INTO payment (<br/>invoice_id, amount, status='PENDING')
        PaySvc->>DB: COMMIT
        
        %% Call Midtrans
        PaySvc->>Midtrans: POST /charge<br/>{transaction_id, gross_amount}
        activate Midtrans
        Midtrans-->>PaySvc: {qr_code, payment_url, expires_at}
        deactivate Midtrans
        
        PaySvc->>DB: UPDATE payment SET pg_reference = ?
        PaySvc-->>MiniApp: 200 OK {qr_code, payment_url, expires_at}
    end
    
    deactivate PaySvc
    
    Note over Driver,Notif: ⏳ Driver scans QRIS via banking app
    
    Note over Driver,Notif: Flow 2: Webhook Processing
    
    Midtrans->>PaySvc: POST /v1/payments/webhook/midtrans<br/>{order_id, status, signature}
    
    activate PaySvc
    
    %% HMAC verification
    PaySvc->>PaySvc: Compute HMAC-SHA512(SERVER_KEY, raw_body)
    PaySvc->>PaySvc: ConstantTimeCompare(computed, signature)
    
    alt Signature invalid
        PaySvc-->>Midtrans: 401 Unauthorized
        PaySvc->>PaySvc: Log security alert
        deactivate PaySvc
        return
    end
    
    %% Idempotency check
    PaySvc->>DB: SELECT * FROM payment WHERE pg_reference = ?
    
    alt Status terminal (PAID/FAILED)
        DB-->>PaySvc: Payment already processed
        PaySvc-->>Midtrans: 200 OK (idempotent)
        deactivate PaySvc
        return
    end
    
    %% Update payment status
    PaySvc->>DB: BEGIN
    
    alt status = 'capture' or 'settlement'
        PaySvc->>DB: UPDATE payment SET<br/>status='PAID', paid_at=NOW()
        PaySvc->>DB: INSERT INTO outbox_event(<br/>topic='payment.paid.v1',<br/>payload={payment_id, invoice_id, amount})
    else status = 'deny' or 'cancel' or 'expire'
        PaySvc->>DB: UPDATE payment SET<br/>status='FAILED', failed_at=NOW()
        PaySvc->>DB: INSERT INTO outbox_event(<br/>topic='payment.failed.v1')
    end
    
    PaySvc->>DB: COMMIT
    
    PaySvc-->>Midtrans: 200 OK
    deactivate PaySvc
    
    %% Async notification
    par Outbox Publisher
        PaySvc->>DB: SELECT * FROM outbox_event WHERE published_at IS NULL
        PaySvc->>RMQ: PUBLISH payment.paid.v1
        PaySvc->>DB: UPDATE outbox_event SET published_at = NOW()
        
        RMQ-->Notif: CONSUME payment.paid.v1
        Notif->>Notif: Render SMS "Pembayaran berhasil IDR 14,500"
        Notiv->>SMS Gateway: Send SMS
    end
```

### Payment State Machine

```
┌─────────────┐
│   PENDING   │ ← Create QRIS Intent
└──────┬──────┘
       │
       ├─── Midtrans webhook: capture/settlement ───▶ ┌─────────────┐
       │                                               │    PAID     │
       │                                               └─────────────┘
       │
       ├─── Midtrans webhook: deny/cancel/expire ───▶ ┌─────────────┐
       │                                               │   FAILED    │
       │                                               └─────────────┘
       │
       └─── Manual timeout (24h) ────────────────────▶ ┌─────────────┐
                                                       │   EXPIRED   │
                                                       └─────────────┘

States PAID, FAILED, EXPIRED are terminal (immutable).
```

### Webhook Security

| Step | Implementation |
|------|----------------|
| 1. Read raw body | Read `io.ReadAll(request.Body)` before JSON parse |
| 2. Get signature | `X-Signature` header from Midtrans |
| 3. Compute HMAC | `HMAC-SHA512(SERVER_KEY, raw_body)` |
| 4. Compare | `crypto/subtle.ConstantTimeCompare(computed, signature)` |
| 5. Reject on mismatch | Return 401 + log security alert |

---

## Related Documentation

- **Architecture Overview:** [`../docs/README.md`](../docs/README.md)
- **Shared Infra Docs:** [`infra`](https://github.com/pintarparkir/infra)
- **API Documentation:** [`../docs/api-documentation/00-overview.md`](../docs/api-documentation/00-overview.md)
- **Implementation Backlog:** [`../docs/implementation-todo/00-backlog.md`](../docs/implementation-todo/00-backlog.md)

---

_For questions or issues, refer to the troubleshooting section in the main README or open an issue on the repo._
