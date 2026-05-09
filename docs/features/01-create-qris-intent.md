# Feature 01 — CreateQrisIntent

**Status:** ✅ shipped
**Owner:** payment-service

## Scope

The mini app POSTs `invoice_id`. We call Midtrans's `/v2/charge` to create a
QRIS transaction and return the EMV-QRIS payload string for the mini app to
render as a QR code.

## API contract

```
POST /v1/payments/qris/intent
Headers: Authorization: Bearer <jwt>, Idempotency-Key: <uuid>
Body: { "invoice_id": "<uuid>" }
→ 201 {
    "payment_id": "...",
    "qris_payload": "00020101...",
    "pg_reference": "MIDTRANS-SANDBOX-...",
    "expires_at": "2026-05-09T15:30:00Z"
  }
```

## Algorithm

```
1. Verify JWT, extract driver_id
2. (gRPC) billing.GetInvoice(invoice_id) — fetch amount_idr, verify status=CLOSED
3. Idempotency check on (invoice_id):
   SELECT * FROM payment WHERE invoice_id=$1 AND status='PENDING'
   if found and not expired → return existing
4. POST to Midtrans /v2/charge with:
   { "payment_type": "qris",
     "transaction_details": { "order_id": invoice_id, "gross_amount": amount_idr },
     "custom_expiry": { "unit": "minute", "expiry_duration": 15 } }
5. INSERT payment (status=PENDING, pg_reference=<from response>)
6. Return QRIS payload + pg_reference
```

## Tasks

- [ ] HTTPS client to Midtrans sandbox (`pkg/midtrans`)
- [ ] Idempotency on `(invoice_id, status=PENDING)` — re-use existing pending intent
- [ ] Test with sandbox keys; assert returned payload starts with `00020101`

## Acceptance criteria

- Two POSTs with the same `invoice_id` return the same `pg_reference`.
- Different `invoice_id`s create distinct `payment` rows.
- Midtrans 5xx → 503 to client (don't double-charge on retry).
