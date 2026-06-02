# Sequence Diagrams — ParkirPintar Business Flows

> **Tujuan:** Dokumentasi sequence diagram untuk setiap business flow di sistem ParkirPintar.  
> **Scope:** End-to-end flows antar service (reservation, billing, payment, notification).  
> **Author:** Solution Architecture · **Last Updated:** 2026-06-01

---

## 📋 Index

| # | Flow Name | Document | Description |
|---|-----------|----------|-------------|
| 1 | **Reservation Flow** | [01-reservation-flow.md](./01-reservation-flow.md) | Spot selection, reservation creation, outbox publishing |
| 2 | **Check-in Flow** | [02-checkin-flow.md](./02-checkin-flow.md) | Geofence validation, state transition PENDING→ACTIVE |
| 3 | **Billing & Checkout Flow** | [03-billing-checkout-flow.md](./03-billing-checkout-flow.md) | Invoice closing, pricing engine execution |
| 4 | **Cancellation Flow** | [04-cancellation-flow.md](./04-cancellation-flow.md) | User/system cancellation with fee calculation |
| 5 | **Payment Flow (QRIS)** | [05-payment-flow.md](./05-payment-flow.md) | QRIS intent creation, Midtrans webhook processing |
| 6 | **Notification Flow** | [06-notification-flow.md](./06-notification-flow.md) | Event consumption, MSISDN resolution, SMS dispatch |
| 7 | **End-to-End Full Journey** | [99-end-to-end-flow.md](./99-end-to-end-flow.md) | Complete user journey: Reserve → Check-in → Check-out → Pay |

---

## 🔍 How to Use These Diagrams

### View in Browser

1. **Mermaid Live Editor**: Copy any `mermaid` block and paste at https://mermaid.live/
2. **VS Code Extension**: Install "Markdown Preview Mermaid Support" for live rendering
3. **GitHub/GitLab**: These render automatically in `.md` files

### Export as Images

```bash
# Using mermaid CLI (npm install -g @mermaid-js/mermaid-cli)
mmdc -i 01-reservation-flow.md -o 01-reservation-flow.png -b transparent
```

### Integration with Service READMEs

Each service README contains a condensed version of relevant flows:

- `user-service/README.md`: Identity role in flows
- `reservation-service/README.md`: Reservation, check-in, check-out, cancel flows
- `billing-service/README.md`: Invoice lifecycle flows
- `payment-service/README.md`: Payment intent and webhook flows
- `notification-service/README.md`: Event-driven notification flows

---

## 🏗️ Legend & Conventions

### Participants

| Icon | Participant | Description |
|------|-------------|-------------|
| 👤 / actor | External User / Driver | Human user initiating actions |
| 📱 Mini-App | Client Application | Mini-app running in super-app |
| 🔌 LB | Load Balancer | Entry point gateway |
| 💾 DB | Database | Postgres instance (per service) |
| 🔧 Cache | Redis Cache | In-memory caching layer |
| 🐰 RMQ | RabbitMQ | Message broker for async events |
| 🌐 External API | Third-party service | Midtrans, Telkomsel SMS Gateway |

### Notation

- **Solid arrow** (`─▶`): Synchronous call / request
- **Dashed arrow** (`──▶`): Asynchronous message / event
- **Red rectangle**: Critical path or user-visible operation
- **Green rectangle**: Background worker / background task
- **Orange rectangle**: External API call with latency
- **Purple box**: State machine transition
- **Gray box**: Idempotency check / security verification

### Color Coding by Service

| Color | Service |
|-------|---------|
| Blue | user-service |
| Purple | reservation-service |
| Green | billing-service |
| Orange | payment-service |
| Red | notification-service |

---

## 🛠️ Tools & Technologies Referenced

| Tool | Purpose | Reference |
|------|---------|-----------|
| **PostgreSQL 16** | Relational database | [`docs/architecture/erd/`](../erd/) |
| **Redis 7** | Caching + distributed locks | [`docs/architecture/service-communication/`](../service-communication/) |
| **RabbitMQ 3.13** | Async eventing (outbox pattern) | [`docs/gap-analysis/02-resilience-and-reliability.md`](../../gap-analysis/02-resilience-and-reliability.md) |
| **Midtrans QRIS** | Payment gateway | [`payment-service/README.md`](../../../payment-service/README.md) |
| **Telkomsel SMS** | SMS gateway provider | [`notification-service/README.md`](../../../notification-service/README.md) |
| **OpenTelemetry** | Distributed tracing | [`docs/gap-analysis/04-observability.md`](../../gap-analysis/04-observability.md) |

---

## 🔄 Relationship with Other Documentation

| Type | Location | Links To |
|------|----------|----------|
| **Architecture Overview** | [`docs/architecture/high-level-design/00-system-overview.md`](../high-level-design/00-system-overview.md) | Component views, C4 diagrams |
| **API Contract** | [`docs/api-documentation/`](../api-documentation/) | REST/gRPC endpoints |
| **ERD** | [`docs/architecture/erd/`](../erd/) | Data models per service |
| **Service Communication** | [`docs/architecture/service-communication/`](../service-communication/) | Sync vs async patterns |
| **Gap Analysis** | [`docs/gap-analysis/`](../../gap-analysis/) | Known issues + improvements |
| **Implementation Todo** | [`docs/implementation-todo/`](../../implementation-todo/) | Task backlog (T-001..T-016) |

---

## ✅ Design Principles Applied

1. **Idempotency**: All write operations idempotent via key-based deduplication
2. **Outbox Pattern**: Transactional event publishing (no 2PC needed)
3. **State Machine Validation**: All transitions validated before DB write
4. **Double-Book Prevention**: PostgreSQL EXCLUDE constraint + Redis lock
5. **PII Encryption**: pgcrypto symmetric encryption at rest (phone/email)
6. **Graceful Degradation**: Notification failures don't block core flows
7. **Circuit Breaker**: External API calls protected (Midtrans, SMS gateway)

---

## 📝 Revision History

| Date | Version | Change | Author |
|------|---------|--------|--------|
| 2026-06-01 | 1.0 | Initial sequence diagram documentation | Solution Architecture |

---

_For questions or feedback, refer to the main project documentation or open an issue._
