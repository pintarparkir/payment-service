# High-Level Design — payment-service

Wraps the Midtrans QRIS sandbox. Receives webhook callbacks; emits
`payment.paid.v1` events for downstream consumers (reservation, notification).

## Position

```
       Mini App ──HTTPS POST──▶ /v1/payments/qris/intent
                                    │
                                    ▼
                            payment-service
                            ┌────────────┐
                            │ POST       │── HTTPS ──▶ Midtrans Sandbox
                            │ webhook    │◀── HTTPS ── Midtrans (HMAC sig)
                            └─────┬──────┘
                                  │ outbox publisher
                                  ▼
                              RabbitMQ ──▶ reservation, notification
```

## Responsibilities

- Translate `invoice_id + amount_idr` into a Midtrans QRIS transaction.
- Persist `payment` row with `pg_reference` (Midtrans `transaction_id`).
- Receive Midtrans webhook callbacks on settlement / failure.
- HMAC-verify webhook payloads (`MIDTRANS_WEBHOOK_SECRET`).
- Emit `payment.paid.v1` (or `payment.failed.v1`) on terminal status.

## Why a thin wrapper

We don't reimplement payment state machines. Midtrans is the source of truth.
Our `payment` table is a *projection* of Midtrans state, kept in sync via webhook.

If the webhook is missed (network blip), a daily reconciliation cron pulls the
status from Midtrans's `/v2/{order_id}/status` endpoint.

## Idempotency

- `CreateQrisIntent` is idempotent on `(invoice_id)` — UNIQUE on `pg_reference`
  prevents duplicate Midtrans transactions for the same invoice.
- Webhook is idempotent on `pg_reference` — duplicate webhook deliveries
  (Midtrans retries) → no duplicate event publish.
