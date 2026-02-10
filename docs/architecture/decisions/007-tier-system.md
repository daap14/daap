# 007. Tier System

## Status
Accepted — supersedes ADR 004's hardcoded infrastructure defaults

## Context
ADR 004 internalized infrastructure parameters (`instances`, `storageSize`, `pgVersion`) by removing them from the API surface and hardcoding them as platform defaults in the template builders (`defaultInstances=1`, `defaultStorageSize=1Gi`, `defaultPGVersion="16"` in `internal/k8s/template/cluster.go`; `poolMode="transaction"` in `pooler.go`). This was the right decision for v0.2: consumers should not choose their own cluster sizing.

However, a single set of hardcoded defaults is insufficient for a production platform. Different workloads need different infrastructure profiles — a development database needs 1 instance with 512Mi of memory, while a production database might need 3 instances with 4Gi of memory and larger storage. Today, changing these defaults requires modifying template builder code and redeploying the platform.

The platform needs a way for platform teams to define named infrastructure profiles (tiers) that product teams can select when creating databases, without exposing raw infrastructure parameters.

### Options Considered

| Option | Pros | Cons |
|--------|------|------|
| **Named tiers (database-managed)** | Platform teams self-serve tier creation via API; tier parameters stored alongside other platform data; no external dependencies | Tier definitions live in the database, not in version-controlled config |
| ConfigMap-based tiers | Tiers are version-controlled K8s resources; familiar GitOps pattern | Requires K8s access to manage tiers; cannot use the same RBAC model as other DAAP resources; adds K8s dependency for a purely platform-level concept |
| Hardcoded tier enum | Simple; no new database table | Cannot add tiers without code changes and redeployment; inflexible |

## Decision

### 1. Tier Entity
Introduce a `tiers` table as a first-class platform entity. Each tier defines a complete infrastructure profile:

| Field | Type | Description |
|-------|------|-------------|
| `id` | UUID | Primary key |
| `name` | VARCHAR(63) | Unique, DNS-compatible identifier (3-63 chars, lowercase alphanumeric + hyphens) |
| `description` | TEXT | Human-readable description shown to product teams |
| `instances` | INT | CNPG cluster replica count (1-10) |
| `cpu` | VARCHAR(20) | K8s CPU resource quantity (e.g., `500m`, `2`) |
| `memory` | VARCHAR(20) | K8s memory resource quantity (e.g., `512Mi`, `4Gi`) |
| `storage_size` | VARCHAR(20) | K8s storage size (e.g., `1Gi`, `100Gi`) |
| `storage_class` | VARCHAR(255) | K8s StorageClass name; empty string uses cluster default |
| `pg_version` | VARCHAR(10) | PostgreSQL major version (e.g., `15`, `16`, `17`) |
| `pool_mode` | VARCHAR(20) | PgBouncer pool mode: `transaction`, `session`, or `statement` |
| `max_connections` | INT | PgBouncer max connections (10-10000) |
| `destruction_strategy` | VARCHAR(20) | `freeze`, `archive`, or `hard_delete` (stored, not enforced until v0.8) |
| `backup_enabled` | BOOLEAN | Whether backups are enabled (stored, not enforced until v0.6) |
| `created_at` | TIMESTAMPTZ | Row creation timestamp |
| `updated_at` | TIMESTAMPTZ | Row update timestamp |

CHECK constraints enforce valid ranges for `instances`, `max_connections`, `pool_mode`, and `destruction_strategy` at the database level.

### 2. Tier Is Required at Database Creation
`POST /databases` requires a `tier` field (tier name, resolved to UUID internally). There is no default tier and no implicit fallback. The platform must create at least one tier before any database can be created.

This replaces the hardcoded defaults from ADR 004. The template builders (`BuildCluster`, `BuildPooler`) become pure functions of tier parameters — no more hardcoded constants.

Pre-v0.5 databases that were created before the tier system will have `tier_id = NULL` in the database. The `tier_id` column is nullable at the schema level, but the application enforces it as required for all new creations.

### 3. Tier Immutability on Existing Databases
Modifying a tier's parameters (e.g., increasing `instances` from 1 to 3) does not retroactively change databases that were created with that tier. The tier's parameters are used at database creation time to configure the CNPG Cluster and Pooler resources. Once created, those K8s resources are independent of the tier definition.

This is a deliberate design choice: tier updates are for future database creations, not for changing running infrastructure. Changing running databases requires a separate mechanism (deferred post-v1).

### 4. Tier Deletion Guard
A tier cannot be deleted if it has active (non-deleted) databases. This is enforced by `ON DELETE RESTRICT` on the `databases.tier_id` foreign key, with an application-level sentinel error (`ErrTierHasDatabases`) that maps the FK violation to a 409 `TIER_HAS_DATABASES` response.

### 5. Forward-Looking Columns
Two tier columns are stored but not enforced at runtime in v0.5:

- `destruction_strategy`: Determines what happens when a database is deleted (`freeze` preserves the data, `archive` backs up then deletes, `hard_delete` removes immediately). Enforcement deferred to v0.8.
- `backup_enabled`: Whether automated backups are configured for databases using this tier. Enforcement deferred to v0.6.

These columns are included now so that platform teams can define their intent upfront. The values are stored, returned in API responses, and validated — they just have no runtime effect yet.

### 6. Authorization
Tier endpoints follow the existing RBAC model from ADR 006:

| Caller | Tier mutations (`POST`, `PATCH`, `DELETE`) | Tier reads (`GET`) |
|--------|-------------------------------------------|-------------------|
| **Superuser** | 403 (no role) | 403 (no role) |
| **Platform user** | Full access | Full response (all fields) |
| **Product user** | 403 | Redacted response: `id`, `name`, `description` only |
| **Unauthenticated** | 401 | 401 |

Tier mutations use `RequireRole("platform")`. Tier reads use `RequireRole("platform", "product")` with role-based response redaction in the handler.

Product users see only the tier's identity and description — they select a tier by name without knowing the underlying infrastructure parameters. This preserves the ADR 004 principle that infrastructure is an implementation detail.

### 7. Template Builder Refactoring
The template builders are refactored to accept tier parameters instead of using hardcoded defaults:

**`ClusterParams`** (expanded):
- `Name`, `Namespace` (existing)
- `Instances`, `CPU`, `Memory`, `StorageSize`, `StorageClass`, `PGVersion` (new, from tier)

**`PoolerParams`** (expanded):
- `Name`, `Namespace`, `ClusterName` (existing)
- `PoolMode`, `MaxConnections` (new, from tier)

The handler maps the resolved `Tier` object to `ClusterParams` and `PoolerParams` before calling the builders. The builders remain pure functions with no database or tier awareness.

### 8. Database Model Extension
The `Database` model gains two fields:
- `TierID *uuid.UUID` — FK to `tiers.id`, nullable for pre-v0.5 databases
- `TierName string` — transient field populated via LEFT JOIN (same pattern as `OwnerTeamName`)

The API contract uses the tier name (`"tier": "standard"`) in both request and response bodies, matching the `ownerTeam` pattern of using human-readable names externally and UUIDs internally.

## Consequences

### Positive
- Platform teams can define multiple infrastructure profiles without code changes or redeployment.
- Product teams select a tier by name without exposure to infrastructure details — maintains the ADR 004 principle.
- Template builders become pure functions of parameters, improving testability and eliminating hardcoded constants.
- Forward-looking columns (`destruction_strategy`, `backup_enabled`) allow platform teams to define intent before the features are enforced.
- The tier deletion guard prevents orphaned databases referencing a deleted tier.

### Negative
- Tier definitions live in the database, not in version-controlled configuration. Platform teams must use the API (not Git) to manage tiers.
- Tier immutability on existing databases means there is no built-in way to upgrade a running database's infrastructure profile. This must be solved separately in a future iteration.
- Adding a required `tier` field to `POST /databases` is a breaking change for any existing API consumers. Since the platform is pre-v1 and internal, this is acceptable.

### Neutral
- No new external dependencies. The tier system uses the existing PostgreSQL database, pgx driver, and Chi router.
- The `tier_id` column on `databases` is nullable to accommodate pre-v0.5 databases that were created without a tier. New databases always have a non-null `tier_id`.
- ADR 004's decision to internalize infrastructure parameters remains valid — tiers are the platform-managed abstraction layer that ADR 004 anticipated ("add a separate admin API in the future").
