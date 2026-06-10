CREATE TABLE IF NOT EXISTS instantsearch_products (
  slug TEXT PRIMARY KEY,
  title TEXT NOT NULL CHECK (length(title) > 0),
  description TEXT NOT NULL CHECK (length(description) > 0),
  category TEXT NOT NULL CHECK (length(category) > 0),
  price_cents INTEGER NOT NULL CHECK (price_cents >= 0),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_instantsearch_products_category
  ON instantsearch_products(category);

CREATE INDEX IF NOT EXISTS idx_instantsearch_products_search_doc
  ON instantsearch_products
  USING GIN (to_tsvector('simple', title || ' ' || description));

DROP POLICY IF EXISTS instantsearch_products_read ON instantsearch_products;
ALTER TABLE instantsearch_products ENABLE ROW LEVEL SECURITY;
CREATE POLICY instantsearch_products_read
  ON instantsearch_products
  FOR SELECT
  USING (true);
