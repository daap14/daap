# 002. Platform Database

## Status
Accepted

## Context
DAAP needs to persist metadata about managed databases — name, owner team, purpose, provisioning status, connection details, and timestamps. Iteration v0.2 introduces CRUD endpoints for the `databases` resource, which requires a relational data store with support for:

- Concurrent read/write access from multiple API server instances.
- Filtering, pagination, and partial text matching on list queries.
- Transactional integrity for multi-step operations (e.g., create record then trigger K8s provisioning).
- A production-grade database that matches what we will run in deployed environments.

We also need to choose a Go database driver, a migration tool, and a strategy for dev/test database management.

## Decision

### Database: PostgreSQL 16

PostgreSQL was chosen over SQLite and other alternatives.

| Option | Pros | Cons |
|--------|------|------|
| **PostgreSQL** | Production-grade, concurrent access, rich query features (ILIKE, full-text search, JSON), strong ecosystem, dogfoods the tech we manage | Requires a running server in dev and CI |
| SQLite | Zero-dependency, embedded, easy setup | Single-writer concurrency model, limited query features, does not match production, no network access |
| MySQL | Widely used, good tooling | Fewer advanced features than PostgreSQL, does not align with CNPG focus |

PostgreSQL wins because:
1. **Dogfooding**: DAAP manages PostgreSQL databases via CNPG. Using PostgreSQL for our own metadata store means we exercise the same technology we offer to users.
2. **Concurrent access**: PostgreSQL handles concurrent connections natively, which is required when running multiple API server replicas.
3. **Query capabilities**: Native support for `ILIKE` (case-insensitive partial matching), array operations, and `JSONB` columns if needed later. Filtering, sorting, and pagination are first-class.
4. **Production parity**: The dev and test database engine matches production exactly, eliminating behavior differences between environments.

### Driver: `github.com/jackc/pgx/v5`

pgx was chosen over `database/sql` + `lib/pq` and GORM.

| Option | Pros | Cons |
|--------|------|------|
| **pgx** | Pure Go, context-aware, pgxpool for connection pooling, PostgreSQL-native types, high performance | PostgreSQL-only (no database abstraction) |
| `database/sql` + `lib/pq` | Standard library interface, database-agnostic | `lib/pq` is in maintenance mode, no built-in pooling, weaker type support |
| GORM | Full ORM, migrations built-in, rapid prototyping | Heavy abstraction, hides SQL, harder to optimize, magic behavior |

pgx wins because:
1. **Pure Go, actively maintained**: pgx is the most actively developed PostgreSQL driver for Go, with no CGO dependency.
2. **Context-aware**: All operations accept `context.Context`, enabling proper timeout and cancellation propagation.
3. **Connection pooling**: `pgxpool` provides built-in connection pooling with configurable limits, health checks, and idle connection management — no external pooler needed at the application level.
4. **PostgreSQL-native types**: Direct support for UUID, TIMESTAMPTZ, arrays, JSONB, and other PostgreSQL types without manual scanning.
5. **Performance**: pgx uses the PostgreSQL binary protocol by default, which is faster than the text protocol used by `lib/pq`.

### Connection Configuration

Connection is configured via the `DATABASE_URL` environment variable, following 12-factor app principles:

```
DATABASE_URL=postgres://daap:daap@localhost:5432/daap?sslmode=disable
```

The Config struct (in `internal/config/`) will be extended:
```go
type Config struct {
    // ... existing fields ...
    DatabaseURL string `envconfig:"DATABASE_URL" required:"true"`
}
```

Connection pool settings use sensible defaults with environment variable overrides:
- `DB_MAX_CONNS`: Maximum pool size (default: 25)
- `DB_MIN_CONNS`: Minimum idle connections (default: 5)
- `DB_MAX_CONN_LIFETIME`: Maximum connection lifetime (default: 1h)
- `DB_MAX_CONN_IDLE_TIME`: Maximum idle time before closing (default: 30m)

### Migration Tool: golang-migrate

`github.com/golang-migrate/migrate/v4` with the PostgreSQL driver.

| Option | Pros | Cons |
|--------|------|------|
| **golang-migrate** | CLI + library, version tracking, up/down migrations, widely used | Separate tool to install |
| goose | Simple, Go-based migrations | Smaller community, fewer driver options |
| Atlas | Declarative schema management, modern | Newer, less battle-tested, commercial features |
| GORM AutoMigrate | Zero config | No down migrations, unpredictable behavior, requires GORM |

golang-migrate wins because:
1. **Explicit up/down migrations**: Every schema change has a reversible counterpart, following the project's database conventions.
2. **CLI and library**: Can be run via `make migrate` (CLI) or embedded in the application for automated migrations.
3. **Version tracking**: Tracks applied migrations in a `schema_migrations` table, preventing duplicate application.
4. **PostgreSQL driver**: Native PostgreSQL support via pgx-compatible driver.

Migration file convention:
```
migrations/
├── 001_create_databases_table.up.sql
├── 001_create_databases_table.down.sql
├── 002_add_some_index.up.sql
└── 002_add_some_index.down.sql
```

### Dev Setup: PostgreSQL 16 in Docker Compose

A PostgreSQL 16 container will be added to `docker-compose.yml` for local development:
- Service name: `postgres`
- Image: `postgres:16-alpine` (pinned minor version)
- Port: 5432 (mapped to host)
- Volume: named volume for data persistence across restarts
- Healthcheck: `pg_isready` command
- Default credentials: `daap` / `daap` / `daap` (user / password / database)

### Testing Strategy: Real PostgreSQL with Transaction Rollback

Tests run against a real PostgreSQL instance (not mocked), using transaction rollback for isolation.

| Approach | Pros | Cons |
|----------|------|------|
| **Real DB + transaction rollback** | Tests real SQL, catches driver issues, fast cleanup | Requires PostgreSQL in CI |
| Mocked database | No external deps, fast | Misses SQL bugs, tests the mock not the DB |
| Real DB + truncation | Tests real SQL | Slower cleanup, ordering issues |

Transaction rollback approach:
1. Each test starts a transaction.
2. All database operations within the test use that transaction.
3. After the test, the transaction is rolled back — no data persists.
4. Tests can run in parallel since each has its own transaction.

This requires PostgreSQL in CI, which is straightforward with GitHub Actions' `services` feature.

## Consequences

### Positive
- PostgreSQL dogfoods the same technology DAAP manages, building team expertise and confidence.
- pgx provides a modern, performant, context-aware database driver with built-in connection pooling.
- golang-migrate ensures schema changes are versioned, reversible, and trackable.
- Transaction rollback testing catches real SQL issues while maintaining test isolation and speed.
- 12-factor configuration via `DATABASE_URL` works seamlessly in Docker, CI, and Kubernetes.

### Negative
- PostgreSQL requires a running container in dev and CI environments, unlike SQLite's zero-dependency model. This adds to the dev setup complexity, but docker-compose mitigates this.
- pgx is PostgreSQL-only. If DAAP ever needed to support a different database backend, the driver layer would need to be replaced. Given DAAP's focus on CNPG/PostgreSQL, this is an acceptable trade-off.
- golang-migrate requires a separate CLI tool installation for running migrations outside the application.

### Neutral
- Connection pool tuning parameters are exposed as environment variables, allowing per-environment optimization without code changes.
- The `schema_migrations` table added by golang-migrate is a small metadata overhead in the database.
