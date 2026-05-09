# Feature 03 — Reconciliation cron

**Status:** 📋 planned
**Owner:** payment-service

## Scope

Daily cron pulls Midtrans status for any `payment` rows still PENDING beyond
their expiry. Catches missed webhooks (network blips, Midtrans retries
exhausted, etc).

## Schedule

`0 2 * * *` (Asia/Jakarta) — runs at 02:00 WIB, low-traffic window.

## Algorithm

```
SELECT id, pg_reference FROM payment
WHERE status = 'PENDING' AND created_at < now() - interval '30 minutes'

For each:
  GET https://api.sandbox.midtrans.com/v2/$pg_reference/status
  parse status, apply same logic as the webhook handler
  emit appropriate event
```

## Tasks

- [ ] `cmd/recon-cron` (cobra subcommand) or `pkg/scheduler` job
- [ ] Rate-limit the polls (Midtrans limits us to 5 rps on the sandbox)
- [ ] Metric: `payment_recon_resolved_total{outcome=paid|failed}`

## Acceptance criteria

- Killing the webhook listener for an hour, then running recon, recovers all
  state (eventually-consistent).
