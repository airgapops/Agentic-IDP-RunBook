-- =============================================================================
-- Template: E-Commerce
-- Usage: agctl db seed postgres --env production --file templates/ecommerce.sql
-- =============================================================================

CREATE TABLE IF NOT EXISTS products (
  id SERIAL PRIMARY KEY,
  name TEXT NOT NULL,
  description TEXT,
  price DECIMAL(10,2) NOT NULL DEFAULT 0.00,
  stock INTEGER NOT NULL DEFAULT 0,
  category TEXT,
  image_url TEXT,
  active BOOLEAN DEFAULT true,
  created_at TIMESTAMPTZ DEFAULT now()
);

CREATE TABLE IF NOT EXISTS orders (
  id SERIAL PRIMARY KEY,
  customer_name TEXT NOT NULL,
  customer_email TEXT,
  total DECIMAL(10,2) NOT NULL DEFAULT 0.00,
  status TEXT DEFAULT 'pending',
  created_at TIMESTAMPTZ DEFAULT now()
);

CREATE TABLE IF NOT EXISTS order_items (
  id SERIAL PRIMARY KEY,
  order_id INTEGER REFERENCES orders(id),
  product_id INTEGER REFERENCES products(id),
  quantity INTEGER NOT NULL DEFAULT 1,
  unit_price DECIMAL(10,2) NOT NULL
);

GRANT ALL ON products, orders, order_items TO zerops;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO zerops;

INSERT INTO products (id, name, description, price, stock, category) VALUES
  (1, 'Kubernetes Cluster Setup Guide', 'Complete guide for air-gapped K8s', 149.99, 100, 'guides'),
  (2, 'PostgREST API Template', 'Production-ready REST API config', 49.99, 500, 'templates'),
  (3, 'Zero-Trust Network Pack', 'NetworkPolicies + mTLS configs', 99.99, 200, 'security'),
  (4, 'Hermes AI Agent License', 'Annual license for Hermes', 299.99, 50, 'software'),
  (5, 'airgapOps Full Stack', 'Everything bundle', 399.99, 25, 'bundles')
ON CONFLICT (id) DO NOTHING;

SELECT setval('products_id_seq', (SELECT MAX(id) FROM products));
