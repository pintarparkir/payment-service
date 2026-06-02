# payment-service

[![Security](https://sonarcloud.io/api/project_badges/measure?project=pintarparkir_payment-service&metric=security_rating)](https://sonarcloud.io/summary/new_code?id=pintarparkir_payment-service)
[![Reliability](https://sonarcloud.io/api/project_badges/measure?project=pintarparkir_payment-service&metric=reliability_rating)](https://sonarcloud.io/summary/new_code?id=pintarparkir_payment-service)
[![Maintainability](https://sonarcloud.io/api/project_badges/measure?project=pintarparkir_payment-service&metric=sqale_rating)](https://sonarcloud.io/summary/new_code?id=pintarparkir_payment-service)
[![Duplications](https://sonarcloud.io/api/project_badges/measure?project=pintarparkir_payment-service&metric=duplicated_lines_density)](https://sonarcloud.io/summary/new_code?id=pintarparkir_payment-service)
[![Coverage](https://sonarcloud.io/api/project_badges/measure?project=pintarparkir_payment-service&metric=coverage)](https://sonarcloud.io/summary/new_code?id=pintarparkir_payment-service)

> **Purpose:** QRIS payment integration — owns payment intent creation, Midtrans webhook processing, and payment events.
> **Author:** Farid Triwicaksono

## Architecture Overview

![Architecture](docs/PintarParkir.architecture.svg)

## E2E Flow

![Flow Diagram](docs/flow.diagram.svg)

## Sequence Diagrams

- [Payment Flow](docs/sequence-diagrams/05-payment-flow.md)

## Tech Stack

- Go 1.25 + Gin (HTTP) + gRPC
- PostgreSQL (pgcrypto for PII encryption)
- Redis (caching + distributed locks)
- RabbitMQ (async event-driven via outbox pattern)
- Cloud Run (GCP) with auto-scaling
- OpenTelemetry (traces + metrics)

**Service-specific:** Midtrans QRIS integration (stub/real mode), webhook signature SHA-512 verification, gRPC server (h2c)

## API

See [OpenAPI Specification](docs/api-specifications/openapi-spec.yaml) and [AsyncAPI Specification](docs/api-specifications/asyncapi-spec.yaml).

## Running Locally

```bash
cp configs/.env.example configs/.env
make run
```

## Testing

```bash
make test          # unit tests
make test-coverage # with coverage report
```

## Deployment

CD via GitHub Actions → GCP Cloud Run (asia-southeast1).
Triggers on push to `main`.

Cloud Run URL: `https://payment-service-725nddkmwq-as.a.run.app`
