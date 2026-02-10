# V1 Roadmap

## Overview

DAAP v1 is delivered across **10 iterations** (v0.1 through v0.10), progressing from a walking skeleton to a complete Database-as-a-Service platform. The arc follows a "document-and-secure-first" philosophy: OpenAPI and auth are established early so that every subsequent feature is documented and access-controlled from day one, avoiding costly retrofits.

v0.1 and v0.2 are complete. Eight iterations remain (v0.3 through v0.10). V1 is "done" when all success criteria in `docs/product/v1-vision.md` are satisfied.

---

## What's Already Done

- **v0.1 -- Foundation (Walking Skeleton)**: Go project scaffolding (Chi router, client-go, envconfig, slog), k3d dev environment with CNPG operator, `GET /health` with K8s connectivity check, response envelope middleware (`{data, error, meta}`), Makefile targets, CI/CD pipeline, Docker multi-stage build.

- **v0.2 -- Database CRUD with Provisioning**: Platform database (PostgreSQL 16 via pgx), migration infrastructure, `databases` table, CNPG Cluster + PgBouncer Pooler template builders, full REST lifecycle (`POST/GET/PATCH/DELETE /databases`), background reconciler (watches provisioning/ready/error statuses), K8s Secret reference for credentials (no plaintext), input validation, integration tests. Infrastructure parameters (instances, storageSize) internalized per ADR 004.

---

## Iteration Plan

### v0.3 -- OpenAPI Specification

**Goal**: Establish an OpenAPI v3 specification for all existing endpoints and set up the tooling so that every new endpoint added in future iterations is documented from the start.

**Features**:
- ADR for OpenAPI tooling choice (e.g., `swaggo/swag` annotation-based generation, or hand-maintained spec validated against implementation)
- OpenAPI v3 spec covering all v0.2 endpoints: `GET /health`, `POST/GET/PATCH/DELETE /databases`, `GET /databases/{id}`
- Document request/response schemas, the `{data, error, meta}` envelope, error formats, and status codes
- `GET /openapi.json` endpoint serving the spec as a static file (no authentication required)
- CI validation step: spec is checked against the implementation on every PR to prevent drift
- Establish the convention that every new endpoint added in subsequent iterations must include its OpenAPI definition in the same PR

**Depends on**: v0.2

**Rationale**: Placing OpenAPI first means the spec grows incrementally with the codebase rather than being bolted on at the end. Retroactive spec writing is error-prone and tedious -- documenting 5 existing endpoints now is manageable, but documenting 20+ endpoints at the end is not. The CI validation step ensures the spec never drifts from the implementation. Every subsequent iteration (auth, tiers, backup, etc.) adds its endpoints to the spec as part of the feature work, keeping documentation current at zero marginal cost.

---

### v0.4 -- Authentication and Authorization (API Keys + RBAC)

**Goal**: Secure the API with team-based API key authentication and role-based access control, enforcing the platform-team / product-team boundary before any platform-team-only features are built.

**Features**:
- API key model: `api_keys` table (id, key_hash, team_name, role [platform-team | product-team], created_at, revoked_at)
- API key endpoints: `POST /api-keys` (create), `GET /api-keys` (list), `DELETE /api-keys/{id}` (revoke) -- platform-team only
- Authentication middleware: extract API key from request header (`X-API-Key`), resolve to team identity and role, reject invalid/revoked keys with 401
- Authorization middleware: enforce role-based access per endpoint:
  - Platform-team: full access to all endpoints
  - Product-team: create databases, manage own databases only (`owner_team` must match), read-only access to tiers (when they exist)
- Ownership scoping on existing database endpoints: `GET /databases` automatically filtered by team for product-team role; `GET/PATCH/DELETE /databases/{id}` returns 404 (not 403) for databases not owned by the team
- API key hashing: store only bcrypt/argon2 hash of the key, never plaintext
- Health and OpenAPI endpoints remain unauthenticated
- Update OpenAPI spec with auth endpoints and security scheme definitions
- Update all existing integration tests to be auth-aware

**Depends on**: v0.3 (new endpoints must include OpenAPI definitions)

**Rationale**: Building auth before platform-team-only features (tiers, quotas) means RBAC is enforced from the moment those features land -- no retrofitting required. In the previous plan, auth at v0.6 meant tiers, destruction strategies, and backup were all built without access control and then retrofitted, which was the single largest regression risk. With auth in place early, every subsequent endpoint is born with role checks, and integration tests exercise the auth path from day one. The cost is that auth is built against a smaller API surface (just database CRUD), but the middleware is generic and applies cleanly to new routes as they are added.

---

### v0.5 -- Tier System

**Goal**: Replace hardcoded infrastructure defaults with a platform-managed tier system, so platform teams define named infrastructure profiles and product teams create databases by selecting a tier.

**Features**:
- Tier domain model and `tiers` table (id, name, description, instances, cpu, memory, storage_size, storage_class, pg_version, ha_replicas, pool_mode, max_connections, destruction_strategy, backup enabled flag, created_at, updated_at)
- Tier CRUD endpoints: `POST/GET/PATCH/DELETE /tiers` -- mutations enforced as platform-team only via the auth middleware from v0.4; `GET /tiers` and `GET /tiers/{id}` available to all roles
- Refactor `POST /databases` to require a `tier` field instead of using hardcoded defaults; cluster/pooler templates read config from the tier
- Refactor cluster and pooler template builders to accept tier parameters (instances, storage, PG version, pool mode, max connections)
- Add `tier_id` foreign key to `databases` table; `GET /databases/{id}` includes the tier name in the response
- Tier deletion guard: cannot delete a tier that has active databases
- Product-team-facing view: `GET /tiers` returns human-readable descriptions of guarantees only (no infrastructure parameters exposed)
- Update OpenAPI spec with tier endpoints and schemas

**Depends on**: v0.4 (tier mutations require platform-team role)

**Rationale**: The tier system is the foundational domain abstraction. Destruction strategies, backup configuration, and quotas all reference tiers. With auth already in place, tier mutations are access-controlled from the start -- platform-team role is enforced on `POST/PATCH/DELETE /tiers` without any placeholder logic. This eliminates the current hardcoded infrastructure defaults (ADR 004's `defaultInstances=1`, `defaultStorageSize=1Gi`), replacing them with explicit platform configuration.

---

### v0.6 -- Backup and Restore

**Goal**: Implement tier-driven backup configuration and point-in-time restore (PITR) so that tiers with backup enabled automatically configure CNPG ScheduledBackup + WAL archiving, and product teams can restore databases to a specific timestamp.

**Features**:
- Extend tier model with backup fields: backup_enabled, backup_frequency (cron), backup_retention_days, wal_archiving_enabled, object_storage_destination (S3 bucket/path)
- CNPG cluster template builder generates backup configuration when the tier has backup_enabled=true (ScheduledBackup resource, WAL archiving to S3-compatible object storage)
- Backup status visibility: `GET /databases/{id}` includes backup status (last successful backup timestamp, next scheduled backup, backup health)
- `POST /databases/{id}/restore` endpoint: accepts a target timestamp, bootstraps a new CNPG cluster from the nearest base backup + WAL replay, creates a new database record referencing the source
- Restore validation: only available for databases whose tier has backup enabled, only for `ready` databases with existing backups; ownership enforced via auth middleware (product teams can only restore their own databases)
- Reconciler: monitor backup health for databases with backup-enabled tiers
- Update OpenAPI spec with restore endpoint and backup-related response fields

**Depends on**: v0.5 (tier model defines backup configuration), v0.4 (restore ownership check)

**Rationale**: Backup and restore is the most infrastructure-heavy feature in v1 and requires S3-compatible object storage as an external dependency. It builds directly on the tier system (backup config per tier). With auth in place since v0.4, the restore endpoint's ownership requirement is enforced automatically. Archive-related backup retention is deferred to v0.8 when destruction strategies are built.

---

### v0.7 -- Per-Tier Resource Quotas

**Goal**: Allow platform teams to set resource quotas per tier, preventing product teams from over-provisioning and saturating the Kubernetes cluster.

**Features**:
- Quota model: `tier_quotas` table (tier_id, max_cpu_cores, max_memory_gib, max_storage_gib, max_databases)
- Quota endpoints: `GET /tiers/{id}/quota` (all roles), `PUT /tiers/{id}/quota` (platform-team only, enforced via auth middleware)
- Quota enforcement on `POST /databases`: calculate the resource cost of the requested tier (instances x cpu, instances x memory, storage), check against the tier's remaining quota, return 422 with clear error if exceeded
- Quota usage visibility: `GET /tiers/{id}/quota` returns both limits and current usage (calculated from active databases using that tier)
- Quota recalculation on database deletion: freed resources are reflected immediately in quota availability
- Update OpenAPI spec with quota endpoints and schemas

**Depends on**: v0.5 (tiers), v0.4 (platform-team role for quota management)

**Rationale**: Quotas are a guardrail feature -- they prevent abuse but don't enable new workflows. The platform functions correctly without quotas (it just has no limits). Placing quotas after tiers means the resource dimensions (cpu, memory, storage per tier) are defined. Auth is already in place, so `PUT /tiers/{id}/quota` is platform-team-only from the start.

---

### v0.8 -- Destruction Strategies and Lifecycle States

**Goal**: Implement the three destruction strategies (freeze, archive, hard delete) as tier-driven behavior, and add the `frozen` and `archived` lifecycle states with unfreeze support.

**Features**:
- Extend database status enum to include `frozen` and `archived`
- `DELETE /databases/{id}` reads the tier's destruction strategy and dispatches accordingly:
  - **Freeze**: mark status `frozen`, CNPG Cluster stays intact, connections refused (mechanism to be decided via ADR -- scale-to-zero vs. NetworkPolicy deny)
  - **Archive**: delete CNPG Cluster + Pooler from K8s, retain backups per the tier's retention policy (backups exist from v0.6), mark status `archived`
  - **Hard delete**: delete CNPG Cluster + Pooler from K8s, delete backups from object storage (if any), mark soft-deleted
- `POST /databases/{id}/unfreeze` endpoint: transitions a `frozen` database back to `ready` (restores connectivity)
- Reconciler updates: skip `frozen` and `archived` databases, handle drift detection for unfreezing
- Validation: only `frozen` databases can be unfrozen; only active (`ready`) databases can be destroyed
- Restore from archived databases: extend the restore endpoint (v0.6) to also accept `archived` databases as source, since their backups are retained
- Update OpenAPI spec with unfreeze endpoint and new status values

**Depends on**: v0.5 (tiers define destruction strategy), v0.6 (archive needs backup retention, hard delete needs backup deletion)

**Rationale**: Destruction strategies are placed after backup because two of three strategies interact with backups: archive retains them, hard delete removes them. Building destruction after backup means both archive and hard delete are fully functional on delivery -- no deferred "complete later" behavior. The freeze strategy is backup-independent and could ship earlier in isolation, but keeping all three strategies together maintains a coherent lifecycle model. Auth is already in place, so ownership checks on `DELETE` and unfreeze are enforced automatically.

---

### v0.9 -- Security Defaults

**Goal**: Harden the platform with mandatory security defaults that apply to all provisioned databases and API interactions.

**Features**:
- **TLS enforcement**: CNPG cluster template enables TLS certificates (CNPG auto-provisions them); connection info indicates `sslmode=verify-full`
- **pgAudit**: add `pgAudit` to `shared_preload_libraries` in the CNPG cluster template for all databases
- **NetworkPolicies**: provision a default-deny NetworkPolicy per database namespace with explicit allow rules for the owning team's application namespaces
- **Audit logging**: log all DAAP API actions (team identity from auth middleware, action type, resource ID, timestamp) to a structured `audit_log` table; `GET /audit-logs` endpoint for platform-team role
- Update OpenAPI spec with audit log endpoint and security-related response changes

**Depends on**: v0.4 (audit logging needs team identity from auth), v0.5 (CNPG template changes require tier-driven provisioning)

**Rationale**: Security defaults are additive configurations on the CNPG cluster template and API middleware -- they harden existing behavior without changing it. TLS and pgAudit are template parameters that apply to new databases (existing databases are unaffected unless re-provisioned). NetworkPolicies are provisioned alongside the CNPG Cluster. Audit logging wraps the auth middleware's team identity into a persistent log. Placing security after the core features (tiers, backup, destruction) means the template is stable and changes are purely additive.

---

### v0.10 -- Extension Tracking

**Goal**: Provide read-only visibility into PostgreSQL extensions enabled per database, for operational awareness and upgrade planning.

**Features**:
- Reconciler periodically queries each ready database's `pg_extension` catalog (connects using credentials from the K8s Secret referenced by `secretName`)
- Store extensions per database in a `database_extensions` table (database_id, extension_name, version, last_checked_at)
- `GET /databases/{id}` includes an `extensions` field listing enabled extensions with versions
- The platform does not manage extensions -- product teams enable them via SQL; the platform only observes
- Update OpenAPI spec with extension fields in database response schema

**Depends on**: v0.9 (NetworkPolicies must allow reconciler-to-database connectivity), v0.2 (reconciler infrastructure)

**Rationale**: Extension tracking is the lightest feature in v1 -- a read-only reconciler enhancement with no mutations and no new API endpoints beyond extending an existing response. It is isolated as the final iteration because it has a subtle infrastructure dependency: the reconciler must connect to managed databases over the network, which means NetworkPolicies (v0.9) must include an allow rule for DAAP-to-database traffic. Keeping it last ensures that dependency is satisfied.

---

## Dependency Graph

```
v0.1 (Foundation)
 |
v0.2 (Database CRUD)
 |
v0.3 (OpenAPI)
 |
v0.4 (Auth & RBAC)
 |
v0.5 (Tier System)
 |\
 | \___________
 |             |
v0.6 (Backup) |
 |             |
 +-----+-------+
       |
v0.7 (Quotas)
       |
v0.8 (Destruction Strategies)
       |
v0.9 (Security Defaults)
       |
v0.10 (Extensions)
       |
   v1.0 RELEASE
```

**Critical path**: v0.1 -> v0.2 -> v0.3 -> v0.4 -> v0.5 -> v0.6 -> v0.8 -> v0.9 -> v0.10

v0.7 (Quotas) depends on v0.5 (tiers) and v0.4 (auth) but not on v0.6 (backup), so it could theoretically run in parallel with v0.6. However, v0.8 (destruction strategies) depends on both v0.6 and v0.5, creating a merge point. v0.7 is sequenced after v0.6 to avoid parallel iteration complexity.

---

## Risk Notes

- **Object storage dependency (v0.6)**: Backup and restore requires S3-compatible object storage. Local dev needs MinIO or a similar S3-compatible service in the k3d environment. This should be spiked during v0.5 (tier system) to validate the infrastructure before backup work begins.

- **OpenAPI tooling choice (v0.3)**: The ADR for OpenAPI tooling (annotation-based generation vs. hand-maintained spec) will determine the maintenance burden for all subsequent iterations. A poor choice here creates compounding friction. Prefer a code-generation approach (e.g., `swaggo/swag`) that keeps the spec close to the handler code.

- **Auth on a small surface (v0.4)**: Building auth against only the database CRUD endpoints means the middleware is tested against a limited set of routes. The risk is that edge cases emerge when tier, backup, and quota endpoints are added later. Mitigation: design the middleware to be route-agnostic (role annotation per route group, not per handler).

- **Freeze implementation uncertainty (v0.8)**: The mechanism for making a frozen database refuse connections (scale CNPG instances to 0 vs. applying a NetworkPolicy deny rule) needs an ADR. Scale-to-zero is simpler but makes unfreeze slower (CNPG must re-provision instances). A NetworkPolicy deny is faster to reverse but more complex to implement. This ADR should be written during v0.5 or v0.6 so the decision is made before v0.8 starts.

- **Extension tracking requires database connectivity (v0.10)**: The reconciler needs to connect to each managed database to query `pg_extension`. This requires the database credentials (from K8s Secrets) and network access from the DAAP namespace to database namespaces. NetworkPolicies (v0.9) must include an explicit allow rule for this traffic path. If the allow rule is missing, extension tracking silently fails.

- **Tier definition immutability**: Changing a tier's infrastructure parameters (e.g., increasing storage) does not retroactively change existing databases. The vision states this explicitly, but it means database-level overrides or migration tooling may be needed post-v1 for fleet-wide changes.
