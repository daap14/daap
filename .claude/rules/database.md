---
globs:
  - "internal/database/**"
  - "migrations/**"
---

# Database Conventions

## Migrations
- All schema changes via migrations — never modify the database manually
- Migrations must be reversible (both `up` and `down`)
- One logical change per migration file
- Name migrations descriptively: `001_create_users_table`, `002_add_email_index`
- Never modify an existing migration that has been applied; create a new one

## Schema Design
- Use UUIDs for primary keys (not auto-increment integers)
- Always include `created_at` and `updated_at` timestamps
- Use soft deletes (`deleted_at`) for user-facing data
- Add `NOT NULL` constraints by default; allow NULL only when explicitly needed
- Define explicit `ON DELETE` behavior for foreign keys (CASCADE, SET NULL, RESTRICT)

## Queries
- Use parameterized queries — never concatenate user input into SQL
- Select only needed columns — avoid `SELECT *`
- Index frequently queried columns (foreign keys, status fields, date ranges)
- Add composite indexes for common multi-column queries
- Use database transactions for multi-step operations

## Naming
- Tables: plural snake_case (`users`, `order_items`)
- Columns: singular snake_case (`email`, `created_at`)
- Foreign keys: `<singular_table>_id` (`user_id`, `order_id`)
- Indexes: `idx_<table>_<columns>` (`idx_users_email`)

## Timestamps
- Use the database's `NOW()` function for `updated_at` in UPDATE queries, not Go's `time.Now().UTC()`
- This ensures clock consistency between application and database servers
- `created_at` should use `DEFAULT NOW()` in the schema, not application-side timestamps
- Only use application-side timestamps when the database function is not available (e.g., soft-delete with explicit `deleted_at`)

## Performance
- Set connection pool limits appropriate for the environment
- Use EXPLAIN/ANALYZE for slow queries
- Avoid N+1 queries — use joins or batch loading
- Paginate large result sets
