CREATE TABLE IF NOT EXISTS cart_items (
  buyer_sub TEXT NOT NULL,
  product_id TEXT NOT NULL,
  qty INTEGER NOT NULL CHECK (qty > 0),
  updated_at TIMESTAMPTZ NOT NULL,
  PRIMARY KEY (buyer_sub, product_id)
);
