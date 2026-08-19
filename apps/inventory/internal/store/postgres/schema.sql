-- Inventory process: own database. product_id is a catalog ULID with no FK.

CREATE TABLE IF NOT EXISTS inventory_sites (
  id TEXT PRIMARY KEY,
  code TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS inventory_stock_balances (
  site_id TEXT NOT NULL,
  product_id TEXT NOT NULL,
  qty INTEGER NOT NULL CHECK (qty >= 0),
  reserved_qty INTEGER NOT NULL CHECK (reserved_qty >= 0),
  version INTEGER NOT NULL DEFAULT 1,
  updated_at TIMESTAMPTZ NOT NULL,
  PRIMARY KEY (site_id, product_id),
  CHECK (reserved_qty <= qty)
);

CREATE TABLE IF NOT EXISTS inventory_stock_movements (
  id TEXT PRIMARY KEY,
  site_id TEXT NOT NULL,
  product_id TEXT NOT NULL,
  type TEXT NOT NULL,
  qty INTEGER NOT NULL CHECK (qty > 0),
  reason TEXT NOT NULL DEFAULT '',
  actor_id TEXT NOT NULL DEFAULT '',
  reservation_id TEXT,
  occurred_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS inventory_reservations (
  id TEXT PRIMARY KEY,
  site_id TEXT NOT NULL,
  product_id TEXT NOT NULL,
  qty INTEGER NOT NULL CHECK (qty > 0),
  order_id TEXT NOT NULL,
  status TEXT NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS inventory_reservations_order_held_idx
  ON inventory_reservations (order_id) WHERE status = 'held';
