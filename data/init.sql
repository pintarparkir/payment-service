-- payment-service Postgres schema.

BEGIN;

DO $$ BEGIN
  CREATE TYPE payment_status AS ENUM ('PENDING','PAID','FAILED','REFUNDED');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

CREATE TABLE IF NOT EXISTS payment (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  invoice_id    uuid NOT NULL,
  method        text NOT NULL DEFAULT 'QRIS',
  status        payment_status NOT NULL DEFAULT 'PENDING',
  pg_reference  text UNIQUE NOT NULL,
  amount_idr    bigint NOT NULL,
  created_at    timestamptz NOT NULL DEFAULT now(),
  paid_at       timestamptz
);
CREATE INDEX IF NOT EXISTS idx_payment_invoice ON payment(invoice_id);

CREATE TABLE IF NOT EXISTS outbox_event (
  id              bigserial PRIMARY KEY,
  aggregate_type  text NOT NULL,
  aggregate_id    text NOT NULL,
  event_type      text NOT NULL,
  payload         jsonb NOT NULL,
  created_at      timestamptz NOT NULL DEFAULT now(),
  published_at    timestamptz
);
CREATE INDEX IF NOT EXISTS idx_outbox_unpublished ON outbox_event(created_at) WHERE published_at IS NULL;

COMMIT;
