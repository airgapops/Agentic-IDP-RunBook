-- =============================================================================
-- Template: Todo App (Default)
-- Usage: agctl db seed postgres --env production --file templates/todos.sql
-- =============================================================================

CREATE TABLE IF NOT EXISTS categories (
  id SERIAL PRIMARY KEY,
  name TEXT NOT NULL,
  color TEXT DEFAULT '#6b7280'
);

CREATE TABLE IF NOT EXISTS todos (
  id SERIAL PRIMARY KEY,
  title TEXT NOT NULL,
  description TEXT,
  completed BOOLEAN DEFAULT false,
  priority INTEGER DEFAULT 3,
  category_id INTEGER REFERENCES categories(id),
  created_at TIMESTAMPTZ DEFAULT now()
);

GRANT ALL ON categories TO zerops;
GRANT ALL ON todos TO zerops;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO zerops;

INSERT INTO categories (id, name, color) VALUES
  (1, 'Trabajo', '#3b82f6'),
  (2, 'Personal', '#22c55e'),
  (3, 'Urgente', '#ef4444')
ON CONFLICT (id) DO NOTHING;

INSERT INTO todos (id, title, description, completed, priority, category_id) VALUES
  (1, 'Configurar cluster K8s', 'Setup completo del cluster airgap', true, 5, 1),
  (2, 'Documentar arquitectura', 'Crear diagramas de red y flujo', false, 4, 1),
  (3, 'Comprar cafe', 'Para las sesiones de noche', false, 2, 2),
  (4, 'Fix NetworkPolicies', 'Migrar de cron a additive policies', true, 5, 3),
  (5, 'Grabar demo', 'Video de PostgREST + Zero-Trust', false, 4, 1)
ON CONFLICT (id) DO NOTHING;

SELECT setval('categories_id_seq', (SELECT MAX(id) FROM categories));
SELECT setval('todos_id_seq', (SELECT MAX(id) FROM todos));
