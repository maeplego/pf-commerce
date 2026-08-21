-- Order process: own database. Payment mock has no card columns.

CREATE TABLE IF NOT EXISTS commerce_orders (
  id TEXT PRIMARY KEY,
  buyer_sub TEXT NOT NULL,
  org_id TEXT NOT NULL DEFAULT 'org-demo-a',
  status TEXT NOT NULL,
  cancel_reason TEXT NOT NULL DEFAULT '',
  amount_minor BIGINT NOT NULL CHECK (amount_minor >= 0),
  currency TEXT NOT NULL,
  idempotency_key TEXT NOT NULL,
  payment_id TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  UNIQUE (buyer_sub, org_id, idempotency_key)
);

ALTER TABLE commerce_orders ADD COLUMN IF NOT EXISTS org_id TEXT NOT NULL DEFAULT 'org-demo-a';

CREATE TABLE IF NOT EXISTS commerce_order_lines (
  order_id TEXT NOT NULL,
  product_id TEXT NOT NULL,
  sku TEXT NOT NULL,
  name TEXT NOT NULL,
  qty INTEGER NOT NULL CHECK (qty > 0),
  unit_price_minor BIGINT NOT NULL CHECK (unit_price_minor >= 0),
  currency TEXT NOT NULL,
  PRIMARY KEY (order_id, product_id)
);

-- Event store is the write model. commerce_orders is the list projection.
CREATE TABLE IF NOT EXISTS commerce_order_events (
  stream_id TEXT NOT NULL,
  version INTEGER NOT NULL CHECK (version > 0),
  event_id TEXT NOT NULL,
  event_type TEXT NOT NULL,
  occurred_at TIMESTAMPTZ NOT NULL,
  payload JSONB NOT NULL,
  PRIMARY KEY (stream_id, version),
  UNIQUE (event_id)
);

CREATE INDEX IF NOT EXISTS commerce_order_events_type_idx ON commerce_order_events (event_type);

-- Transactional outbox: written in the same TX as event append, drained to notify.
CREATE TABLE IF NOT EXISTS commerce_outbox (
  id TEXT PRIMARY KEY,
  aggregate_id TEXT NOT NULL,
  event_type TEXT NOT NULL,
  payload JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  published_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS commerce_outbox_unpublished_idx
  ON commerce_outbox (created_at) WHERE published_at IS NULL;
