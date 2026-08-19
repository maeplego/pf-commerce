-- Order process: own database. Payment mock has no card columns.

CREATE TABLE IF NOT EXISTS commerce_orders (
  id TEXT PRIMARY KEY,
  buyer_sub TEXT NOT NULL,
  status TEXT NOT NULL,
  cancel_reason TEXT NOT NULL DEFAULT '',
  amount_minor BIGINT NOT NULL CHECK (amount_minor >= 0),
  currency TEXT NOT NULL,
  idempotency_key TEXT NOT NULL,
  payment_id TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  UNIQUE (buyer_sub, idempotency_key)
);

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
