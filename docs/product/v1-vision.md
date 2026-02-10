# DAAP V1 Vision

## What V1 Is

DAAP v1 is a **self-hosted Database-as-a-Service platform** that lets product teams create and manage PostgreSQL databases through a REST API, while platform teams control infrastructure guarantees through tiers, quotas, and RBAC.

V1 runs on a single Kubernetes cluster with CloudNativePG as the provisioning backend. It enforces the responsibility model defined in `docs/product/responsibility-model.md`: product teams own their data and schema, the platform owns availability, durability, and infrastructure.

V1 is API-only. No CLI, no web portal, no developer dashboard. The OpenAPI specification is the interface contract, and teams integrate through HTTP.

---

## Target Environment

- **Runtime**: Single Kubernetes cluster, distribution-agnostic (EKS, GKE, AKS, vanilla K8s).
- **Prerequisites**: CNPG operator installed, PostgreSQL instance for DAAP's own metadata store, object storage (S3-compatible) for backups (if any tier configures backups).
- **Local development**: k3d with CNPG operator.
- **DAAP deployment**: Dedicated namespace for the DAAP API server, reconciler, and platform database. Database CNPG clusters provisioned in team-specific or shared namespaces.

---

## V1 Features

### 1. Tier System

Platform teams define a fixed menu of tiers. Each tier is a named configuration that maps to infrastructure parameters invisible to product teams.

**A tier defines:**
- Compute profile (instances, CPU, memory)
- Storage (size, storage class)
- High availability (standby replicas, failover behavior)
- PostgreSQL version
- Backup strategy (frequency, retention, WAL archiving configuration -- or no backup at all for tiers that do not require it)
- Connection pooling (PgBouncer pool mode, max connections)
- Destruction strategy (one of freeze, archive, or hard -- assigned by the platform team, not chosen by the product team)

**Product team experience:**
- `GET /tiers` returns the available tiers with human-readable descriptions of guarantees.
- `POST /databases` accepts a `tier` field. The platform provisions according to the tier definition.
- Product teams never see instances, storage sizes, or replication topology.

**Platform team experience:**
- Tier definitions are managed via the API (CRUD on tiers, platform-team role required).
- Changing a tier definition affects future databases created with that tier. Existing databases retain their provisioned configuration unless explicitly migrated.

### 2. Per-Tier Resource Quotas

Platform teams set resource quotas **per tier**, limiting the total resources that can be consumed across all CNPG clusters provisioned under a given tier. Quotas are guardrails to prevent product teams from creating dozens of databases that saturate the Kubernetes cluster or induce excessive costs.

**Why per-tier, not per-team:** The platform cannot precisely measure the actual resource consumption of individual PostgreSQL databases at runtime. Quotas are based on the tier's defined resource envelope (what the tier allocates per database), not on observed usage.

**Enforcement:**
- On `POST /databases`, the platform calculates the resource cost of the requested tier and checks against the tier's remaining quota.
- If the request would exceed the quota, the API returns a 422 with a clear error indicating which resource limit was hit.
- Quotas are managed via the API (platform-team role required).

**Quota dimensions:**
- Total CPU (cores) allocatable under this tier
- Total memory (GiB) allocatable under this tier
- Total storage (GiB) allocatable under this tier
- Maximum number of databases under this tier

### 3. Backup and Restore

Backup strategy is tier-driven. Each tier defines its own backup configuration, which may range from no backup at all (e.g., a development tier in some organizations) to frequent backups with long retention (e.g., a production tier).

**Backup:**
- If the tier defines a backup strategy, the platform provisions CNPG `ScheduledBackup` resources and enables WAL archiving to object storage.
- Backup frequency and retention are defined per tier (e.g., production: hourly backup, 30-day retention; dev: no backup).
- Backup status is visible in the database record via the API.
- If a tier has no backup configured, the API clearly indicates this in the tier description and database record.

**Restore (PITR):**
- `POST /databases/{id}/restore` accepts a target timestamp.
- The platform bootstraps a new CNPG cluster from the nearest base backup + WAL replay to the target time.
- The restored database is a new database record with a reference to the source database.
- Restore is only available for databases whose tier includes a backup strategy.
- Product teams must have ownership of the source database to trigger a restore.

### 4. Destruction Strategies

The platform supports three destruction strategies. Each tier has exactly one destruction strategy assigned by the platform team at tier creation time. Product teams do not choose the strategy -- it is determined by the tier.

**Freeze:**
- Database becomes inaccessible (connections refused).
- CNPG Cluster and data remain on the Kubernetes cluster.
- Reversible: the database can be unfrozen via the API.
- Database status transitions to `frozen`.

**Archive:**
- CNPG Cluster is deleted from Kubernetes.
- Backups are retained according to an archive retention policy.
- Data recoverable via restore from backups.
- Database status transitions to `archived`.

**Hard Delete:**
- CNPG Cluster is deleted from Kubernetes.
- All backups are deleted from object storage (if any).
- Data is permanently and irrecoverably removed.
- Database record is soft-deleted (status `deleted`, `deleted_at` set).

**API:**
- `DELETE /databases/{id}` executes the destruction strategy defined by the database's tier. No strategy parameter.
- The API validates ownership before executing.

### 5. Extension Tracking

Read-only visibility of PostgreSQL extensions enabled per database.

- The reconciler periodically queries each database's `pg_extension` catalog and stores the result.
- `GET /databases/{id}` includes an `extensions` field listing enabled extensions with versions.
- The platform does not manage extensions -- product teams enable them via SQL.
- Extension data is used by the platform during upgrade planning: incompatible extensions may block PostgreSQL version upgrades.

### 6. Authentication and Authorization (API Keys + RBAC)

Team-based authentication with role-based access control.

**Authentication:**
- API keys issued per team. Each request includes the API key (via header).
- The platform resolves the key to a team identity and role.

**Roles:**

| Role | Capabilities |
|------|-------------|
| **platform-team** | Manage tiers, manage quotas, view all databases, scale databases, force destruction, manage API keys |
| **product-team** | Create databases (within quota), manage own databases (get, update, delete, restore), view tiers, view quota usage |

**Authorization rules:**
- Product team users can only access databases owned by their team (`owner_team` matches their team identity).
- Tier and quota management requires platform-team role.
- All actions are audit-logged with team identity, action, and timestamp.

### 7. Security Defaults

These security features are non-optional in v1:

- **TLS for all database connections**: CNPG auto-provisions TLS certificates per cluster. TLS-only connections enforced.
- **pgAudit enabled by default**: Every provisioned database has pgAudit configured as a shared_preload_library. Audit logs are available via standard PostgreSQL logging.
- **Request body limits**: All API endpoints limit request bodies to 1MB (already implemented).
- **No plaintext credentials**: API responses contain K8s Secret references, never credential values.
- **NetworkPolicies**: Default-deny NetworkPolicy provisioned per database namespace. Explicit allow rules for the owning team's application namespaces.
- **Audit logging**: All DAAP API actions logged with team identity, action type, resource ID, and timestamp.

### 8. OpenAPI Specification

The REST API ships with an OpenAPI v3 specification.

- Generated from or validated against the actual API implementation.
- Covers all endpoints, request/response schemas, error formats, and authentication.
- Published as a static file served by the API (`GET /openapi.json`).
- Enables teams to generate client libraries in any language.

---

## V1 Database Lifecycle

```
                    +-----------+
                    |           |
         POST /databases       |
                    |           |
                    v           |
              provisioning      |
                    |           |
           CNPG ready?         |
            /         \        |
           v           v       |
         ready       error ----+--- (reconciler retries)
           |
     (active use)
           |
     DELETE (tier strategy applied)
      /      |       \
     v       v        v
  frozen  archived  deleted
     |
  (unfreeze)
     |
     v
   ready
```

**Statuses:**
- `provisioning`: CNPG Cluster being created, not yet healthy.
- `ready`: CNPG Cluster healthy, connections available.
- `error`: CNPG Cluster in failed state (reconciler monitors for recovery).
- `frozen`: Database inaccessible, cluster intact, reversible.
- `archived`: Cluster deleted, backups retained, recoverable via restore.
- `deleted`: Soft-deleted, data may or may not exist depending on tier's strategy.

---

## V1 API Surface

### Database Endpoints (product-team + platform-team)
| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/databases` | Create a database (name, owner_team, purpose, tier) |
| `GET` | `/databases` | List databases (filtered, paginated, scoped to team) |
| `GET` | `/databases/{id}` | Get database details (including extensions, connection info if ready) |
| `PATCH` | `/databases/{id}` | Update mutable fields (owner_team, purpose) |
| `DELETE` | `/databases/{id}` | Destroy database using tier's destruction strategy |
| `POST` | `/databases/{id}/restore` | PITR restore to a target timestamp |
| `POST` | `/databases/{id}/unfreeze` | Unfreeze a frozen database |

### Tier Endpoints (platform-team only for mutations)
| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/tiers` | List available tiers (all roles) |
| `GET` | `/tiers/{id}` | Get tier details (all roles) |
| `POST` | `/tiers` | Create a tier (platform-team only) |
| `PATCH` | `/tiers/{id}` | Update a tier (platform-team only) |
| `DELETE` | `/tiers/{id}` | Delete a tier, only if no databases use it (platform-team only) |

### Quota Endpoints (platform-team only for mutations)
| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/tiers/{id}/quota` | Get quota for a tier (all roles) |
| `PUT` | `/tiers/{id}/quota` | Set/update quota for a tier (platform-team only) |

### Auth Endpoints (platform-team only)
| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api-keys` | Create an API key for a team |
| `GET` | `/api-keys` | List API keys |
| `DELETE` | `/api-keys/{id}` | Revoke an API key |

### System Endpoints (no auth)
| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/health` | Health check (already implemented) |
| `GET` | `/openapi.json` | OpenAPI v3 specification |

---

## What V1 Does NOT Include

These are explicitly deferred to post-v1:

| Feature | Reason for Deferral |
|---------|-------------------|
| **Observability / monitoring stack** | Dedicated feature research planned after v1. DAAP v1 does not provision PodMonitors, PrometheusRules, or Grafana dashboards. |
| **Credential rotation** | Non-trivial to perform without downtime. DAAP will provide guidance for credential rotation post-v1 after researching zero-downtime approaches. |
| **Database movement** | Requires logical replication or pg_dump/restore orchestration across CNPG clusters. Complex, not needed for initial value delivery. |
| **Database branching / forking** | Requires deep storage-layer integration not available in standard CNPG. |
| **Web UI / developer portal** | The API is the platform. A portal can be built on top of the OpenAPI spec post-v1. |
| **CLI tool** | Can be generated from the OpenAPI spec. Not needed when the API is the primary interface. |
| **GitOps integration (CRD-based)** | CRD-based provisioning alongside REST API is a future integration path for teams using ArgoCD/Flux. |
| **Multi-cluster / multi-region** | V1 targets a single cluster. Cross-cluster movement and multi-region HA are future capabilities. |
| **Automated scaling** | Scaling is platform-initiated in v1. Automated rules based on metrics depend on the observability stack (deferred). |
| **OIDC / OAuth2 authentication** | API keys are sufficient for v1. OIDC can be added as an authentication backend post-v1 without changing the authorization model. |

---

## Success Criteria for V1

V1 is complete when:

1. A platform team can define tiers (including PostgreSQL version, backup strategy, destruction strategy, compute/storage profile) and set per-tier quotas via the API.
2. A product team can create a database by choosing a tier, and the platform provisions a CNPG Cluster with the correct configuration, backup policy (if configured), pgAudit, TLS, and NetworkPolicy.
3. A product team can list, get, update, and destroy their databases using the tier's destruction strategy, within quota limits.
4. A product team can trigger PITR restore (for tiers with backup) via the API.
5. Frozen databases can be unfrozen and returned to active use.
6. The API enforces RBAC: product teams can only access their own databases, platform teams can manage all resources.
7. Extension tracking shows enabled extensions per database.
8. All API actions are audit-logged.
9. The OpenAPI v3 specification is published and accurate.
10. The platform runs on any Kubernetes distribution with CNPG operator and object storage (for backup-enabled tiers).
11. Local development works on k3d with `make cluster-up && make cnpg-install`.
