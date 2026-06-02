# End-to-End Flow — Complete User Journey

> **Purpose:** Comprehensive sequence diagram showing the full parking lifecycle across ALL services  
> **Scope:** Reserve → Confirm → Check-in → Check-out → Invoice → Payment → SMS Receipt  
> **Author:** Solution Architecture · **Last Updated:** 2026-06-01

---

## 📖 Overview

Dokumen ini menampilkan **complete user journey** dari awal sampai akhir, mencakup semua 5 microservices:

| Service | Role in Journey |
|---------|-----------------|
| **user-service** | Driver identity & MSISDN (identity provider) |
| **reservation-service** | Spot inventory & reservation lifecycle |
| **billing-service** | Invoice ledger & pricing engine |
| **payment-service** | QRIS payment intent & Midtrans integration |
| **notification-service** | Event consumer → SMS dispatch |

### Key Characteristics

| Aspect | Implementation |
|--------|----------------|
| **Sync communication** | gRPC (reservation ↔ billing), REST (mini-app) |
| **Async events** | RabbitMQ via outbox pattern |
| **Idempotency** | All write operations idempotent on keys |
| **State machine** | PENDING → CONFIRMED → ACTIVE → COMPLETED |
| **PII protection** | pgcrypto encryption for phone/email |
| **Double-book prevention** | PostgreSQL EXCLUDE constraint + Redis lock |

---

## 🔢 Complete Sequence Diagram

```mermaid
sequenceDiagram
    autonumber
    actor 👤 as 👤 Driver
    participant 📱 as 📱 Mini-App
    participant 🔌 as 🔌 Load Balancer
    participant 💾_User as 💾 User Service
    participant 🔧_Redis as 🔧 Redis Cache
    participant 💾_Res as 💾 Reservation Service
    participant 💾_Billing as 💾 Billing Service
    participant 💾_Pay as 💾 Payment Service
    participant 💾_Notif as 💾 Notification Service
    participant 🔧_RMQ as 🔧 RabbitMQ
    participant 🌐 as 🌐 Midtrans API
    participant 📱_SMS as 📱 Telkomsel SMS Gateway
    participant 💾_DB_Res as 💾 DB Reservation
    participant 💾_DB_Bill as 💾 DB Billing
    
    Note over 👤,💾_DB_Res: ──────────────────────────────────────────────
    PHASE 1: Browse & Create Reservation
    Note over 👤,💾_DB_Res: ──────────────────────────────────────────────
    
    👤->>📱: Tap "Find Parking"
    📱->>🔌: GET /v1/availability?type=CAR&floor=F2
    
    %% User Service (Lazy Registration)
    🔌->>💾_User: Validate JWT driver_id
    activate 💾_User
    💾_User->>🔧_Redis: GET profile:driver_id
    alt Cache miss
        💾_User->>💾_User: UpsertDriver(external_user_id, name, phone_enc)
        💾_User->>🔧_Redis: SET profile TTL=5min
    end
    deactivate 💾_User
    
    %% Availability Lookup
    🔌->>💾_Res: Forward availability request
    
    par Availability Response
        💾_Res->>💾_DB_Res: SELECT * FROM spot WHERE vehicle_type='CAR' AND status='AVAILABLE'
        💾_DB_Res-->>💾_Res: [F2-C-001..F2-C-030]
        
        💾_Res->>🔧_Redis: SET availability:CAR:F2 TTL=5min
        💾_Res-->>📱: 200 OK {available_spots}
    end
    
    👤->>📱: Select "F2-C-014" → Tap "Reserve Now"
    📱->>🔌: POST /v1/reservations {spot_id, vehicle_type}
    📱->>🔌: Header: Idempotency-Key: uuid-abc123
    
    activate 💾_Res
    
    %% Double-book Prevention
    💾_Res->>🔧_Redis: SETNX lock:spot:F2-C-014 30
    alt Lock acquired
        🔧_Redis-->>💾_Res: true
        
        %% Transactional Write
        💾_Res->>💾_DB_Res: BEGIN
        💾_Res->>💾_DB_Res: INSERT INTO reservation<br/>(state='PENDING', hold_window=[now, now+60min])
        
        %% Outbox Pattern
        💾_Res->>💾_DB_Res: INSERT outbox_event('reservation.created.v1', {...})
        💾_Res->>💾_DB_Res: COMMIT
        
        💾_Res-->>📱: 201 Created {reservation_id, spot_id, hold_until}
        deactivate 💾_Res
    else Lock held
        💾_Res-->>📱: 409 Conflict "Spot already reserved"
    end
    
    %% Outbox Publisher (background)
    activate 🔧_RMQ
    💾_Res->>🔧_RMQ: PUBLISH reservation.created.v1
    🔧_RMQ-->💾_Billing: CONSUME
    deactivate 🔧_RMQ
    
    activate 💾_Billing
    
    %% Open Invoice
    💾_Billing->>💾_DB_Bill: BEGIN
    💾_Billing->>💾_DB_Bill: INSERT invoice(reservation_id, status='OPEN')
    💾_Billing->>💾_DB_Bill: INSERT line_item(kind='BOOKING', amount=2000)
    💾_Billing->>💾_DB_Bill: COMMIT
    
    💾_Billing->>🔧_RMQ: PUBLISH billing.invoice.opened.v1
    💾_Billing->>💾_Notif: Deliver event
    deactivate 💾_Billing
    
    Note over 👤,💾_Notif: ──────────────────────────────────────────────
    PHASE 2: Confirm & Check-In
    Note over 👤,💾_Notif: ──────────────────────────────────────────────
    
    👤->>📱: Tap "Confirm Spot"
    📱->>💾_Res: POST /v1/reservations/{id}/confirm
    
    activate 💾_Res
    💾_Res->>💾_DB_Res: UPDATE state='CONFIRMED'
    💾_Res->>💾_DB_Res: INSERT outbox_event('reservation.confirmed.v1')
    💾_Res->>💾_DB_Res: COMMIT
    💾_Res-->>📱: 200 OK
    
    💾_Res->>🔧_RMQ: PUBLISH reservation.confirmed.v1
    
    par Async Processing
        🔧_RMQ-->💾_Notif: CONSUME
        
        %% MSISDN Resolution
        💾_Notif->>💾_User: GetUserById(driver_id)
        💾_User-->>💾_Notif: phone_e164
        
        %% Render and Send SMS
        💾_Notif->>💾_Notif: Template "Reservasi spot F2-C-014..."
        💾_Notif->>📱_SMS: POST /send_sms
        
        💾_Notif-->>📱: SMS sent successfully
    end
    
    %% Driver arrives at building
    👤->>📱: Tap "Check-In"<br/>GPS: lat=-6.20015, lon=106.81705
    
    activate 💾_Res
    💾_Res->>💾_Res: Haversine distance = 45m <= 150m threshold ✅
    💾_Res->>💾_DB_Res: UPDATE state='ACTIVE', checked_in_at=NOW()
    💾_Res-->>📱: 200 OK {state: 'ACTIVE'}
    deactivate 💾_Res
    
    Note over 👤,💾_Notif: ──────────────────────────────────────────────
    PHASE 3: Check-Out & Invoice Closing
    Note over 👤,💾_Notif: ──────────────────────────────────────────────
    
    👤->>📱: Tap "Check-Out"<br/>Parking duration: 2.5 hours
    📱->>💾_Res: POST /v1/reservations/{id}/check-out
    
    activate 💾_Res
    💾_Res->>💾_DB_Res: UPDATE state='COMPLETED', checked_out_at=NOW()
    💾_Res->>💾_DB_Res: INSERT outbox_event('reservation.checked_out.v1')
    💾_Res->>💾_DB_Res: COMMIT
    💾_Res-->>📱: 200 OK {ready for payment}
    deactivate 💾_Res
    
    %% Outbox Publish
    activate 🔧_RMQ
    💾_Res->>🔧_RMQ: PUBLISH reservation.checked_out.v1
    🔧_RMQ-->💾_Billing: CONSUME
    deactivate 🔧_RMQ
    
    %% Pricing Engine Calculation
    activate 💾_Billing
    💾_Billing->>💾_Billing: CalculatePricing({checkedIn:08:00, checkedOut:10:30, type:CAR})
    Note right of 💾_Billing: BOOKING: 2,000<br/>HOURLY (2.5h): 12,500<br/>TOTAL: 14,500
    
    💾_Billing->>💾_DB_Bill: UPDATE invoice SET total=14500, status='CLOSED'
    💾_Billing->>💾_DB_Bill: INSERT line_item(kind='HOURLY', amount=12500)
    💾_Billing->>💾_DB_Bill: INSERT outbox_event('billing.invoice.closed.v1')
    💾_Billing->>💾_DB_Bill: COMMIT
    deactivate 💾_Billing
    
    💾_Billing->>🔧_RMQ: PUBLISH billing.invoice.closed.v1
    🔧_RMQ-->💾_Notif: CONSUME
    activate 💾_Notif
    💾_Notif->>📱_SMS: POST /send_sms<br/>"Total IDR 14,500. Silakan bayar."
    deactivate 💾_Notif
    
    Note over 👤,💾_Notif: ──────────────────────────────────────────────
    PHASE 4: Payment Intent & QRIS Generation
    Note over 👤,💾_Notif: ──────────────────────────────────────────────
    
    👤->>📱: Tap "Pay Now"
    📱->>💾_Pay: POST /v1/payments/qris-intent {invoice_id: INV-001}
    
    activate 💾_Pay
    
    %% Verify invoice exists
    💾_Pay->>💾_Billing: GetInvoice(invoice_id)
    💾_Billing-->>💾_Pay: total_idr=14500, status=CLOSED
    
    %% Create payment record
    💾_Pay->>💾_DB_Res: BEGIN
    💾_Pay->>💾_DB_Res: INSERT payment(invoice_id, status='PENDING', amount=14500)
    💾_Pay->>💾_DB_Res: COMMIT
    
    %% Call Midtrans QRIS
    💾_Pay->>🌐: POST /charge {gross_amount: 14500, payment_type: qris}
    activate 🌐
    🌐-->>💾_Pay: {qr_code: base64_encoded}
    deactivate 🌐
    
    💾_Pay-->>📱: 200 OK {qris_image_url, expires_at}
    deactivate 💾_Pay
    
    📱->>📱: Display QR code image on screen
    
    Note over 👤,💾_Notif: ⏳ Driver scans QRIS via mobile banking app
    Note over 👤,💾_Notif: ──────────────────────────────────────────────
    
    Note over 👤,💾_Notif: ──────────────────────────────────────────────
    PHASE 5: Webhook Processing & Receipt
    Note over 👤,💾_Notif: ──────────────────────────────────────────────
    
    🌐->>💾_Pay: POST /v1/payments/webhook/midtrans<br/>{transaction_status: capture, signature_key: "..."}
    
    activate 💾_Pay
    
    %% Step 1: HMAC Signature Verification
    💾_Pay->>💾_Pay: ConstantTimeCompare(HMAC-SHA512(raw_body), signature)
    
    alt Valid signature
        %% Idempotency check
        💾_Pay->>💾_DB_Res: BEGIN
        💾_Pay->>💾_DB_Res: UPDATE payment SET status='PAID', paid_at=NOW()
        💾_Pay->>💾_DB_Res: INSERT outbox_event('payment.paid.v1', {...})
        💾_Pay->>💾_DB_Res: COMMIT
        
        💾_Pay->>🔧_RMQ: PUBLISH payment.paid.v1
        💾_Pay-->>🌐: 200 OK
        
        %% Async notification
        par Notification
            🔧_RMQ-->💾_Notif: CONSUME payment.paid.v1
            activate 💾_Notif
            
            💾_Notif->>💾_Notif: Template "Pembayaran berhasil Rp14,500. Terima kasih!"
            💾_Notif->>📱_SMS: POST /send_sms
            
            💾_Notif-->>📱: SMS receipt delivered!
            deactivate 💾_Notif
        end
        
    else Invalid signature
        💾_Pay-->>🌐: 401 Unauthorized
        Note left of 💾_Pay: Security alert logged
    end
    deactivate 💾_Pay
    
    Note over 👤,💾_Notif: ──────────────────────────────────────────────
    END: Complete Journey Successful ✅
    Note over 👤,💾_Notif: Total journey time: ~5-10 minutes
    Note over 👤,💾_Notif: Total cost: IDR 14,500
    Note over 👤,💾_Notif: SMS notifications: 3 (confirm, charge, receipt)
    Note over 👤,💾_Notif: ──────────────────────────────────────────────
```

---

## 🔄 State Machine Integration Across Services

```
┌─────────────────────────────────────────────────────────────────────────┐
│                           RESERVATION STATE MACHINE                     │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│   ┌──────────┐                                                            │
│   │ CREATED  │ ← Phase 1: Reservation created                          │
│   └────┬─────┘                                                            │
│        │                                                                 
│        ▼                                                                   │
│   ┌──────────┐                    ┌──────────┐                            │
│   │ CONFIRMED│◀─────────────────▶│ CANCELLED│                            │
│   └────┬─────┘                    └──────────┘                            │
│        │                                                                  │
│        │ confirm                                                          │
│        ▼                                                                   │
│   ┌──────────┐                                                             │
│   │  ACTIVE  │◀── check-in (geofence)                                      │
│   └────┬─────┘                                                             │
│        │                                                                   │
│        │ check-out                                                          │
│        ▼                                                                   │
│   ┌──────────┐                                                             │
│   │COMPLETED │ → Invoice closed, ready for payment                         │
│   └────┬─────┘                                                             │
│        │                                                                   │
│        │ no-show (>60min hold expiry)                                       │
│        ▼                                                                   │
│   ┌──────────┐                                                             │
│   │ EXPIRED  │ → No-show fee applied                                     │
│   └──────────┘                                                             │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## 📊 Service Communication Matrix

| Trigger | Source Service | Target Service | Protocol | Sync/Async | Event Type |
|---------|---------------|----------------|----------|------------|------------|
| Create reservation | reservation-service | billing-service | Outbox | Async | `reservation.created.v1` |
| Check-in | reservation-service | — | REST | Sync | — |
| Check-out | reservation-service | billing-service | Outbox | Async | `reservation.checked_out.v1` |
| Close invoice | billing-service | notification-service | Outbox | Async | `billing.invoice.closed.v1` |
| Payment intent | payment-service | midtrans | HTTPS | Sync | — |
| Payment status | midtrans | payment-service | Webhook | Async | `payment.paid.v1` |
| SMS dispatch | notification-service | — | SMS API | Sync | — |
| MSISDN lookup | notification-service | user-service | gRPC | Sync | — |

---

## 🕐 Timeline View

```
T+0s     👤 Opens mini-app → Browse availability (50ms)
T+2s     👤 Reserves spot → Create reservation (150ms)
T+3s     Outbox publishes event → Billing opens invoice (2s)
T+5s     Notification sends confirmation SMS (500ms)

T+10m    👤 Arrives → Check-in (geofence validation, 30ms)
         ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
T+20m    👤 Leaves → Check-out (100ms)
T+21m    Billing closes invoice + calculates fees (200ms)
T+22m    Notification sends charging SMS (500ms)

T+23m    👤 Taps Pay → Generate QRIS (400ms)
T+24m    👤 Scans QRIS → Waits for webhook (1-5 min typical)

T+25m    Midtrans webhook → Payment PAID (100ms)
T+26m    Notification sends receipt SMS (500ms)

T+26m    JOURNEY COMPLETE ✅
```

---


