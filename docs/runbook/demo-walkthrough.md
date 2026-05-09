# Demo Walkthrough — payment-service

End-to-end runbook. Includes the offline pricing of pricing tests `go test`
won't catch — the webhook signature suite plus a live QRIS-intent + webhook flow.

## Setup (~30 s)

```bash
cd ../infra && podman compose up -d         # postgres, redis, otel
cd ../payment-service
cp configs/.env.example configs/.env
make migrate-up                              # creates payment + outbox tables
make run                                     # REST :8083
```

`MIDTRANS_STUB_MODE=true` is the default; the service never calls the real
Midtrans API. Flip to false (with valid sandbox credentials) to hit the real
endpoint — same flow.

Health check:
```bash
curl -s http://localhost:8083/healthz   # → {"status":"ok"}
```

## Scenario 1 — Webhook signature suite (offline)

```bash
go test -v ./pkg/midtrans/...
```

You'll see:
- `TestParseAndVerify_HappySettlement`        — settlement+accept verifies
- `TestParseAndVerify_TamperedSignatureRejected` — flipped sig byte → ErrSignatureInvalid
- `TestParseAndVerify_WrongServerKeyRejected` — different key → ErrSignatureInvalid
- `TestParseAndVerify_EmptyKeySkipsCheck`     — dev-mode bypass works
- `TestNotification_TerminalClassification`   — settlement/capture → paid; deny/expire/cancel/failure → failed; pending → no-op; fraud-deny doesn't slip through

Engine is constant-time-compared (`crypto/subtle.ConstantTimeCompare`); bad
signatures don't leak timing information.

## Scenario 2 — Happy path with stub Midtrans (~30 s)

```bash
# Build a dev JWT (signature check is skipped when SUPER_APP_JWT_PUBLIC_KEY_PEM is empty)
PAYLOAD=$(printf '{"sub":"demo","phone":"+62811","exp":9999999999}' | base64 | tr -d '=' | tr '/+' '_-')
TOKEN="eyJhbGciOiJSUzI1NiJ9.${PAYLOAD}.x"
INVOICE=$(uuidgen | tr 'A-Z' 'a-z')

# 1. Create QRIS intent
INTENT=$(curl -s -X POST http://localhost:8083/v1/payments/qris/intent \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d "{\"invoice_id\":\"$INVOICE\",\"amount_idr\":10000}")
echo "$INTENT" | jq .
PAYMENT_ID=$(echo "$INTENT" | jq -r .payment_id)
PG_REF=$(echo "$INTENT" | jq -r .pg_reference)
# → payment_id, qris_payload starts with "00020101...", pg_reference="STUB-<invoice>"

# 2. Inspect (PENDING)
curl -s -H "Authorization: Bearer $TOKEN" \
  "http://localhost:8083/v1/payments/$PAYMENT_ID" | jq .status   # → "PENDING"

# 3. Simulate Midtrans webhook (settlement, signature_key empty for dev)
curl -s -X POST http://localhost:8083/v1/payments/webhook/midtrans \
  -H "Content-Type: application/json" \
  -d "{\"order_id\":\"$INVOICE\",\"status_code\":\"200\",\"gross_amount\":\"10000.00\",\"transaction_id\":\"$PG_REF\",\"transaction_status\":\"settlement\",\"fraud_status\":\"accept\",\"signature_key\":\"\"}"
# → {"status":"ok"}

# 4. Status flipped to PAID
curl -s -H "Authorization: Bearer $TOKEN" \
  "http://localhost:8083/v1/payments/$PAYMENT_ID" | jq .status   # → "PAID"
```

## Scenario 3 — Webhook idempotency

```bash
# Replay the same webhook → only one payment.paid.v1 in outbox
for i in 1 2 3; do
  curl -s -X POST http://localhost:8083/v1/payments/webhook/midtrans \
    -H "Content-Type: application/json" \
    -d "{\"order_id\":\"$INVOICE\",\"status_code\":\"200\",\"gross_amount\":\"10000.00\",\"transaction_id\":\"$PG_REF\",\"transaction_status\":\"settlement\",\"fraud_status\":\"accept\",\"signature_key\":\"\"}" > /dev/null
done

psql "postgres://postgres:postgres@localhost:5432/payment_service?sslmode=disable" \
  -c "SELECT event_type, COUNT(*) FROM outbox_event GROUP BY event_type ORDER BY event_type;"
# →  payment.intent.created.v1 |  1
#    payment.paid.v1           |  1
```

The guard is the `status != PENDING` short-circuit in `repository/postgres.MarkSettled` —
once the row is terminal, further calls are no-ops with no extra event.

## Scenario 4 — Tampered webhook is rejected

When `MIDTRANS_WEBHOOK_SECRET` is set (production-like), bad signatures are
rejected with HTTP 401 BEFORE any DB work happens:

```bash
# In configs/.env: MIDTRANS_WEBHOOK_SECRET=test-server-key
# Restart the service.

curl -s -X POST http://localhost:8083/v1/payments/webhook/midtrans \
  -H "Content-Type: application/json" \
  -d '{"order_id":"x","status_code":"200","gross_amount":"10000.00","transaction_id":"y","signature_key":"deadbeef"}' \
  -w "\nHTTP:%{http_code}\n"
# → {"error":"SIGNATURE_INVALID"}
# → HTTP:401
```

## Scenario 5 — Real Midtrans sandbox (optional)

If you have sandbox credentials:
```bash
# In configs/.env:
MIDTRANS_STUB_MODE=false
MIDTRANS_SERVER_KEY=SB-Mid-server-<your-key>
MIDTRANS_WEBHOOK_SECRET=<copy from Midtrans dashboard>
```
Restart and call `/v1/payments/qris/intent` with a real invoice — Midtrans returns
a scannable QRIS payload. Their webhook will arrive at
`/v1/payments/webhook/midtrans` after the customer scans+pays in the simulator.

## Cleanup

```bash
# Ctrl-C the make run, then:
cd ../infra && podman compose down
```
