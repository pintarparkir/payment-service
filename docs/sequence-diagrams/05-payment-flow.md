# Payment Flow (QRIS) - Sequence Diagram

> **Purpose:** Detailed flow for QRIS payment intent creation and Midtrans webhook processing  
> **Related Docs:** [`payment-service/README.md`](../../../payment-service/README.md) · [`docs/architecture/erd/04-payment-service.md`](../erd/04-payment-service.md)  
> **Author:** Solution Architecture · **Last Updated:** 2026-06-01

---

## 📖 Overview

Flow ini mencakup:
1. **QRIS Intent Creation** — Driver requests QR code, payment-service calls Midtrans
2. **Midtrans Webhook Processing** — Midtrans confirms payment, webhook verified and processed
3. **Outbox Publishing** — `payment.paid.v1` / `payment.failed.v1` → notification

### Participants

| Name | Type | Responsibility |
|------|------|----------------|
| 👤 Driver | Actor | Scans QRIS via banking app |
| 📱 Mini-App | Client | Requests QRIS intent, displays QR code |
| 💾 Payment Service | gRPC/REST | QRIS intent creation, webhook handler |
| 🌐 Midtrans API | External | QR code generation, payment gateway |
| 💾 Postgres DB | Relational | Payment state persistence |
| 🔧 RabbitMQ | Message Queue | Async event publishing |
| 💾 Notification Service | Consumer | SMS dispatch |

---

## 🔢 Sequence Diagram - QRIS Intent Creation

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

## 🔢 Sequence Diagram - Midtrans Webhook Processing

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

---

## 💳 Payment State Machine

```
┌──────────────┐
│   PENDING    │ ← Create QRIS Intent
└──────┬───────┘
       │
       ├─── Midtrans: capture/settlement ──▶ ┌──────────────┐
       │                                      │    PAID      │ (terminal)
       │                                      └──────────────┘
       │
       ├─── Midtrans: deny/cancel/expire ──▶ ┌──────────────┐
       │                                      │   FAILED     │ (terminal)
       │                                      └──────────────┘
       │
       └─── Timeout (24h no webhook) ──────▶ ┌──────────────┐
                                              │   EXPIRED    │ (terminal)
                                              └──────────────┘
```

---

## 🔐 Webhook Security Implementation

```go
func VerifySignature(serverKey string, body []byte, signature string) bool {
    h := hmac.New(sha512.New, []byte(serverKey))
    h.Write(body)
    expected := hex.EncodeToString(h.Sum(nil))
    return subtle.ConstantTimeCompare([]byte(expected), []byte(signature)) == 1
}

// Handler
func (wh *WebhookHandler) HandleWebhook(c *gin.Context) {
    body, err := io.ReadAll(c.Request.Body)
    if err != nil {
        c.AbortWithStatus(500)
        return
    }
    
    signature := c.GetHeader("X-Signature")
    if !VerifySignature(wh.config.ServerKey, body, signature) {
        wh.log.Warn("webhook signature mismatch", ...)
        c.JSON(401, gin.H{"error": "invalid signature"})
        return
    }
    
    // Process webhook...
}
```

## 🐛 Error Scenarios & Handling

| Scenario | Detection | Recovery |
|----------|-----------|----------|
| **Webhook signature mismatch** | HMAC verification fails | 401 + security alert, manual reconciliation |
| **Webhook replay (duplicate pg_reference)** | Payment already PAID | Return 200 idempotent response |
| **Midtrans /charge timeout** | HTTP timeout > 3s | Retry with same idempotency key (max 3) |
| **Circuit breaker open** | Midtrans errors > threshold | Fast-fail 503, retry after half-open |
| **Invoice not found or not CLOSED** | gRPC to billing fails | 400 Bad Request |
| **Amount mismatch** | invoice.total vs payment.amount_idr | 400 Bad Request |
| **RabbitMQ unavailable** | Outbox publish error | Event stays unpublished; alert at 10k |

---

## 📊 Performance Characteristics

| Operation | Latency | Notes |
|-----------|---------|-------|
| QRIS intent creation (stub) | < 100ms | No external API call |
| QRIS intent creation (Midtrans) | < 500ms | Includes Midtrans API call time |
| Webhook processing | < 100ms | Signature verification + DB update |
| Outbox publish lag | < 5s | Poll interval-based |

---

## ✅ Success Criteria

- [ ] QRIS intent created only once per invoice (idempotent)
- [ ] Midtrans webhook signature verified before business logic
- [ ] Webhook replay handled idempotently (same pg_reference, same response)
- [ ] Payment terminal states (PAID, FAILED) are immutable
- [ ] Circuit breaker protects Midtrans from cascading failures
- [ ] Outbox events published for both PAID and FAILED states
- [ ] SMS receipt sent for successful payments

---

_Last updated: 2026-06-01 · Owner: Solution Architecture_
