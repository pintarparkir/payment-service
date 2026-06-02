# Billing & Checkout Flow - Sequence Diagram

> **Purpose:** Detailed flow for invoice closing with pricing engine and payment intent  
> **Related Docs:** [`billing-service/README.md`](../../../billing-service/README.md) · [`reservation-service/README.md`](../../../reservation-service/README.md)  
> **Author:** Solution Architecture · **Last Updated:** 2026-06-01

---

## 📖 Overview

Flow ini mencakup:
1. **Check-out** — Driver leaves parking spot, reservation state → COMPLETED
2. **Pricing engine** — Pure-functional calculation of parking fees
3. **Invoice closing** — Invoice status OPEN → CLOSED with line items
4. **Event propagation** — `billing.invoice.closed.v1` → notification

### Participants

| Name | Type | Responsibility |
|------|------|----------------|
| 👤 Driver | Actor | Initiates check-out |
| 📱 Mini-App | Client | Sends check-out request, displays total |
| 💾 Reservation Service | gRPC | State machine transition |
| 💾 Billing Service | gRPC | Invoice lifecycle + pricing |
| 🔧 Pricing Engine | Pure function | Deterministic fee calculation |
| 💾 Postgres DB | Relational | Invoice + line item persistence |
| 🔧 RabbitMQ | Message Queue | Async event delivery |
| 💾 Notification Service | Consumer | SMS receipt dispatch |

---

## 🔢 Sequence Diagram

```mermaid
sequenceDiagram
    autonumber
    actor 👤 as 👤 Driver
    participant 📱 as 📱 Mini-App
    participant 💾_Res as 💾 Reservation Service
    participant 💾_DB as 💾 Postgres DB
    participant 🔧_RMQ as 🔧 RabbitMQ
    participant 💾_Bill as 💾 Billing Service
    participant 🔧_Price as 🔧 Pricing Engine
    participant 💾_Notif as 💾 Notification Service
    
    Note over 👤,💾_Notif: ──────────────────────────────────────────────
    PHASE 1: Driver Checks Out
    Note over 👤,💾_Notif: ──────────────────────────────────────────────
    
    👤->>📱: Tap "Check-Out"
    Note right of 📱: Driver about to leave building
    
    📱->>💾_Res: POST /v1/reservations/{id}/check-out
    activate 💾_Res
    
    %% Step 1: Validate and update reservation state
    💾_Res->>💾_DB: BEGIN TRANSACTION
    activate 💾_DB
    
    💾_Res->>💾_DB: SELECT state, checked_in_at<br/>FROM reservation WHERE id = ? FOR UPDATE
    
    alt State != 'ACTIVE'
        alt State == 'COMPLETED'
            💾_Res-->>📱: 400 Bad Request "Already checked out"
        else State == 'CANCELLED'
            💾_Res-->>📱: 400 Bad Request "Reservation cancelled"
        else State == 'PENDING' or 'CONFIRMED'
            💾_Res-->>📱: 400 Bad Request "Must check-in first"
        end
        💾_Res->>💾_DB: ROLLBACK
        deactivate 💾_Res
        deactivate 💾_DB
        return
    end
    
    💾_Res->>💾_Res: Validate state transition:<br/>CanTransition('ACTIVE', 'COMPLETED')
    Note right of 💾_Res: pkg/state validation gate
    
    💾_Res->>💾_DB: UPDATE reservation SET<br/>state = 'COMPLETED',<br/>checked_out_at = NOW()<br/>WHERE id = ?
    
    %% Outbox: publish event
    💾_Res->>💾_DB: INSERT INTO outbox_event (<br/>topic='reservation.checked_out.v1',<br/>payload={reservation_id, driver_id,<br/>checked_in_at, checked_out_at})
    
    💾_Res->>💾_DB: COMMIT
    deactivate 💾_DB
    
    💾_Res-->>📱: 200 OK "Check-out successful. Proceed to payment."
    deactivate 💾_Res
    
    Note over 👤,💾_Notif: ──────────────────────────────────────────────
    PHASE 2: Outbox Publisher → Event Delivery
    Note over 👤,💾_Notif: ──────────────────────────────────────────────
    
    💾_Res->>🔧_RMQ: PUBLISH reservation.checked_out.v1<br/>{reservation_id, driver_id,<br/>checked_in_at, checked_out_at,<br/>vehicle_type: 'CAR', spot_id: 'F2-C-014'}
    
    🔧_RMQ-->💾_Bill: CONSUME reservation.checked_out.v1
    activate 💾_Bill
    
    Note over 👤,💾_Notif: ──────────────────────────────────────────────
    PHASE 3: Pricing Engine Execution
    Note over 👤,💾_Notif: ──────────────────────────────────────────────
    
    %% Step 1: Fetch invoice & reservation data
    💾_Bill->>💾_DB: BEGIN TRANSACTION
    activate 💾_DB
    
    💾_Bill->>💾_DB: SELECT * FROM invoice<br/>WHERE reservation_id = ? FOR UPDATE
    💾_DB-->>💾_Bill: {id: INV-001, status: 'OPEN',<br/>reservation_id: uuid, driver_id: uuid}
    
    alt Invoice not found
        💾_Bill-->🔧_RMQ: NACK + REQUEUE (transient error)
        deactivate 💾_Bill
        return
    end
    
    alt Invoice status != 'OPEN'
        Note right of 💾_Bill: Already closed (idempotent)
        💾_Bill->>💾_DB: COMMIT
        💾_Bill-->🔧_RMQ: ACK
        deactivate 💾_Bill
        return
    end
    
    %% Step 2: Invoke pricing engine
    💾_Bill->>🔧_Price: CalculatePricing({<br/>checkedInAt: '2026-06-01T08:00:00Z',<br/>checkedOutAt: '2026-06-01T10:30:00Z',<br/>vehicleType: 'CAR',<br/>cancelled: false,<br/>noShow: false<br/>})
    
    activate 🔧_Price
    
    Note right of 🔧_Price: ┌──────────────────────────────────────────┐
                             │ Pure Functional Pricing Engine          │
                             │                                          │
                             │ Input: {checkedIn, checkedOut, type}    │
                             │ Output: {lines[], total}                │
                             │                                          │
                             │ No I/O, No time.Now(), No side-effects  │
                             │ Fully deterministic, ~250 LOC           │
                             └──────────────────────────────────────────┘
    
    🔧_Price->>🔧_Price: calcDuration = 2.5 hours
    
    %% Rate table (vehicle-type dependent)
    Note right of 🔧_Price: Rates (CAR):<br/>- BOOKING: IDR 2,000<br/>- HOURLY: IDR 5,000/h<br/>- OVERNIGHT: IDR 30,000 flat (22:00-06:00)<br/>- CANCELLATION: booking fee<br/>- NOSHOW: IDR 5,000 penalty
    
    %% CAR rate calculation
    alt duration <= 1 hour
        🔧_Price->>🔧_Price: BOOKING = 2000<br/>HOURLY = 5000
    else duration <= 3 hours
        🔧_Price->>🔧_Price: BOOKING = 2000<br/>HOURLY = ceil(2.5) * 5000 = 12500
    else duration > 3 hours
        🔧_Price->>🔧_Price: BOOKING = 2000<br/>HOURLY = 3 * 5000 = 15000<br/>EXTRA = (duration - 3) * 5000
    end
    
    %% Overnight check
    alt checkedOut crosses 22:00-06:00
        🔧_Price->>🔧_Price: Apply OVERNIGHT flat = 30000
    end
    
    🔧_Price-->>💾_Bill: PricingOutput{<br/>lines: [{kind: BOOKING, amount: 2000},<br/>{kind: HOURLY, amount: 12500}],<br/>total: 14500<br/>}
    deactivate 🔧_Price
    
    Note over 👤,💾_Notif: ──────────────────────────────────────────────
    PHASE 4: Close Invoice
    Note over 👤,💾_Notif: ──────────────────────────────────────────────
    
    %% Step 3: Update invoice
    💾_Bill->>💾_DB: UPDATE invoice SET<br/>total_idr = 14500,<br/>status = 'CLOSED',<br/>closed_at = NOW()<br/>WHERE id = INV-001
    
    %% Step 4: Insert line items
    💾_Bill->>💾_DB: INSERT INTO invoice_line (<br/>invoice_id, kind, amount,<br/>description, created_at)<br/>VALUES (INV-001, 'BOOKING', 2000,<br/>'Booking fee CAR', NOW())
    
    💾_Bill->>💾_DB: INSERT INTO invoice_line (<br/>invoice_id, kind, amount,<br/>description, created_at)<br/>VALUES (INV-001, 'HOURLY', 12500,<br/>'2.5 hours parking x 5000', NOW())
    
    %% Step 5: Outbox event
    💾_Bill->>💾_DB: INSERT INTO outbox_event (<br/>topic='billing.invoice.closed.v1',<br/>payload={invoice_id: INV-001,<br/>reservation_id: uuid,<br/>driver_id: uuid,<br/>total_idr: 14500,<br/>closed_at: NOW()})
    
    💾_Bill->>💾_DB: COMMIT
    deactivate 💾_DB
    
    Note over 👤,💾_Notif: ──────────────────────────────────────────────
    PHASE 5: Invoice Closed → Notification
    Note over 👤,💾_Notif: ──────────────────────────────────────────────
    
    %% Send invoice closed event
    💾_Bill->>🔧_RMQ: PUBLISH billing.invoice.closed.v1
    deactivate 💾_Bill
    
    %% Deliver to notification
    🔧_RMQ-->💾_Notif: CONSUME billing.invoice.closed.v1
    activate 💾_Notif
    
    💾_Notif->>💾_Notif: Resolve MSISDN (from cache or gRPC)
    💾_Notif->>💾_Notif: Render SMS template:<br/>"Tagihan parkir IDR 14.500. Silakan bayar via menu Payment di aplikasi."
    
    💾_Notif->>SMS Gateway: POST /send_sms
    deactivate 💾_Notif
    
    Note over 👤,💾_Notif: ──────────────────────────────────────────────
    Summary: ✅ Checkout complete, invoice closed
    Note over 👤,💾_Notif: Total fees: IDR 14,500
    Note over 👤,💾_Notif: Next step: User pays via QRIS (payment-flow)
    Note over 👤,💾_Notif: ──────────────────────────────────────────────
```

---

## 💲 Pricing Engine Rules

### Rate Table

| Fee Type | Condition | CAR (IDR) | MOTORCYCLE (IDR) |
|----------|-----------|-----------|-------------------|
| **BOOKING** | Always, flat fee | 2,000 | 1,000 |
| **HOURLY** | Per hour (or fraction) | 5,000/h | 2,000/h |
| **OVERNIGHT** | 22:00 - 06:00 next day | 30,000 flat | 15,000 flat |
| **CANCELLATION** | Within 30min of hold expiry | Booking fee | Booking fee |
| **NOSHOW** | Reservation expired | 5,000 penalty | 2,500 penalty |

### Unit Test Cases

```go
func TestPricingEngine(t *testing.T) {
    tests := []struct {
        name     string
        input    PricingInput
        expected PricingOutput
    }{
        {
            name: "car_short_stay_1hour",
            input: PricingInput{
                CheckedInAt:  time.Date(2026, 6, 1, 8, 0, 0, 0, time.UTC),
                CheckedOutAt: time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC),
                VehicleType:  "CAR",
            },
            expected: PricingOutput{
                Lines: []Line{
                    {Kind: "BOOKING", Amount: 2000},
                    {Kind: "HOURLY",  Amount: 5000},
                },
                Total: 7000,
            },
        },
        {
            name: "car_2_5_hours",
            input: PricingInput{
                CheckedInAt:  time.Date(2026, 6, 1, 8, 0, 0, 0, time.UTC),
                CheckedOutAt: time.Date(2026, 6, 1, 10, 30, 0, 0, time.UTC),
                VehicleType:  "CAR",
            },
            expected: PricingOutput{
                Lines: []Line{
                    {Kind: "BOOKING", Amount: 2000},
                    {Kind: "HOURLY",  Amount: 12500}, // 2.5h * 5000
                },
                Total: 14500,
            },
        },
        {
            name: "motorcycle",
            input: PricingInput{
                CheckedInAt:  time.Date(2026, 6, 1, 8, 0, 0, 0, time.UTC),
                CheckedOutAt: time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC),
                VehicleType:  "MOTORCYCLE",
            },
            expected: PricingOutput{
                Lines: []Line{
                    {Kind: "BOOKING", Amount: 1000},
                    {Kind: "HOURLY",  Amount: 4000}, // 2h * 2000
                },
                Total: 5000,
            },
        },
        // ... 13 test cases total
    }
}
```

---

## 🐛 Error Scenarios & Handling

| Scenario | Detection | Recovery |
|----------|-----------|----------|
| **Check-out on already completed reservation** | State != ACTIVE | Return 400, idempotent check |
| **Invoice already closed (idempotent)** | status = CLOSED | ACK event, skip processing |
| **Pricing engine invalid input** | Invalid VehicleType | ACK → DLQ (permanent error) |
| **Billing DB down during invoice close** | SQL connection error | NACK + requeue (will retry) |
| **Outbox publish failure** | RabbitMQ unreachable | Event stays unpublished; alert at 10k count |
| **Concurrent check-out on same reservation** | FOR UPDATE serializes | One succeeds, other waits & finds state=COMPLETED |

---

## 📊 Performance Characteristics

| Operation | Latency | Notes |
|-----------|---------|-------|
| API check-out response | < 50ms | Minimal work: state update + outbox insert |
| Pricing engine execution | < 1ms | Pure functional, no I/O |
| Invoice close (DB) | < 20ms | Single row update + 2 inserts |
| Outbox publish lag | < 5s | Poll interval-based |
| End-to-end billing flow | ~5-10s | Async via RabbitMQ |

---

## 🔄 Related Flows

| Previous Flow | This Flow | Next Flow |
|---------------|-----------|-----------|
| Check-in (02-checkin) | **Check-out & Billing** | Payment via QRIS (05-payment) |

---

## ✅ Success Criteria

- [ ] Invoice closed with correct total based on pricing rules
- [ ] Idempotent invoice close (same event delivered twice → same result)
- [ ] All line items persisted and auditable
- [ ] Eventually consistent: invoice closed within 5s of check-out
- [ ] Pricing engine deterministic: same time input → same total every run

---

_Last updated: 2026-06-01 · Owner: Solution Architecture_
