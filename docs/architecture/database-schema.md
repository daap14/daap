# Database Schema

This document describes the database schema for DAAP's platform database (PostgreSQL 16). The platform database stores metadata about managed databases — it does not store user application data.

## Tables

### `databases`

Stores metadata for each managed database instance. Each row represents a CNPG-backed PostgreSQL database that DAAP provisions and manages on behalf of a team.

```sql
CREATE TABLE databases (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(63) NOT NULL,
    owner_team VARCHAR(255) NOT NULL,
    purpose TEXT NOT NULL DEFAULT '',
    namespace VARCHAR(255) NOT NULL DEFAULT 'default',
    cluster_name VARCHAR(255) NOT NULL,
    pooler_name VARCHAR(255) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'provisioning',
    host VARCHAR(255),
    port INTEGER,
    secret_name VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);
```

#### Column Details

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| `id` | UUID | PK, auto-generated | Unique identifier for the database record |
| `name` | VARCHAR(63) | NOT NULL | User-provided name; unique among non-deleted records. Limited to 63 chars to comply with K8s naming constraints |
| `owner_team` | VARCHAR(255) | NOT NULL | Team that owns this database |
| `purpose` | TEXT | NOT NULL, default `''` | Free-text description of the database's purpose |
| `namespace` | VARCHAR(255) | NOT NULL, default `'default'` | Kubernetes namespace where the CNPG Cluster is deployed |
| `cluster_name` | VARCHAR(255) | NOT NULL | Name of the CNPG Cluster custom resource in Kubernetes |
| `pooler_name` | VARCHAR(255) | NOT NULL | Name of the CNPG Pooler (PgBouncer) custom resource |
| `status` | VARCHAR(20) | NOT NULL, default `'provisioning'` | Current lifecycle status (see Status Values below) |
| `host` | VARCHAR(255) | nullable | PostgreSQL connection host (populated when ready) |
| `port` | INTEGER | nullable | PostgreSQL connection port (populated when ready) |
| `secret_name` | VARCHAR(255) | nullable | Kubernetes Secret name containing credentials (populated when ready) |
| `created_at` | TIMESTAMPTZ | NOT NULL, default `NOW()` | Record creation timestamp |
| `updated_at` | TIMESTAMPTZ | NOT NULL, default `NOW()` | Last update timestamp |
| `deleted_at` | TIMESTAMPTZ | nullable | Soft delete timestamp; NULL means active |

#### Status Values

The `status` column follows a lifecycle state machine:

```
provisioning --> ready --> deleting --> deleted
     |                       ^
     +--> error -------------+
              |
              +--> provisioning  (retry)
```

| Status | Description |
|--------|-------------|
| `provisioning` | CNPG Cluster and Pooler resources have been submitted to Kubernetes; waiting for them to become ready |
| `ready` | The database is fully provisioned, connection details are populated, and it is available for use |
| `error` | Provisioning or operation failed; the `purpose` field or a future `error_message` field may contain details |
| `deleting` | Deletion has been requested; CNPG resources are being removed from Kubernetes |
| `deleted` | The database has been fully removed; the record is soft-deleted (`deleted_at` is set) |

#### Indexes

```sql
-- Unique name among active (non-deleted) databases
CREATE UNIQUE INDEX idx_databases_name_active ON databases (name) WHERE deleted_at IS NULL;

-- Filter by owner team
CREATE INDEX idx_databases_owner_team ON databases (owner_team);

-- Filter by status
CREATE INDEX idx_databases_status ON databases (status);
```

- **`idx_databases_name_active`**: Partial unique index ensuring no two active databases share the same name. Deleted databases are excluded, allowing name reuse after deletion.
- **`idx_databases_owner_team`**: Supports filtering the database list by team ownership, which is expected to be the most common query pattern.
- **`idx_databases_status`**: Supports filtering by status (e.g., finding all `provisioning` databases for the reconciliation loop).

## Migration Convention

Migrations live in `migrations/` at the project root. Each migration consists of a pair of files:

```
migrations/
├── 001_create_databases_table.up.sql
├── 001_create_databases_table.down.sql
├── 002_remove_credentials_add_secret_ref.up.sql
├── 002_remove_credentials_add_secret_ref.down.sql
├── 003_drop_dbname.up.sql
└── 003_drop_dbname.down.sql
```

- **Numbering**: Sequential three-digit prefix (`001`, `002`, `003`, ...).
- **Naming**: `NNN_description.up.sql` for the forward migration, `NNN_description.down.sql` for the rollback.
- **Reversibility**: Every `.up.sql` must have a corresponding `.down.sql` that fully reverses the change.
- **Idempotency**: Use `IF NOT EXISTS` / `IF EXISTS` where appropriate, though golang-migrate tracks applied versions and will not re-run migrations.
- **One logical change per migration**: Do not combine unrelated schema changes in a single migration.

### Migration 001: Create Databases Table

#### `migrations/001_create_databases_table.up.sql`

```sql
CREATE TABLE databases (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(63) NOT NULL,
    owner_team VARCHAR(255) NOT NULL,
    purpose TEXT NOT NULL DEFAULT '',
    namespace VARCHAR(255) NOT NULL DEFAULT 'default',
    cluster_name VARCHAR(255) NOT NULL,
    pooler_name VARCHAR(255) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'provisioning',
    host VARCHAR(255),
    port INTEGER,
    dbname VARCHAR(255),
    username VARCHAR(255),
    password VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX idx_databases_name_active ON databases (name) WHERE deleted_at IS NULL;
CREATE INDEX idx_databases_owner_team ON databases (owner_team);
CREATE INDEX idx_databases_status ON databases (status);
```

#### `migrations/001_create_databases_table.down.sql`

```sql
DROP TABLE IF EXISTS databases;
```

The down migration drops the entire table, which implicitly removes all associated indexes.

## Design Notes

### Soft Deletes
The `deleted_at` column enables soft deletes. Queries for active databases must include `WHERE deleted_at IS NULL`. The partial unique index on `name` respects this, allowing a previously deleted database name to be reused.

### Connection Details
The `host`, `port`, and `secret_name` columns are nullable because they are populated asynchronously after the CNPG Cluster becomes ready. The reconciliation loop updates these fields when the cluster reaches a healthy state. Consumers retrieve credentials directly from the Kubernetes Secret referenced by `secret_name`.

### Future Considerations
- An `error_message` column may be added to store detailed error information when `status` is `error`.
- A `spec` JSONB column may be added to store the full requested CNPG Cluster spec for audit purposes.
- Additional indexes may be needed as query patterns emerge (e.g., composite index on `owner_team` + `status`).
