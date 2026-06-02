# Cancellation Flow - Sequence Diagram

> **Purpose:** Detailed flow for user-initiated and system-initiated cancellation  
> **Related Docs:** [`reservation-service/README.md`](../../../reservation-service/README.md) · [`billing-service/README.md`](../../../billing-service/README.md)  
> **Author:** Solution Architecture · **Last Updated:** 2026-06-01

---

## 📖 Overview

Flow ini mencakup:
1. **User cancellation** — Driver cancels before check-in
2. **No-show expiry** — System auto-expires reservation after hold period
3. **Fee calculation** — Cancellation fee vs free cancellation based on timing
4. **Event propagation** — `reservation.cancelled.v1` / `reservation.expired.v1` → billing + notification

### Participants

| Name | Type | Responsibility |
|------|------|----------------|
| 👤 Driver | Actor | Initiates cancellation (user flow) |
| 📱 Mini-App | Client | Sends cancel request, displays confirmation |
| 💾 Reservation Service | gRPC | State machine transition |
| 🔧 No-Show Worker | Background worker | Automatic expiry scanning |
| 💾 Billing Service | Consumer | Fee calculation |
| 💾 Postgres DB | Relational | Data persistence |
| 🔧 RabbitMQ | Message Queue | Async event delivery |
| 💾 Notification Service | Consumer | SMS dispatch |

---

## 🔢 Sequence Diagram - User Cancellation

```mermaid
sequenceDiagram
    autonumber
    actor 👤 as 👤 Driver
    participant 📱 as 📱 Mini-App
    participant 💾_Res as 💾 Reservation Service
    participant 💾_RM as 💾 State Machine
    participant 💾_DB as 💾 Postgres DB
    participant 🔧_RMQ as 🔧 RabbitMQ
    participant 💾_Bill as 💾 Billing Service
    participant 💾_Notif as 💾 Notification Service
    
    Note over 👤,💾_Notif: ──────────────────────────────────────────────
    FLOW A: User-Initiated Cancellation
    Note over 👤,💾_Notif: ──────────────────────────────────────────────
    
    👤->>📱: Tap "Cancel Reservation"
    Note right of 📱: Optional confirmation dialog
    
    📱->>💾_Res: POST /v1/reservations/{id}/cancel<br/>{reason: "schedule_change"}
    activate 💾_Res
    
    💾_Res->>💾_DB: BEGIN TRANSACTION
    activate 💾_DB
    
    %% Step 1: Fetch reservation with lock (serialize access)
    💾_Res->>💾_DB: SELECT * FROM reservation<br/>WHERE id = ? FOR UPDATE
    💾_DB-->>💾_Res: {state: 'CONFIRMED',<br/>hold_end: 2026-06-01T09:10:00Z,<br/>created_at: 2026-06-01T08:00:00Z}
    
    %% Step 2: Validate state transition via state machine
    💾_Res->>💾_RM: CanTransition('CONFIRMED', 'CANCELLED')
    💾_RM-->>💾_Res: true (valid transition)
    
    alt INVALID transition
        Note right of 💾_Res: e.g. ACTIVE → CANCELLED (not allowed)
        alt state == 'ACTIVE'
            💾_Res->>📱: 400 Bad Request "Cannot cancel active reservation"
        else state == 'COMPLETED'
            💾_Res->>📱: 400 Bad Request "Reservation already completed"
        else state == 'CANCELLED'
            💾_Res->>📱: 400 Bad Request "Already cancelled"
        else state == 'EXPIRED'
            💾_Res->>📱: 400 Bad Request "Already expired (no-show)"
        end
        💾_Res->>💾_DB: ROLLBACK
        deactivate 💾_Res
        deactivate 💾_DB
        return
    end
    
    %% Step 3: Update state to CANCELLED
    💾_Res->>💾_DB: UPDATE reservation SET<br/>state = 'CANCELLED',<br/>cancelled_at = NOW(),<br/>cancel_reason = 'schedule_change'<br/>WHERE id = ?
    
    %% Step 4: Outbox event
    💾_Res->>💾_DB: INSERT INTO outbox_event (<br/>topic='reservation.cancelled.v1',<br/>payload={reservation_id, driver_id,<br/>spot_id, reason, cancelled_at,<br/>hold_end, created_at}<br/>)
    Note right of 💾_Res: send hold_end in payload<br/>for billing fee calculation
    
    %% Step 5: Commit
    💾_Res->>💾_DB: COMMIT
    deactivate 💾_DB
    
    %% Success response
    💾_Res-->>📱: 200 OK {<br/>state: 'CANCELLED',<br/>cancelled_at: '2026-06-01T08:30:00Z',<br/>message: 'Reservation cancelled'<br/>}
    deactivate 💾_Res
    
    Note over 👤,💾_Notif: ──────────────────────────────────────────────
    PHASE 2: Background Processing
    Note over 👤,💾_Notif: ──────────────────────────────────────────────
    
    %% Outbox publisher
    💾_Res->>🔧_RMQ: PUBLISH reservation.cancelled.v1
    
    %% Parallel: Billing and Notification
    par Billing Service
        🔧_RMQ-->💾_Bill: CONSUME reservation.cancelled.v1
        activate 💾_Bill
        
        💾_Bill->>💾_Bill: Determine cancellation fee:<br/>cancelled_at: 08:30<br/>hold_end: 09:10<br/>timeBeforeExpiry = 40 min
        
        alt timeBeforeExpiry <= 30 min (within grace period)
            Note right of 💾_Bill: Retain booking fee as penalty
            💾_Bill->>💾_DB: INSERT INTO invoice_line (<br/>kind='CANCELLATION',<br/>amount=2000,<br/>description='Cancel fee')
        else timeBeforeExpiry > 30 min (early cancellation)
            Note right of 💾_Bill: No fee! Free cancellation
        end
        
        💾_Bill->>💾_DB: UPDATE invoice SET<br/>status='CLOSED',<br/>total_idr=COALESCE(fees, 0)
        💾_Bill->>💾_DB: COMMIT
        deactivate 💾_Bill
        
    and Notification Service
        🔧_RMQ-->💾_Notif: CONSUME reservation.cancelled.v1
        activate 💾_Notif
        
        💾_Notif->>💾_Notif: Resolve MSISDN (cache or gRPC)
        💾_Notif->>💾_Notif: Render SMS:<br/>"Reservasi Anda telah dibatalkan."
        
        deactivate 💾_Notif
    end
    
    Note over 👤,💾_Notif: ──────────────────────────────────────────────
    Summary: ✅ Cancellation processed
    Note over 👤,💾_Notif: Fee status: Free cancellation (early enough)
    Note over 👤,💾_Notif: ──────────────────────────────────────────────
```

---

## ⏰ Sequence Diagram - No-Show Expiry (System-Initiated)

```mermaid
sequenceDiagram
    autonumber
    participant 💾_Res as 💾 Reservation Service
    participant 🔧_Worker as 🔧 No-Show Worker
    participant 💾_DB as 💾 Postgres DB
    participant 🔧_RMQ as 🔧 RabbitMQ
    participant 💾_Bill as 💾 Billing Service
    participant 💾_Notif as 💾 Notification Service
    
    Note over 💾_Res,💾_Notif: ──────────────────────────────────────────────
    FLOW B: No-Show Expiry (Automatic)
    Note over 💾_Res,💾_Notif: ──────────────────────────────────────────────
    
    Note over 🔧_Worker: Ticker fires every 1 minute<br/>Scans for expired reservations
    
    loop Polling Loop (every 1 minute)
        🔧_Worker->>💾_DB: BEGIN
        activate 🔧_Worker
        
        %% Scan expired CONFIRMED reservations
        🔧_Worker->>💾_DB: SELECT id, driver_id, spot_id<br/>FROM reservation<br/>WHERE state = 'CONFIRMED'<br/>AND hold_end < NOW()<br/>ORDER BY hold_end ASC<br/>LIMIT 50<br/>FOR UPDATE SKIP LOCKED
        activate 💾_DB
        
        Note right of 💾_DB: ┌────────────────────────────────┐
                             │ FOR UPDATE SKIP LOCKED        │
                             │                               │
                             │ - Locks matched rows          │
                             │ - Skips rows locked by other  │
                             │   transactions                │
                             │ - Prevents double-expiry      │
                             └────────────────────────────────┘
        
        alt No expired rows found
            💾_DB-->>🔧_Worker: Empty result set
            🔧_Worker->>💾_DB: COMMIT
            deactivate 🔧_Worker
            deactivate 💾_DB
            return
        end
        
        💾_DB-->>🔧_Worker: [{id: uuid, driver_id: uuid,<br/>spot_id: F2-C-014}]
        deactivate 💾_DB
        
        %% Process each expired reservation
        loop Per expired row
            %% Step 1: Update state
            🔧_Worker->>💾_DB: UPDATE reservation SET<br/>state = 'EXPIRED',<br/>expired_at = NOW()<br/>WHERE id = ? AND state = 'CONFIRMED'
            
            %% Step 2: Outbox event
            🔧_Worker->>💾_DB: INSERT INTO outbox_event (<br/>topic='reservation.expired.v1',<br/>payload={reservation_id, driver_id,<br/>spot_id, expired_at}<br/>)
        end
        
        🔧_Worker->>💾_DB: COMMIT
        
        %% Release spot for reuse (already handled by EXCLUDE constraint on expiry)
        deactivate 💾_DB
        deactivate 🔧_Worker
        
        %% Publish events
        🔧_Worker->>🔧_RMQ: PUBLISH reservation.expired.v1
        
        %% Parallel consumers
        par Billing: Apply No-Show Fee
            🔧_RMQ-->💾_Bill: CONSUME reservation.expired.v1
            activate 💾_Bill
            
            💾_Bill->>💾_DB: BEGIN
            💾_Bill->>💾_DB: INSERT INTO invoice_line (<br/>kind='NOSHOW', amount=5000)
            💾_Bill->>💾_DB: UPDATE invoice SET<br/>total_idr=7000, status='CLOSED'
            Note right of 💾_Bill: Total: BOOKING(2000) + NOSHOW(5000) = 7000
            💾_Bill->>💾_DB: COMMIT
            deactivate 💾_Bill
            
        and Notification: Send SMS
            🔧_RMQ-->💾_Notif: CONSUME reservation.expired.v1
            activate 💾_Notif
            
            💾_Notif->>💾_Notif: Render SMS:<br/>"Reservasi expired. Fee no-show dikenakan."
            💾_Notif->>SMS: POST /send_sms
            deactivate 💾_Notif
        end
    end
    
    Note over 💾_Res,💾_Notif: ──────────────────────────────────────────────
    Summary: ✅ No-show expired, fee applied
    Note over 💾_Res,💾_Notif: No-show fees: BOOKING(2000) + PENALTY(5000) = 7000
    Note over 💾_Res,💾_Notif: ──────────────────────────────────────────────
```

---

## 🔑 Cancellation Fee Rules

| Condition | Fee | Rationale |
|-----------|-----|-----------|
| Cancel > 30 min before hold expiry | **Free** | User cancels early; spot can be rebooked |
| Cancel ≤ 30 min before hold expiry | **Booking fee** (IDR 2,000 CAR) | Spot likely can't be rebooked quickly |
| No-show (expired) | **Booking fee + penalty** (IDR 7,000 CAR) | Revenue loss + deterrent |
| System cancel (payment failed) | **Booking fee** | Partial compensation |

**Fee Calculation:**
```go
func CalculateCancelFee(cancelledAt, holdEnd time.Time, vehicleType string) int64 {
    timeBeforeExpiry := holdEnd.Sub(cancelledAt)
    
    if timeBeforeExpiry > 30*time.Minute {
        return 0 // Free cancellation
    }
    
    // Retain booking fee as penalty
    switch vehicleType {
    case "CAR":
        return 2000
    case "MOTORCYCLE":
        return 1000
    default:
        return 0
    }
}
```

**No-show Fee Calculation:**
```go
func CalculateNoShowFee(vehicleType string) (int64, int64) {
    // No-show = booking fee retained + no-show penalty
    switch vehicleType {
    case "CAR":
        return 2000 + 5000, 7000
    case "MOTORCYCLE":
        return 1000 + 2500, 3500
    default:
        return 0, 0
    }
}
```

---

## 🐛 Error Scenarios & Handling

| Scenario | Detection | Recovery |
|----------|-----------|----------|
| **Cancel active reservation** | State = ACTIVE | Return 400, must check-out first |
| **Cancel already processed reservation** | State = COMPLETED | Return 400, already finished |
| **Concurrent cancel (two requests)** | FOR UPDATE serializes | First succeeds, second finds state=CANCELLED |
| **No-show worker crashes mid-batch** | `SKIP LOCKED` + fresh poll | Next poll picks up unexpired rows |
| **Billing service unavailable** | Consumer NACK | Requeue for retry, eventual consistency |
| **User with no invoice** | Invoice not found | Billing consumer creates minimal invoice (T-001 saga) |

---

## 🔄 State Machine Transitions

```
                    User Cancel
    PENDING ──────────────────────────▶ CANCELLED
    CONFIRMED ────────────────────────▶ CANCELLED
    ACTIVE ───────────────────────────▶ (blocked: must complete)
    COMPLETED ────────────────────────▶ (blocked: terminal state)
    
    CONFIRMED ──[no-show worker]──▶ EXPIRED
    EXPIRED ───────────────────────▶ CANCELLED (if user appeals)
```

### Transitions Table

| From State | To State | Trigger | Valid? |
|------------|----------|---------|--------|
| PENDING | CANCELLED | User action | ✅ |
| CONFIRMED | CANCELLED | User action | ✅ |
| CONFIRMED | EXPIRED | No-show worker | ✅ |
| ACTIVE | CANCELLED | User action | ❌ |
| COMPLETED | CANCELLED | User action | ❌ |
| CANCELLED | EXPIRED | No-show worker | ❌ |
| EXPIRED | CANCELLED | User action | ⚠️ (beyond MVP) |

---

## 📊 Performance Characteristics

| Operation | Latency | Notes |
|-----------|---------|-------|
| User cancellation (API) | < 50ms | Minimal work: state update + outbox |
| No-show worker scan | < 200ms per batch | FOR UPDATE SKIP LOCKED |
| Fee calculation | < 1ms | Pure function, no I/O |
| Event delivery to billing | ~1-5s | Outbox poll interval |

---

## ✅ Success Criteria

- [ ] User cancellation blocked on invalid states (ACTIVE, COMPLETED)
- [ ] No-show worker expires exactly one HOLD_DURATION after hold_end
- [ ] No-show worker doesn't double-expire same reservation (SKIP LOCKED)
- [ ] Free cancellation if user cancels early (> 30min before hold expiry)
- [ ] Cancellation fee applied if within grace period (≤ 30min)
- [ ] No-show penalty applied correctly (booking fee + penalty)
- [ ] SMS sent for both user cancellation and no-show expiry

---

_Last updated: 2026-06-01 · Owner: Solution Architecture_
