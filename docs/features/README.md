# Features — payment-service

| File                              | Status | Summary                                       |
|-----------------------------------|--------|-----------------------------------------------|
| `01-create-qris-intent.md`        | ✅     | Mini-app POST → Midtrans `/charge` → QRIS payload |
| `02-midtrans-webhook.md`          | ✅     | HMAC-SHA512 verified, terminal-status dispatch + idempotent |
| `03-reconciliation-cron.md`       | 📋     | Daily pull for missed webhooks (Beyond MVP)   |

Legend: 📋 planned · ⏳ in progress · ✅ shipped · 🚫 deferred
