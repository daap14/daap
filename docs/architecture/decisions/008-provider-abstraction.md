# 008. Provider Abstraction & Blueprint System

## Status
Accepted — supersedes ADR 007's typed tier fields

## Context
ADR 007 introduced platform-managed tiers to replace ADR 004's hardcoded infrastructure defaults. Each tier defines 12 configurable fields — but 8 of them (`instances`, `cpu`, `memory`, `storageSize`, `storageClass`, `pgVersion`, `poolMode`, `maxConnections`) are direct CNPG CRD mappings. This coupling flows through the entire stack: the tier model (Go struct with typed fields) maps to `ClusterParams`/`PoolerParams`, which feed template builders, which produce CNPG manifests. Every new CNPG feature DAAP wants to expose requires a new Go field, a new migration, new validation logic, a new API field, and a new OpenAPI entry.

The problem is twofold:

1. **CNPG exposes ~200+ configuration options; DAAP's tier model captures ~3-4%.** Platform users who need multi-zone HA, custom PostgreSQL tuning, WAL storage separation, backup to S3, or monitoring cannot configure them through DAAP. The typed tier model is a bottleneck.

2. **The tier conflates two concerns.** It is both the product-facing contract (what product users select) and the infrastructure implementation (how resources are configured). This makes it impossible to swap providers (e.g., AWS RDS) without restructuring the entire tier model, handler, template layer, and reconciler.

The project manifesto states: "DAAP is opinionated about how ownership is defined, not about how infrastructure must be defined." Product users should have a simple interface (pick a tier, get a database). Platform users should have maximum freedom to define the infrastructure behind each tier.

### Industry Research

Nine platforms were analyzed (Neon, Supabase, AWS RDS, Cloud SQL, Azure PG, Crossplane, Kratix, Humanitec, Backstage). The most relevant pattern is Crossplane's Claim/Composition model, where the user-facing schema (XRD) is defined separately from the infrastructure implementation (Composition). Multiple Compositions can satisfy the same XRD — analogous to multiple tiers backed by different blueprints.

The key insight: DAAP's tier is both the schema and the implementation today. Splitting them enables full provider flexibility without changing the product-facing API.

### Options Considered

| Option | Pros | Cons |
|--------|------|------|
| **Blueprint + Provider (chosen)** | Full CNPG coverage; provider-agnostic; product API unchanged; platform engineers write native YAML | Breaking change from v0.5; blueprints stored in DB not Git |
| Extend tier with more CNPG fields | No breaking change; incremental | Still coupled to CNPG; never reaches full coverage; each field requires migration |
| ConfigMap-based templates | GitOps-friendly; version-controlled | Requires K8s access to manage; cannot use DAAP's RBAC model |
| JSON schema + field mapping | Structured; validatable | Reinvents a worse version of YAML; hostile DX for K8s engineers |

## Decision

### 1. Three-Entity Separation: Tier, Blueprint, Provider

Split the current tier into three entities with clear separation of concerns:

- **Tier** (product-facing contract): `{ id, name, description, blueprint_id, destructionStrategy, backupEnabled }` — what the product user sees and selects. No infrastructure details. The product-facing API (`GET /tiers` for product users, `POST /databases { tier: "production" }`) is unchanged from v0.5.

- **Blueprint** (platform-facing infrastructure definition): `{ id, name, provider, manifests }` — the full provider-native infrastructure definition, stored as multi-document YAML with Go template placeholders. Opaque to product users.

- **Provider** (code-level interface): `Apply()`, `Delete()`, `CheckHealth()` — knows how to create, remove, and monitor resources for a specific infrastructure backend.

The relationship chain: `Database → Tier → Blueprint → Provider`.

### 2. The Blueprint Entity

#### Schema

| Field | Type | Description |
|-------|------|-------------|
| `id` | UUID | Primary key |
| `name` | VARCHAR(63) | Unique, DNS-compatible identifier (3-63 chars) |
| `provider` | VARCHAR(63) | Registered provider name (e.g., `"cnpg"`) |
| `manifests` | TEXT | Multi-document YAML with Go template placeholders |
| `created_at` | TIMESTAMPTZ | Row creation timestamp |
| `updated_at` | TIMESTAMPTZ | Row update timestamp |

#### Why YAML

Platform engineers work with CNPG in YAML — they write Cluster manifests, test with `kubectl apply`, iterate. The blueprint stores provider-native configuration in its native format. Forcing translation to JSON or typed fields is hostile DX.

#### Why Multi-Document

A production CNPG deployment is not just one Cluster and one Pooler. It can include a Cluster, multiple Poolers (read-write and read-only), ScheduledBackup, ConfigMap (Prometheus metrics queries), PodMonitor, and NetworkPolicy. A dev blueprint might produce 2 resources; a production blueprint might produce 8. The resource set varies per blueprint — it cannot be a fixed schema.

#### Template Variables

Available to all blueprints regardless of provider:

| Variable | Source | Example |
|----------|--------|---------|
| `{{ .Name }}` | Database name | `orders-db` |
| `{{ .Namespace }}` | K8s namespace (from config) | `daap-system` |
| `{{ .ID }}` | Database UUID | `550e8400-...` |
| `{{ .OwnerTeam }}` | Owning team name | `checkout` |
| `{{ .OwnerTeamID }}` | Owning team UUID | `550e8400-...` |
| `{{ .Tier }}` | Tier name | `production` |
| `{{ .TierID }}` | Tier UUID | `550e8400-...` |
| `{{ .Blueprint }}` | Blueprint name | `cnpg-prod-ha` |
| `{{ .Provider }}` | Provider name | `cnpg` |

Templates are rendered using Go's `text/template` with a `ProviderDatabase` struct as context.

### 3. Blueprint Immutability

Blueprints are **write-once**. They cannot be edited after creation. There is no `PATCH /blueprints/{id}` endpoint.

To change infrastructure configuration:
1. Create a new blueprint (e.g., `cnpg-prod-ha-v2`)
2. Update the tier to point to the new blueprint: `PATCH /tiers/{id} { "blueprintId": "new-uuid" }`

This ensures:
- Existing databases remain on the infrastructure they were created with (no silent changes).
- New databases get the updated infrastructure.
- Clear audit trail: each database's `tier_id` at creation time traces back to a specific immutable `blueprint_id`.
- No ambiguity: DAAP does not re-apply manifests after creation, so mutable blueprints would create a gap between "what the blueprint says" and "what's actually deployed."

Blueprints can be deleted only if no tier references them (`ON DELETE RESTRICT`).

### 4. Blueprint Validation

DAAP performs only structural validation on upload:
- Is it valid YAML?
- Does it split into one or more documents (`---` separated)?
- Does each document have `apiVersion`, `kind`, and `metadata.name`?
- Are Go template placeholders parseable (`text/template.Parse()` succeeds)?

DAAP does **not** validate against CRD schemas. This avoids coupling DAAP to CNPG CRD versions and keeps the blueprint format truly provider-agnostic. Platform users can pre-validate with `kubectl apply --dry-run=server` before uploading.

If a blueprint has invalid manifests, the error surfaces at database creation time when `provider.Apply()` fails. The database status transitions to `error`.

### 5. The Simplified Tier

The tier model is reduced to:

| Field | Type | Description |
|-------|------|-------------|
| `id` | UUID | Primary key |
| `name` | VARCHAR(63) | Unique, DNS-compatible identifier |
| `description` | TEXT | Human-readable description shown to product teams |
| `blueprint_id` | UUID | FK to `blueprints.id` (`ON DELETE RESTRICT`) |
| `destruction_strategy` | VARCHAR(20) | `freeze`, `archive`, or `hard_delete` (stored, not enforced until v0.9) |
| `backup_enabled` | BOOLEAN | Whether backups are enabled (stored, not enforced until v0.7) |
| `created_at` | TIMESTAMPTZ | Row creation timestamp |
| `updated_at` | TIMESTAMPTZ | Row update timestamp |

**Removed fields**: `instances`, `cpu`, `memory`, `storage_size`, `storage_class`, `pg_version`, `pool_mode`, `max_connections`. These are now defined inside the blueprint's YAML manifests.

The `blueprint_id` column is nullable at the database level (existing tiers from v0.5 have no blueprint), but the application enforces it as required for all new tier creations and updates.

### 6. The Provider Interface

```go
type Provider interface {
    Apply(ctx context.Context, db ProviderDatabase, manifests string) error
    Delete(ctx context.Context, db ProviderDatabase) error
    CheckHealth(ctx context.Context, db ProviderDatabase) (HealthResult, error)
}

type HealthResult struct {
    Status     string  // "provisioning", "ready", "error"
    Host       *string
    Port       *int
    SecretName *string
}
```

Providers are registered in a `Registry` at startup. The blueprint's `provider` field must match a registered provider — unknown providers are rejected at `POST /blueprints` with a `400` error.

For v0.6, DAAP ships a single provider: CNPG. The registry design accommodates future providers (RDS, etc.) without code changes to the tier/blueprint/handler layers.

### 7. CNPG Provider Implementation

The CNPG provider:

- **`Apply()`**: Renders Go templates, splits into documents, parses each into `unstructured.Unstructured`, injects mandatory labels (`daap.io/database`, `app.kubernetes.io/managed-by`), and applies each resource via the Kubernetes dynamic client.

- **`Delete()`**: Uses label-based discovery — scans for all resources in the database's namespace with label `daap.io/database={name}` and deletes them. This works regardless of how many resources the blueprint created.

- **`CheckHealth()`**: Gets the CNPG Cluster resource, reads `.status.phase`, and maps to `HealthResult`. Same logic as the current reconciler, moved into the provider.

### 8. Label-Based Deletion Strategy

When DAAP applies a blueprint's resources, it injects mandatory labels at render time:

```yaml
metadata:
  labels:
    daap.io/database: orders-db
    app.kubernetes.io/managed-by: daap
```

These labels are injected by DAAP code — they are not optional in the platform user's YAML template. Existing labels from the blueprint are preserved; DAAP adds its own on top.

On deletion, the provider scans for all resources with the `daap.io/database={name}` label and deletes them.

**Alternatives rejected:**
- **Store inventory in DB**: Adds complexity, can drift from reality if resources are manually deleted.
- **K8s owner references**: Requires two-phase apply, cross-CRD ownerRefs confuse operators, only works for K8s providers.
- **Labels + DB inventory**: Most resilient but most complex; overkill given that label injection is mandatory and reliable.

### 9. Reconciler Changes

The reconciler replaces its dependency on `k8s.ResourceManager.GetClusterStatus()` with `Provider.CheckHealth()`:

```go
// Before
type Reconciler struct {
    repo   database.Repository
    k8sMgr k8s.ResourceManager  // CNPG-specific
}

// After
type Reconciler struct {
    repo     database.Repository
    provider Provider             // provider-agnostic
}
```

Each provider implements `CheckHealth()` with its own logic. The reconciler calls `provider.CheckHealth()` and updates the database status based on the result — no provider-specific code in the reconciler.

For v0.6, the reconciler receives a single provider instance (CNPG). Multi-provider reconciliation (routing different databases to different providers) is deferred.

### 10. Authorization

Blueprint endpoints follow the existing RBAC model from ADR 006:

| Caller | Blueprint mutations (`POST`, `DELETE`) | Blueprint reads (`GET`) |
|--------|---------------------------------------|------------------------|
| **Superuser** | 403 (no role) | 403 (no role) |
| **Platform user** | Full access | Full response (all fields) |
| **Product user** | 403 | 403 |
| **Unauthenticated** | 401 | 401 |

Blueprints are platform-only for both reads and writes. Product users never see blueprints — they interact only with tiers.

Tier mutations (`POST`, `PATCH`, `DELETE`) move to `RequireRole("platform")` only (same as v0.5). Tier reads remain `RequireRole("platform", "product")` with role-based redaction: product users see `{ id, name, description }` only.

### 11. Breaking Change from v0.5

This is a **breaking change**. The v0.5 tier API (with typed CNPG fields) is replaced entirely. No automatic migration of existing tiers to blueprints is provided.

Rationale:
- DAAP is pre-v1. Breaking changes are acceptable.
- Auto-generating blueprints from existing tier fields would produce low-quality YAML (only the ~4% of CNPG config that was exposed).
- Platform users should author blueprints intentionally, leveraging CNPG features that were previously inaccessible.

The migration path for existing deployments:
1. Platform user writes blueprint YAML files that match or improve upon their existing tier configurations.
2. Platform user creates blueprints and tiers via the new API.
3. Existing databases in K8s continue running (DAAP does not touch live resources).
4. New databases are created through the new system.

### 12. Deferred Decisions

- **Base templates + overrides**: UX sugar where platform users start from a built-in base template and specify only overrides. Deferred until real usage patterns emerge.
- **Composable trait blocks**: Reusable concern-based blocks (compute, storage, HA, backup) that assemble into blueprints. Deferred for the same reason.
- **Drift detection**: DAAP does not compare desired state (blueprint) against live state. That is ArgoCD's responsibility.
- **Auto-remediation**: DAAP does not re-apply resources if they are modified or deleted externally.
- **Multi-resource health aggregation**: Health checks use the provider's primary signal (CNPG Cluster phase). Missing sidecars (Pooler, ScheduledBackup) are operational issues detected by monitoring, not by DAAP.
- **Multi-provider reconciliation**: The reconciler takes a single provider. Routing databases to different providers based on their blueprint is a future enhancement.

## Consequences

### Positive
- Platform teams can leverage the full CNPG configuration surface (~200+ options) through blueprint YAML, not just the 8 fields exposed by v0.5 tiers.
- Adding new CNPG features (WAL storage, monitoring, custom PG parameters, backup to S3) requires no code changes — platform users write it in their blueprint YAML.
- The provider interface decouples DAAP from CNPG. A future RDS or Supabase provider can be added by implementing three methods, with no changes to the tier, blueprint, handler, or reconciler layers.
- Product-facing API is unchanged — product users still `POST /databases { tier: "production" }` and `GET /tiers` returns `{ id, name, description }`.
- Blueprint immutability provides a clear audit trail: every database traces back to a specific, unchangeable infrastructure definition.
- Label-based deletion is simple, reliable, and provider-native for K8s backends.

### Negative
- Breaking change from v0.5: existing tiers must be recreated with blueprints. Not suitable if DAAP had external consumers, but acceptable pre-v1.
- Blueprint YAML is validated structurally only — invalid manifests are not caught until `provider.Apply()` at database creation time. This trades safety for flexibility.
- Blueprints are stored in the database, not in version-controlled configuration. Platform teams must use the API (not Git) to manage blueprints. (GitOps integration via a sync mechanism is a possible future enhancement.)
- Blueprint immutability means platform users must create new blueprints for every change, which could lead to blueprint proliferation. Naming conventions (e.g., `cnpg-prod-ha-v1`, `cnpg-prod-ha-v2`) help manage this.

### Neutral
- ADR 007's tier entity remains valid in its simplified form — it is still the product-facing named profile. The 8 CNPG-specific fields move into blueprints.
- ADR 004's principle (infrastructure is an implementation detail for product users) is preserved and strengthened — the abstraction layer is now the blueprint, not hardcoded defaults or typed tier fields.
- No new external dependencies. Uses Go's `text/template` (stdlib), existing `sigs.k8s.io/yaml`, and the existing Kubernetes dynamic client.
- The `blueprints` table uses the same PostgreSQL database, pgx driver, and repository pattern as all other DAAP entities.
