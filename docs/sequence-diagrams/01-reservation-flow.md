# Reservation Flow - Sequence Diagram

> **Purpose:** Detailed flow for reservation creation with idempotency and double-book prevention  
> **Related Docs:** [`reservation-service/README.md`](../../../reservation-service/README.md) · [`docs/architecture/erd/02-reservation-service.md`](../erd/02-reservation-service.md)  
> **Author:** Solution Architecture · **Last Updated:** 2026-06-01

---

## 📖 Overview

Flow ini mencakup:
1. **Browse availability** — Driver melihat spot kosong per lantai
2. **Create reservation** — User memilih spot atau request system assignment
3. **Outbox publishing** — Async event ke billing-service untuk invoice creation
4. **Double-book prevention** — Redis lock + PostgreSQL EXCLUDE constraint

### Participants

| Name | Type | Responsibility |
|------|------|----------------|
| 👤 Driver | Actor | Initiates reservation |
| 📱 Mini-App | Client | Collects input, displays results |
| 🔌 Load Balancer | Infra | Entry point, JWT validation |
| 💾 Reservation Service | gRPC | Core business logic |
| 🔧 Redis Cache | In-memory | Distributed locking, availability cache |
| 💾 Postgres DB | Relational | Spot inventory, reservation data |
| 🔧 RabbitMQ | Message Queue | Async event publishing |
| 💾 Billing Service | gRPC (async) | Invoice creation via event consumer |

---

## 🔢 Sequence Diagram

```mermaid
sequenceDiagram
    autonumber
    actor 👤 as 👤 Driver
    participant 📱 as 📱 Mini-App
    participant 🔌 as 🔌 Load Balancer
    participant 💾 as 💾 Reservation Service
    participant 🔧 as 🔧 Redis Lock
    participant 💾_DB as 💾 Postgres DB
    participant 🔧_RMQ as 🔧 RabbitMQ
    participant 💾_Billing as 💾 Billing Service
    
    Note over 👤,💾_Billing: ──────────────────────────────────────────────
    PHASE 1: Browse Availability
    Note over 👤,💾_Billing: ──────────────────────────────────────────────
    
    👤->>📱: Tap "Find Parking"
    📱->>🔌: GET /v1/availability?type=CAR&floor=F2
    🔌->>💾: Forward request (JWT validated)
    activate 💾
    
    %% Check availability cache first
    💾->>🔧_Cache: GET availability:CAR:F2
    alt Cache hit
        🔧_Cache-->>💾: [F2-C-001..F2-C-030]
        💾-->>📱: 200 OK {available_spots}
    else Cache miss
        💾->>💾_DB: BEGIN
        💾->>💾_DB: SELECT * FROM spot<br/>WHERE vehicle_type='CAR'<br/>AND floor='F2' AND status='AVAILABLE'
        activate 💾_DB
        💾_DB-->>💾: 30 rows
        deactivate 💾_DB
        
        💾->>🔧_Cache: SET availability:CAR:F2 TTL=5min
        
        💾->>💾_DB: COMMIT
        💾-->>📱: 200 OK {available_spots}
    end
    deactivate 💾
    
    Note over 👤,💾_Billing: ──────────────────────────────────────────────
    PHASE 2: Create Reservation (User Selects Spot)
    Note over 👤,💾_Billing: ──────────────────────────────────────────────
    
    👤->>📱: Tap "F2-C-014" → "Reserve Now"
    📱->>🔌: POST /v1/reservations<br/>{spot_id: "F2-C-014", vehicle_type: "CAR"}
    Note right of 📱: Headers:<br/>Authorization: Bearer JWT<br/>Idempotency-Key: uuid-12345
    activate 🔌
    🔌->>💾: Forward (JWT decoded)
    activate 💾
    
    %% Step 1: Idempotency check
    💾->>💾_DB: BEGIN
    💾->>💾_DB: SELECT * FROM idempotency_key WHERE key = 'uuid-12345'
    alt Key exists (replay attempt)
        💾_DB-->>💾: Found cached response
        💾-->>📱: 200 OK (cached)
        deactivate 💾
        return
    end
    
    %% Step 2: Redis distributed lock (optimistic locking)
    💾->>🔧: SETNX lock:spot:F2-C-014 30
    alt Lock acquired (first request wins)
        🔧-->>💾: true
        
        %% Step 3: Transactional DB write
        💾->>💾_DB: BEGIN TRANSACTION
        
        %% Double-book prevention at DB level
        %% This will fail if spot is already reserved during hold_window
        💾->>💾_DB: INSERT INTO reservation (<br/>id, driver_id, spot_id,<br/>state='PENDING',<br/>hold_window=[NOW(), NOW()+60min])
        Note right of 💾_DB: EXCLUDE CONSTRAINT:<br/>USING gist (spot_id WITH =,<br/>hold_window WITH &&)<br/>WHERE state IN (PENDING,CONFIRMED,ACTIVE)
        
        alt Insert successful
            💾_DB-->>💾: reservation_id = uuid-v4
            
            %% Step 4: Outbox pattern (same tx)
            💾->>💾_DB: INSERT INTO outbox_event (<br/>id, topic, payload, created_at)<br/>VALUES (..., 'reservation.created.v1', {...})
            
            %% Record idempotency key
            💾->>💾_DB: INSERT INTO idempotency_key (<br/>key, response, expires_at)<br/>VALUES ('uuid-12345', {...}, NOW()+24h)
            
            %% Commit everything atomically
            💾->>💾_DB: COMMIT
            deactivate 💾_DB
            
            %% Cleanup lock
            💾->>🔧: DEL lock:spot:F2-C-014
            💾-->>📱: 201 Created {<br/>reservation_id, spot_id,<br/>hold_until, driver_id<br/>}
            deactivate 💾
            
            %% Step 5: Background outbox publisher (async)
            par Outbox Publisher Worker (polls every 2s)
                💾->>💾_DB: SELECT * FROM outbox_event<br/>WHERE published_at IS NULL LIMIT 100
                💾_DB-->>💾: Rows waiting publish
                
                💾->>🔧_RMQ: PUBLISH to parkirpintar.events<br/>routing_key: 'reservation.created.v1'<br/>Body: {reservation_id, driver_id, spot_id, ...}
                
                💾->>💾_DB: UPDATE outbox_event<br/>SET published_at = NOW()<br/>WHERE id = ?
            end
            
            %% Event reaches billing service
            🔧_RMQ-->💾_Billing: CONSUME reservation.created.v1
            activate 💾_Billing
            
            %% Billing service opens invoice
            💾_Billing->>💾_DB: BEGIN
            💾_Billing->>💾_DB: SELECT * FROM invoice WHERE reservation_id = ?
            alt Invoice not exists (open new one)
                💾_Billing->>💾_DB: INSERT INTO invoice (<br/>reservation_id, driver_id, status='OPEN')
                💾_Billing->>💾_DB: INSERT INTO invoice_line (<br/>invoice_id, kind='BOOKING', amount=2000)
            else Invoice exists (replay protection)
                💾_Billing->>💾_DB: SKIP (idempotent)
            end
            💾_Billing->>💾_DB: COMMIT
            deactivate 💾_Billing
            
            %% Optional: Send confirmation SMS
            opt Send Confirmation SMS
                💾_Billing->>🔧_RMQ: PUBLISH billing.invoice.opened.v1
                🔧_RMQ-->Notification: CONSUME → Render template → Send SMS
            end
            
        else Constraint violation (double-book!)
            💾_DB-->>💾: ERROR: conflicting key or interval
            💾->>💾_DB: ROLLBACK
            💾->>🔧: DEL lock:spot:F2-C-014
            💾-->>📱: 409 Conflict "Spot already reserved"<br/>Try alternative spot
            deactivate 💾
        end
        
    else Lock held (concurrent request still processing)
        🔧-->>💾: false
        💾->>💾_DB: ROLLBACK
        💾-->>📱: 409 Conflict "Try again (lock contention)"
        deactivate 💾
    end
    deactivate 🔌
    
    Note over 👤,💾_Billing: ──────────────────────────────────────────────
    Summary: ✅ Reservation created, ⚠️ Billing pending async
    Note over 👤,💾_Billing: Total time: ~150-250ms (sync path)
    Note over 👤,💾_Billing: Event delivery: ~1-5s (outbox poll interval)
    Note over 👤,💾_Billing: ──────────────────────────────────────────────
```

---

## 🔑 Key Technical Details

### 1. Idempotency Strategy

| Layer | Mechanism | TTL |
|-------|-----------|-----|
| **HTTP** | `Idempotency-Key` header | 24 hours |
| **gRPC** | Metadata `idempotency-key` | 24 hours |
| **Repository** | Partial unique index `WHERE idempotency_key IS NOT NULL` | Forever (until row deleted) |

**Implementation:**
```go
// repository.go
const idxIdempotencyKey = `
CREATE INDEX CONCURRENTLY idx_reservation_idem 
ON reservation(idempotency_key) 
WHERE idempotency_key IS NOT NULL;
`

// handler.go
func (h *Handler) CreateReservation(...) {
    idempotencyKey := r.Header.Get("Idempotency-Key")
    if existing := h.repo.GetByIDempotencyKey(ctx, idempotencyKey); existing != nil {
        return h.writeCachedResponse(existing) // Fast-path replay
    }
    // Proceed with create...
}
```

### 2. Double-Book Prevention

Two-layer defense:

1. **Redis optimistic lock** (fast failure)
   ```bash
   SETNX lock:spot:F2-C-014 30  # 30 second TTL
   ```
   
2. **PostgreSQL EXCLUDE constraint** (strong consistency)
   ```sql
   ALTER TABLE reservation ADD CONSTRAINT no_overlapping_reservations
   EXCLUDE USING gist (
       spot_id WITH =,
       hold_window WITH &&
   ) WHERE state IN ('PENDING', 'CONFIRMED', 'ACTIVE');
   ```

### 3. Outbox Pattern

Atomicity via single transaction:

```go
func (r *Repository) CreateWithOutbox(ctx context.Context, ...) (*Reservation, error) {
    return r.tx.ExecFunc(ctx, func(tx pg.Tx) (*Reservation, error) {
        // 1. Create reservation row
        res, err := insertReservation(tx, params)
        if err != nil {
            return nil, err
        }
        
        // 2. Create outbox event row (same transaction!)
        _, err = tx.ExecContext(ctx,
            `INSERT INTO outbox_event(topic, payload) VALUES ($1, $2)`,
            "reservation.created.v1", payloadJSON,
        )
        
        return res, err
    })
}
```

Outbox worker polls every 2 seconds:
```go
func (w *PublisherWorker) run(ctx context.Context) {
    ticker := time.NewTicker(2 * time.Second)
    for {
        select {
        case <-ticker.C:
            events, err := w.db.Query(`SELECT * FROM outbox_event WHERE published_at IS NULL LIMIT 100`)
            // Publish to RabbitMQ, then update published_at
        case <-ctx.Done():
            return
        }
    }
}
```

---

## 🐛 Error Scenarios & Handling

| Scenario | Detection | Recovery |
|----------|-----------|----------|
| **Double-book attempt** | PostgreSQL EXCLUDE constraint error | Return 409, show alternative spots to user |
| **Redis connection loss** | SETNX timeout | Retry once; if fails, proceed with DB-only check (fallback) |
| **RabbitMQ unavailable** | Publish error in outbox worker | Event stays in `published_at IS NULL`; alert on >1000 unpublished rows |
| **Idempotency key collision** | Unique constraint violation | Return cached response from previous successful request |
| **Transaction deadlock** | PostgreSQL deadlock detection error | Retry with exponential backoff (max 3 attempts) |
| **Invalid JWT token** | Middleware validation | Return 401 Unauthorized |
| **Missing required fields** | Request validation | Return 400 Bad Request with field errors |

---

## 📊 Performance Considerations

| Operation | p99 Latency | Notes |
|-----------|-------------|-------|
| Browse availability | < 80ms | Cached in Redis (5min TTL) |
| Create reservation | < 200ms | Includes DB commit, excludes async event |
| Outbox publish lag | < 5s | Determined by poll interval (2s) |
| Concurrent limit | ~1000 req/s | Depends on DB connection pool size |

### Optimization Tips

1. **Availability caching**: Use Redis hash structure keyed by `{vehicle_type}:{floor}`
2. **Connection pooling**: Set `MAX_OPEN_CONNS=25`, `MAX_IDLE_CONNS=10` for PostgreSQL
3. **Read replicas**: Route availability reads to read replica, writes to primary
4. **Spot locking granularity**: Consider using per-floor lock instead of per-spot if contention high

---

## 🔄 Related Flows

This flow triggers subsequent flows:

| Trigger | Subsequent Flow | Document |
|---------|-----------------|----------|
| `reservation.created.v1` | Open invoice in billing-service | [`billing-service/README.md`](../../../billing-service/README.md) |
| `reservation.confirmed.v1` | Confirm spot + send SMS | [`notification-service/README.md`](../../../notification-service/README.md) |
| Driver arrives | Check-in flow | [`02-checkin-flow.md`](./02-checkin-flow.md) |
| Driver leaves | Check-out + Close invoice | [`03-billing-checkout-flow.md`](./03-billing-checkout-flow.md) |

---

## ✅ Success Criteria

- [x] Reservation created without double-book even under concurrent access
- [x] Idempotency enforced across all retry attempts
- [x] Outbox event eventually delivered to billing-service (< 5s)
- [x] Invoice created for every valid reservation
- [x] No data loss on service restarts
- [x] Metrics exposed: `reservations_created_total`, `duplicates_prevented_total`

---

_Last updated: 2026-06-01 · Owner: Solution Architecture_
