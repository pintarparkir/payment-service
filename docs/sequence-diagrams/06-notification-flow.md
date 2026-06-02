# Notification Flow - Sequence Diagram

> **Purpose:** Detailed flow for event consumption, MSISDN resolution, and SMS dispatch  
> **Related Docs:** [`notification-service/README.md`](../../../notification-service/README.md)  
> **Author:** Solution Architecture · **Last Updated:** 2026-06-01

---

## 📖 Overview

Flow ini mencakup:
1. **Event consumption** — RabbitMQ delivers domain events to notification-service
2. **MSISDN resolution** — Resolve driver's phone number via cache or user-service gRPC
3. **Template rendering** — Format SMS based on event type
4. **SMS dispatch** — Send via Telkomsel gateway (stub in dev)
5. **Error handling** — Distinguish transient vs permanent failures for DLQ/NACK

### Participants

| Name | Type | Responsibility |
|------|------|----------------|
| 🔧 RMQ | RabbitMQ | Message broker for domain events |
| 💾 Notification Service | Consumer | Event → SMS processing |
| 🧠 Cache | In-memory | MSISDN cache (TTL 5min) |
| 💾 User Service | gRPC | MSISDN resolution fallback |
| 📱 SMS Gateway | External | Telkomsel SMS API (or stub) |
| 🗑️ DLQ | RabbitMQ | Dead letter queue for permanent failures |
| 👤 Driver | Actor | Receives SMS notification |

---

## 🔢 Sequence Diagram - Successful Flow

```mermaid
sequenceDiagram
    autonumber
    participant 🔧 as 🔧 RabbitMQ
    participant 💾 as 💾 Notification Service
    participant 🧠 as 🧠 MSISDN Cache
    participant 💾_User as 💾 User Service
    participant 📱 as 📱 SMS Gateway
    participant 👤 as 👤 Driver
    
    Note over 🔧,👤: ──────────────────────────────────────────────
    FLOW: reservation.confirmed.v1 → SMS Dispatch
    Note over 🔧,👤: ──────────────────────────────────────────────
    
    %% Event arrives from outbox publisher
    🔧->>💾: DELIVER reservation.confirmed.v1<br/>{reservation_id, driver_id, spot_id, hold_end}
    activate 💾
    
    %% Step 1: Parse event payload
    💾->>💾: Parse JSON body<br/>alt Parse error (invalid JSON)
        💾->>🔧: ACK + Move to DLQ
        Note right of 💾: Permanent error, no retry
        deactivate 💾
        return
    end
    
    %% Step 2: Extract driver_id
    alt Missing required field (driver_id)
        💾->>🔧: ACK + Move to DLQ
        Note right of 💾: Data integrity issue
        deactivate 💾
        return
    end
    
    %% Step 3: MSISDN resolution
    💾->>🧠: GET cache:user:{driver_id}
    
    alt Cache hit (phone found)
        🧠-->>💾: phone_e164
        Note right of 💾: Cache TTL = 5min
    else Cache miss
        %% Step 3b: Fallback to user-service
        💾->>💾_User: gRPC GetUserById(driver_id)
        activate 💾_User
        
        alt User found
            💾_User-->>💾: phone_e164 (decrypted via pgcrypto)
            💾->>🧠: SET cache:user:{driver_id} = phone_e164 TTL=5min
        else User NOT_FOUND
            💾_User-->>💾: error=NOT_FOUND
            💾->>🔧: ACK + Move to DLQ
            Note right of 💾: Permanent error (missing driver)
            deactivate 💾_User
            deactivate 💾
            return
        end
        
        deactivate 💾_User
    end
    
    %% Step 4: Render SMS template
    💾->>💾: Render template for event_type='reservation.confirmed.v1'
    Note right of 💾: Template: "Reservasi spot {spot_id} dikonfirmasi. Silakan check-in sebelum {hold_end}."
    💾->>💾: message = "Reservasi spot F2-C-014 dikonfirmasi. Silakan check-in sebelum 09:10."
    
    %% Step 5: Send SMS via gateway
    💾->>📱: POST /send_sms {to: "+628123456789", message: "...", sender_id: "ParkirPintar"}
    activate 📱
    
    alt SMS 2xx Success
        📱-->>💾: 200 OK {message_id: "msg-123"}
        💾->>🔧: ACK
        Note right of 💾: Message processed successfully
        deactivate 📱
    else SMS 4xx Client Error
        %% Invalid phone, blocked number, etc.
        📱-->>💾: 400 Bad Request {error: "invalid destination"}
        💾->>🔧: ACK + Move to DLQ
        Note right of 💾: Permanent error (won't recover on retry)
        deactivate 📱
    else SMS 5xx Server Error
        %% Telkomsel gateway down, rate limit, etc.
        📱-->>💾: 503 Service Unavailable
        💾->>🔧: NACK + REQUEUE
        Note right of 💾: Transient error, will retry
        deactivate 📱
    end
    
    deactivate 💾
    
    Note over 🔧,👤: ──────────────────────────────────────────────
    Summary: ✅ SMS sent successfully
    Note over 🔧,👤: Total latency: < 30s (SLO target)
    Note over 🔧,👤: Driver receives: "Reservasi spot F2-C-014 dikonfirmasi..."
    Note over 🔧,👤: ──────────────────────────────────────────────
```

## 🔢 Sequence Diagram - Error Handling & DLQ

```mermaid
sequenceDiagram
    autonumber
    participant 🔧 as 🔧 RabbitMQ
    participant 💾 as 💾 Notification Service
    participant 🧠 as 🧠 MSISDN Cache
    participant 💾_User as 💾 User Service
    participant 🗑️ as 🗑️ DLQ (notification.events.dlq)
    participant 👤 as 👤 Driver
    
    Note over 🔧,👤: ──────────────────────────────────────────────
    SCENARIO: Permanent Errors → DLQ
    Note over 🔧,👤: ──────────────────────────────────────────────
    
    🔧->>💾: DELIVER reservation.confirmed.v1<br/>{driver_id: "invalid"}
    activate 💾
    
    %% Missing driver_id
    💾->>💾: Parse event
    alt Missing driver_id
        💾->>🔧: ACK + Move to DLQ
        Note right of 💾: Schema violation
        deactivate 💾
        return
    end
    
    %% Try cache
    💾->>🧠: GET cache:user:invalid
    🧠-->>💾: miss
    
    %% Fallback to user-service
    💾->>💾_User: gRPC GetUserById("invalid")
    activate 💾_User
    
    💾_User-->>💾: NOT_FOUND
    deactivate 💾_User
    
    %% Permanent error → DLQ
    💾->>🔧: ACK + Move to DLQ
    Note right of 💾: Will not recover with retry
    deactivate 💾
    
    %% DLQ inspection
    Note over 🗑️,👤: DLQ now contains this message for manual review
    participant 💻 as 💻 Operator
    
    💻->>🗑️: LIST DLQ messages
    🗑️-->>💻: [{event: reservation.confirmed.v1, reason: missing driver_id}]
    
    %% After investigation, operator replays
    alt Root cause fixed (driver created)
        💻->>🔧: REPLAY DLQ message to main exchange
        Note right of 💻: After driver onboarding
        🔧-->💾: DELIVER reservation.confirmed.v1 (replayed)
        %% Flow proceeds normally from here
    else Root cause not fixed
        💻->>🗑️: IGNORE (remain in DLQ for audit)
    end
```

## 🔢 Sequence Diagram - Transient Error → Retry

```mermaid
sequenceDiagram
    autonumber
    participant 🔧 as 🔧 RabbitMQ
    participant 💾 as 💾 Notification Service
    participant 💾_User as 💾 User Service
    participant 📱 as 📱 SMS Gateway
    participant 👤 as 👤 Driver
    
    Note over 🔧,👤: ──────────────────────────────────────────────
    SCENARIO: Transient Error → Retry → Success
    Note over 🔧,👤: ──────────────────────────────────────────────
    
    loop Retry Loop (max 3 attempts)
        🔧->>💾: DELIVER reservation.confirmed.v1
        activate 💾
        
        %% Processing...
        💾->>🧠: Cache miss
        💾->>💾_User: gRPC GetUserById(driver_id)
        activate 💾_User
        
        %% Simulate timeout on 1st attempt
        alt Attempt == 1
            💾_User-->>💾: context deadline exceeded (timeout after 2s)
            deactivate 💾_User
            
            %% Transient error → NACK
            💾->>🔧: NACK + REQUEUE
            Note right of 💾: Will retry after backoff
            deactivate 💾
            
        else Attempt >= 2
            %% Success on retry
            💾_User-->>💾: phone_e164
            deactivate 💾_User
            
            %% Render and send SMS
            💾->>📱: POST /send_sms
            activate 📱
            📱-->>💾: 200 OK
            💾->>🔧: ACK
            Note right of 💾: Success on retry
            deactivate 📱
            deactivate 💾
            return
        end
    end
    
    %% Max retries exceeded → DLQ
    💾->>🔧: ACK + Move to DLQ
    Note right of 💾: Exhausted retry attempts
    deactivate 💾
```

---

## 📱 SMS Template Library

| Event Type | Template (Indonesian) | Variables |
|------------|-----------------------|-----------|
| `reservation.confirmed.v1` | "Reservasi spot {spot_id} dikonfirmasi. Silakan check-in sebelum {hold_end}." | spot_id, hold_end |
| `reservation.cancelled.v1` | "Reservasi Anda telah dibatalkan. Jika ada pertanyaan, hubungi customer service." | reason (optional) |
| `reservation.expired.v1` | "Reservasi expired (no-show). Anda dikenai biaya sebesar Rp {amount}." | amount (booking + noshow) |
| `reservation.checked_out.v1` | "Sesi parkir Anda telah selesai. Total estimasi: Rp {amount}. Silakan bayar via mini-app." | amount (estimate) |
| `billing.invoice.closed.v1` | "Tagihan parkir IDR {amount}. Silakan bayar via menu Payment." | amount (total_idr) |
| `payment.paid.v1` | "Pembayaran berhasil Rp {amount}. Terima kasih telah menggunakan ParkirPintar." | amount |
| `payment.failed.v1` | "Pembayaran gagal: {reason}. Silakan coba lagi atau hubungi customer service." | reason |

**Formatting Rules:**
- Amounts formatted with thousand separator: `14500 → "14.500"`
- Time format: `15:30` (24-hour, WIB)
- Phone numbers: E.164 format without `+` in logs (masked)

---

## 🔐 Error Classification Logic

```go
func ClassifyError(eventType string, err error) ErrorClassification {
    switch {
    case errors.Is(err, json.UnmarshalTypeError{}):
        return Permanent // Schema violation
    
    case errors.Is(err, sql.ErrNoRows):
        if strings.Contains(err.Error(), "user_profile") {
            return Permanent // Missing driver record
        }
        return Transient // Temporary DB glitch
    
    case errors.Is(err, context.DeadlineExceeded),
         strings.Contains(err.Error(), "timeout"):
        return Transient // Will likely succeed on retry
    
    case strings.Contains(err.Error(), "connection refused"),
         strings.Contains(err.Error(), "network is unreachable"):
        return Transient // Infrastructure issue
    
    case strings.Contains(strings.ToUpper(err.Error()), "4XX"):
        return Permanent // Client error (bad request)
    
    case strings.Contains(strings.ToUpper(err.Error()), "5XX"):
        return Transient // Server error (retry)
    
    default:
        return Transient // Safe default: retry
    }
}

// Handler
func (h *Handler) HandleMessage(delivery amqp.Delivery) error {
    err := h.processEvent(delivery.Body)
    class := h.ClassifyError(delivery.RoutingKey, err)
    
    switch class {
    case Permanent:
        delivery.Ack(false) // Accept but move to DLQ via binding
        h.dlq.Increment(delivery.RoutingKey)
    case Transient:
        delivery.Nack(false, true) // Requeue
        h.metrics.TransientErrors.Inc()
    }
    return nil
}
```

---

## 📊 Performance & Reliability Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Event → SMS latency (p99) | < 30s | From RabbitMQ delivery to SMS gateway ACK |
| DLQ rate | < 1% per day | Messages moved to DLQ / total consumed |
| MSISDN cache hit rate | ≥ 80% | Cache hits / total resolution attempts |
| SMS success rate | ≥ 98% | Successful SMS sends / total attempts |
| Consumer idle time | > 90% | Time spent waiting for events |

---


