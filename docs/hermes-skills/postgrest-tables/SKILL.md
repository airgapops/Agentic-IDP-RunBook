---
name: postgrest-tables
description: >-
  Create tables in PostgreSQL and verify the auto-generated PostgREST endpoints.
  Use this skill when asked to: create a table, add columns, create schema,
  set up database tables, or provision REST endpoints.
  Executes SQL via kubectl exec into the PostgreSQL pod.
version: 1.0.0
metadata:
  author: platform-team
  tags: [postgrest, postgresql, tables, sql, rest]
---

# 📊 SKILL: Create Tables + REST Endpoints

## When to Use
- User says "create a table for X"
- User says "add columns to X"
- User says "set up tables for products, orders, etc."
- User describes data they want to store

## CRITICAL RULES
1. **EXECUTE all commands** — never show steps
2. **Use `kubectl --kubeconfig=/root/ryzen-kubeconfig.yaml`** to exec into postgres pod
3. **ALWAYS grant permissions** to the `zerops` role after creating tables
4. **ALWAYS verify** by calling PostgREST endpoints with curl

## Database Connection
- Host: postgres (service name in same namespace)
- Port: 5432
- User: zerops
- Password: zerops123
- Database: app
- Namespace: dp-default-default-development-f8e58905

## EXECUTE ALL STEPS BELOW

### Step 1: Identify the PostgreSQL pod

```bash
PG_POD=$(kubectl --kubeconfig=/root/ryzen-kubeconfig.yaml get pods -n dp-default-default-development-f8e58905 -l openchoreo.dev/component=postgres -o jsonpath='{.items[0].metadata.name}')
echo "PostgreSQL pod: $PG_POD"
```

### Step 2: Create tables via kubectl exec

Convert the user's table descriptions to SQL CREATE TABLE statements and run them:

```bash
kubectl --kubeconfig=/root/ryzen-kubeconfig.yaml exec -n dp-default-default-development-f8e58905 $PG_POD -- psql -U zerops -d app -c "
CREATE TABLE IF NOT EXISTS {TABLE_NAME} (
  {COLUMN_DEFINITIONS}
);

-- Grant full access to zerops role (needed for PostgREST)
GRANT ALL ON {TABLE_NAME} TO zerops;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO zerops;
"
```

> **IMPORTANT**: Always use `IF NOT EXISTS` to make it idempotent.
> **IMPORTANT**: Always grant permissions after table creation.
> **IMPORTANT**: For SERIAL/auto-increment columns, grant SEQUENCES access.

### Step 3: Insert sample data (optional, if user asks)

```bash
kubectl --kubeconfig=/root/ryzen-kubeconfig.yaml exec -n dp-default-default-development-f8e58905 $PG_POD -- psql -U zerops -d app -c "
INSERT INTO {TABLE_NAME} ({COLUMNS}) VALUES
  ({VALUES_1}),
  ({VALUES_2}),
  ({VALUES_3})
ON CONFLICT DO NOTHING;
"
```

### Step 4: Reload PostgREST schema cache

```bash
PGRST_DEP=$(kubectl --kubeconfig=/root/ryzen-kubeconfig.yaml get deploy -n dp-default-default-development-f8e58905 -l openchoreo.dev/component=postgrest -o jsonpath='{.items[0].metadata.name}')
kubectl --kubeconfig=/root/ryzen-kubeconfig.yaml rollout restart deployment $PGRST_DEP -n dp-default-default-development-f8e58905
sleep 10
```

### Step 5: Verify REST endpoints

```bash
echo "=== List all tables ==="
curl -sk https://postgrest.zerops.internal/ | python3 -m json.tool 2>/dev/null || curl -sk https://postgrest.zerops.internal/

echo "=== GET {TABLE_NAME} ==="
curl -sk https://postgrest.zerops.internal/{TABLE_NAME} | python3 -m json.tool 2>/dev/null

echo "=== POST test ==="
curl -sk -X POST https://postgrest.zerops.internal/{TABLE_NAME} \
  -H "Content-Type: application/json" \
  -H "Prefer: return=representation" \
  -d '{TEST_JSON}' | python3 -m json.tool 2>/dev/null
```

### Step 6: Report to user

Report in this format:

```
✅ Tables created and REST endpoints verified!

📊 Tables:
  • {TABLE_NAME_1}: {COLUMN_COUNT} columns
  • {TABLE_NAME_2}: {COLUMN_COUNT} columns

🌐 REST Endpoints:
  • GET    https://postgrest.zerops.internal/{TABLE_NAME_1}         — List all
  • GET    https://postgrest.zerops.internal/{TABLE_NAME_1}?id=eq.1 — Get by ID
  • POST   https://postgrest.zerops.internal/{TABLE_NAME_1}         — Create
  • PATCH  https://postgrest.zerops.internal/{TABLE_NAME_1}?id=eq.1 — Update
  • DELETE https://postgrest.zerops.internal/{TABLE_NAME_1}?id=eq.1 — Delete

🔍 Useful filters:
  • ?name=like.*keyword*     — Search
  • ?order=created_at.desc   — Sort
  • ?limit=10&offset=0       — Paginate
  • ?select=id,name          — Select columns

🖥️ Database access: kubectl exec into postgres pod (no pgAdmin needed)

Next actions:
  • "Inserta datos de ejemplo en {TABLE_NAME}"
  • "Crea otra tabla para {ENTITY}"
  • "Promueve postgrest a staging"
```

## SQL Type Reference

| User says | SQL type |
|-----------|----------|
| id, número auto | `SERIAL PRIMARY KEY` |
| texto, nombre | `TEXT NOT NULL` |
| texto opcional | `TEXT` |
| precio, decimal | `DECIMAL(10,2)` |
| entero, cantidad | `INTEGER DEFAULT 0` |
| fecha | `DATE` |
| fecha y hora | `TIMESTAMPTZ DEFAULT now()` |
| booleano, activo | `BOOLEAN DEFAULT true` |
| email | `TEXT UNIQUE` |
| relación, FK | `INTEGER REFERENCES {other_table}(id)` |
| JSON | `JSONB DEFAULT '{}'::jsonb` |

## Example: User says "Create a products table with name, price, stock"

SQL to generate:

```sql
CREATE TABLE IF NOT EXISTS products (
  id SERIAL PRIMARY KEY,
  name TEXT NOT NULL,
  price DECIMAL(10,2) NOT NULL DEFAULT 0,
  stock INTEGER NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ DEFAULT now()
);
GRANT ALL ON products TO zerops;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO zerops;
```
