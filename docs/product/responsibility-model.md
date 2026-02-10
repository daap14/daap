# DAAP Responsibility Model

## Purpose

This document defines the explicit responsibility boundary between **product teams** (database consumers) and the **platform team** (DAAP operators). It is not a guideline -- it is part of the product contract. Every capability in DAAP falls on one side of this boundary.

The manifesto states: *"This platform explicitly defines a responsibility model. It is not neutral. It is not negotiable. It is part of the product."*

---

## Principles

1. **Product teams are autonomous within their tier.** They self-serve database creation, access credentials, and manage their data. The platform does not gate or approve these actions.
2. **The platform owns everything beneath the database abstraction.** Infrastructure, availability, durability, backup execution, monitoring infrastructure, and movement between systems are platform responsibilities.
3. **Tiers are the contract between product teams and the platform.** A tier defines what the platform guarantees (backup frequency, retention, HA, resource limits) and what the product team receives. Product teams choose a tier; the platform fulfills it.
4. **Observability is shared but split by concern.** The platform monitors infrastructure health. Product teams are alerted on application-induced issues. Both have visibility into dashboards.
5. **Destructive operations are tier-determined.** The platform supports multiple destruction strategies (freeze, archive, hard delete). Each tier has exactly one assigned strategy -- product teams do not choose how their database is destroyed.

---

## Responsibility Matrix

### Product Team Responsibilities

| Area | Responsibility | Details |
|------|---------------|---------|
| **Database creation** | Self-service via API | Product teams create databases autonomously by providing name, owner_team, purpose, and tier. No approval workflow. |
| **Tier selection** | Choose from fixed menu | Product teams select a tier defined by the platform team. No custom profiles. No direct control over instances, storage, or replication. |
| **Schema management** | Full ownership | Migrations, DDL, index creation, extension enabling -- all product team responsibility. The platform does not run, review, or approve schema changes. |
| **Data correctness** | Full ownership | Application-level data integrity, referential consistency, and business logic constraints. |
| **Credential access** | On-demand retrieval | Product teams fetch credentials from the platform (via K8s Secret reference) when needed. No forced rotation. Credential rotation is a post-v1 capability. |
| **Lifecycle decisions** | Initiate destruction | Product teams decide when a database is no longer needed. The destruction strategy (freeze, archive, or hard delete) is determined by the tier, not chosen by the product team. |
| **Application monitoring** | Act on alerts | Product teams receive and respond to application-induced alerts (connection saturation, deadlocks, long-running queries, high latency). |

### Platform Team Responsibilities

| Area | Responsibility | Details |
|------|---------------|---------|
| **Tier definitions** | Create and maintain | Platform team defines tiers (infrastructure configuration, backup policy, HA level, resource quotas, allowed destruction strategies). Product teams cannot create or modify tiers. |
| **Quotas** | Enforce per-tier limits | Platform team sets and enforces resource quotas per tier (total CPU, memory, storage allocatable under each tier). Quotas are guardrails against cluster saturation and excessive costs, not precise usage tracking. |
| **Provisioning execution** | Fulfill tier contract | When a product team creates a database, the platform provisions the underlying CNPG Cluster, Pooler, networking, and monitoring resources according to the selected tier. |
| **Backup** | Tier-driven | The platform manages backup according to the tier's backup strategy. Tiers may define WAL archiving + scheduled base backups with configurable frequency and retention, or no backup at all (e.g., a development tier). The backup strategy is a platform decision embedded in the tier. |
| **Restore** | Execute PITR | The platform executes point-in-time restore by bootstrapping a new CNPG cluster from backups. Product teams can request a restore; the platform performs it. |
| **Credential generation** | Automatic | The platform generates credentials at provisioning time (via CNPG) and stores them as K8s Secrets. Credential rotation is a post-v1 capability (requires research into zero-downtime rotation). |
| **Scaling** | Platform-initiated | Platform team decides when to scale based on observability data (metrics, alerts, recommendations). Product teams cannot directly request scaling. The platform provides the metrics that inform scaling decisions. |
| **Infrastructure monitoring** | Detect and respond | Platform team is alerted on infrastructure issues: CPU, memory, disk usage, network health, cluster unhealthy state, backup failures. |
| **Movement** | Platform-only operation | Moving a database between underlying PostgreSQL systems is exclusively a platform operation. Product teams are notified but do not participate. Database identity, name, and owner remain unchanged. |
| **Extension tracking** | Operational visibility | The platform tracks which PostgreSQL extensions are enabled per database. Extensions can be blockers for version upgrades (e.g., incompatible extension versions). The platform does not manage extensions but must be aware of them. |
| **Destruction execution** | Execute tier's strategy | When a product team requests destruction, the platform executes the strategy assigned to the database's tier (freeze, archive, or hard delete). The product team does not choose the strategy. |

---

## Tiers: The Contract

Tiers are the mechanism through which the platform delivers its guarantees. Each tier defines:

| Dimension | What the tier specifies |
|-----------|------------------------|
| **Compute profile** | Number of instances, CPU/memory per instance (hidden from product teams) |
| **Storage** | Storage size and class (hidden from product teams) |
| **High availability** | Standby replicas, failover behavior |
| **PostgreSQL version** | The PostgreSQL major version for databases on this tier |
| **Backup strategy** | Backup frequency, retention period, WAL archiving configuration -- or no backup at all |
| **Destruction strategy** | Exactly one of freeze, archive, or hard delete -- assigned by the platform team |
| **Connection pooling** | PgBouncer configuration (pool mode, max connections) |
| **Resource quota** | Per-tier limits on total CPU, memory, storage, and database count allocatable under this tier |

Product teams see a fixed menu of tiers with human-readable descriptions of guarantees (e.g., "Production: PostgreSQL 16, HA with automated failover, hourly backups with 30-day retention"). They never see the infrastructure parameters.

Platform teams define tiers as configuration. Changing a tier definition can affect all databases using that tier -- this is intentional and is a platform operation.

---

## Destruction Strategies

The platform supports three destruction strategies. Each tier has exactly one strategy assigned by the platform team. Product teams do not choose the strategy -- when they delete a database, the tier's strategy is applied automatically.

### Freeze
- Database becomes inaccessible (connections refused).
- CNPG Cluster and data remain intact on the Kubernetes cluster.
- Reversible: the database can be unfrozen and returned to active use.
- Use case: temporary decommission, investigation, or accidental deletion recovery.

### Archive
- CNPG Cluster is deleted from Kubernetes (compute resources freed).
- Backups are retained according to a configurable archive retention policy.
- Data can be recovered by restoring from backups (new cluster bootstrapped).
- Not instantly reversible -- restore requires provisioning a new cluster.
- Use case: database no longer needed but data must be preserved for compliance or future reference.

### Hard Delete
- CNPG Cluster is deleted from Kubernetes.
- All backups are deleted (if any).
- Data is permanently and irrecoverably removed.
- Use case: ephemeral databases, test data, or data that must be purged (GDPR right to erasure).

Examples:
- A "development" tier might use the hard delete strategy (no need to preserve dev data).
- A "production" tier might use the archive strategy (data preserved in backups even after cluster removal).

---

## Monitoring and Alerting Split

### Product Team Alerts (Application-Induced)

These alerts indicate issues caused by or visible to the application layer:

| Alert | Trigger |
|-------|---------|
| Connection saturation | Active connections approaching pool limit |
| Deadlocks detected | PostgreSQL deadlock events |
| Long-running queries | Queries exceeding a threshold duration |
| High query latency | p95/p99 query latency above threshold |
| Replication lag (read impact) | Lag affecting read-replica consistency |
| Idle-in-transaction | Connections idle in transaction beyond threshold |

### Platform Team Alerts (Infrastructure)

These alerts indicate issues in the underlying infrastructure:

| Alert | Trigger |
|-------|---------|
| Cluster unhealthy | CNPG Cluster status not healthy for > 5 minutes |
| Cluster down | All instances unreachable |
| Disk usage high | Storage usage exceeding warning/critical thresholds |
| CPU/memory pressure | Sustained resource pressure on database pods |
| Backup failed | Scheduled backup did not complete |
| Backup stale | No successful backup within expected window |
| WAL archive lag | WAL archiving falling behind |
| Network issues | Pod-to-pod or service connectivity failures |

### Shared Visibility

Both teams have access to Grafana dashboards for their databases. The platform auto-provisions monitoring (PodMonitor + PrometheusRules) for every database -- this is non-optional.

---

## Credential Management

- Credentials are generated automatically at provisioning time by CNPG.
- Credentials are stored as Kubernetes Secrets, referenced by name in the DAAP API.
- The API never returns plaintext credentials -- only the Secret reference.
- Product teams retrieve credentials from Kubernetes directly using the Secret reference.
- There is no forced periodic rotation.
- Credential rotation is deferred to post-v1. It requires research into zero-downtime rotation approaches before the platform can provide this capability safely.

---

## Extension Tracking

- Product teams are free to enable any PostgreSQL extension available in the CNPG image.
- The platform does not manage, approve, or restrict extensions.
- The platform tracks which extensions are enabled per database for operational visibility.
- Extension information is used by the platform during upgrade planning: incompatible extensions may block PostgreSQL version upgrades. In such cases, the platform notifies the product team that an extension must be updated before the upgrade can proceed.

---

## Movement

- Database movement between underlying PostgreSQL systems is exclusively a platform operation.
- Product teams cannot request or initiate movement.
- The platform decides when movement is necessary (cluster upgrade, infrastructure refresh, capacity rebalancing).
- During movement, the database identity (name, ID, owner, tier) remains unchanged. Only the underlying CNPG Cluster changes.
- Product teams are notified before and after movement, but do not participate in execution.
- Movement must be performed without downtime whenever possible, as stated in the manifesto.

---

## What This Model Does Not Cover

The following are explicitly out of scope for this responsibility model and will be addressed in future documents:

- **Authentication and authorization**: How API access is controlled (which teams can access which databases). Will be defined in the v1 vision or a dedicated security document.
- **Network policies**: How cross-tenant database traffic is restricted. Platform responsibility, but implementation details are deferred.
- **Compliance-specific requirements**: SOC 2, GDPR, HIPAA-specific controls. The model supports compliance (audit logging, backup retention, hard delete) but does not prescribe specific compliance frameworks.
- **Multi-cluster / multi-region**: Movement within a single Kubernetes cluster is covered. Cross-cluster or cross-region movement is a future capability.
