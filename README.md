# payment-service

QRIS payment integration via Midtrans sandbox. Receives webhook callbacks,
emits `payment.paid.v1` events on settlement.

## At a glance

| Surface | Port  | Used by                                   |
|---------|-------|-------------------------------------------|
| REST    | 8083  | Mini app (CreateQrisIntent), Midtrans webhook |
| gRPC    | 9092  | s2s (`GetPayment`, future fan-out)         |

## REST API

| Method | Path                              | Description                       |
|--------|-----------------------------------|-----------------------------------|
| POST   | /v1/payments/qris/intent          | Create QRIS payload via Midtrans  |
| POST   | /v1/payments/webhook/midtrans     | Webhook callback (HMAC-verified)  |

## Service dependencies

| Dependency       | Protocol  | Purpose                                |
|------------------|-----------|----------------------------------------|
| Midtrans (HTTP)  | HTTPS     | QRIS sandbox / production gateway      |
| RabbitMQ         | AMQP      | Emit `payment.paid.v1` / `failed.v1`   |
| PostgreSQL       | TCP       | Payment storage                        |

## Run

```bash
cd ../infra && docker compose up -d
cd ../payment-service
cp configs/.env.example configs/.env
make migrate-up
make run
```
