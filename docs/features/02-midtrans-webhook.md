# Feature 02 — Midtrans webhook

**Status:** ✅ shipped
**Owner:** payment-service

## Scope

Midtrans POSTs to `/v1/payments/webhook/midtrans` whenever a transaction reaches
a terminal status. We verify the HMAC signature, update the local row, and emit
`payment.paid.v1` (or `payment.failed.v1`).

## Verification

Midtrans sends `signature_key` = SHA-512(`order_id + status_code + gross_amount + server_key`).
We recompute and compare in constant time. Mismatch → 401, no body changes.

## Algorithm

```
1. Parse JSON body → MidtransNotification
2. expected = sha512(order_id + status_code + gross_amount + MIDTRANS_WEBHOOK_SECRET)
3. if !hmac.Equal(received_signature_key, expected) → 401
4. Find payment by pg_reference (== Midtrans transaction_id)
5. if payment.status != PENDING → 200 (idempotent on retry)
6. switch transaction_status:
     "settlement", "capture" → status=PAID, paid_at=now(),
                               outbox: payment.paid.v1
     "deny", "expire", "cancel", "failure" → status=FAILED,
                               outbox: payment.failed.v1
7. Return 200
```

## Tasks

- [ ] HMAC-SHA512 verifier (constant-time compare)
- [ ] `usecase.HandleWebhook` orchestration
- [ ] Outbox row + publisher
- [ ] DLQ for unparseable payloads
- [ ] Test: replay same webhook 3× → 1 outbox row, 200 OK each time

## Acceptance criteria

- Tampered signature → 401 with `error: SIGNATURE_INVALID`.
- Replayed webhook for already-PAID payment → 200, no second event.
- Unknown `pg_reference` → 404 (Midtrans will retry; we log + alert).
