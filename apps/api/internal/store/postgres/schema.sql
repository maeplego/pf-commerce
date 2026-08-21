CREATE TABLE IF NOT EXISTS cart_items (
  buyer_sub TEXT NOT NULL,
  org_id TEXT NOT NULL DEFAULT 'org-demo-a',
  product_id TEXT NOT NULL,
  qty INTEGER NOT NULL CHECK (qty > 0),
  updated_at TIMESTAMPTZ NOT NULL,
  PRIMARY KEY (buyer_sub, org_id, product_id)
);

ALTER TABLE cart_items ADD COLUMN IF NOT EXISTS org_id TEXT NOT NULL DEFAULT 'org-demo-a';

ALTER TABLE cart_items DROP CONSTRAINT IF EXISTS cart_items_pkey;
ALTER TABLE cart_items ADD PRIMARY KEY (buyer_sub, org_id, product_id);
