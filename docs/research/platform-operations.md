# Platform Operations & SRE Patterns for Managed Databases

## Executive Summary

- **Kubernetes PostgreSQL operators have matured significantly**: CloudNativePG, Crunchy Data PGO, and Zalando are the three leading operators, with CloudNativePG emerging as the community favorite due to its Kubernetes-native architecture (no Patroni dependency), CNCF Sandbox status, and rapid adoption growth.
- **Backup/restore has converged on object-store + WAL archiving**: All major operators support continuous WAL archiving to S3/GCS/Azure with PITR capability. The choice of backup tool (Barman vs pgBackRest vs WAL-G) is the key differentiator, along with Volume Snapshot support for faster recovery.
- **Observability follows the Prometheus + Grafana + Alertmanager pattern universally**: Every operator integrates with the Prometheus ecosystem. The emerging trend is OpenTelemetry adoption for unified logs/metrics/traces, reducing vendor lock-in.
- **Multi-tenancy on Kubernetes requires defense-in-depth**: Namespace isolation alone is insufficient. Production multi-tenancy demands layered RBAC, NetworkPolicies, ResourceQuotas, Pod Security Standards, and potentially virtual clusters (vcluster) for hard isolation.
- **Percona Everest (now OpenEverest) is the closest open-source reference to a full DBaaS platform**: It provides a unified UI/API over multiple database operators, demonstrating the pattern DAAP should follow — abstracting operator complexity behind a product-level interface.

## Detailed Findings

### 1. CloudNativePG (CNPG)

**What it is:** A Kubernetes-native PostgreSQL operator originally developed by EnterpriseDB, now a CNCF Sandbox project. Unlike other operators, CNPG does not depend on Patroni or StatefulSets — it uses standalone Pods controlled entirely by the operator, with an Instance Manager embedded in each Pod.

**Key features relevant to DAAP:**
- **Cluster CRD**: Single `Cluster` resource defines instances, storage, backup, monitoring, replication, and managed services. The `instances` field controls cluster size (1 = standalone, >1 = primary + replicas with automated failover).
- **Declarative everything**: Roles, databases, services, poolers, backups, and scheduled backups are all managed via CRDs (`Backup`, `ScheduledBackup`, `Pooler`, `Database`, `Publication`, `Subscription`).
- **Plugin architecture (CNPG-I)**: Since v1.26, backup/recovery is moving to a pluggable interface. The Barman Cloud Plugin is the officially supported backup plugin, replacing the deprecated in-core Barman integration.

**Backup/Restore:**
- Object store backups (S3, GCS, Azure) via Barman Cloud Plugin — full and incremental
- Volume Snapshot backups via Kubernetes CSI
- Continuous WAL archiving with parallel WAL archiving/restore for high-write environments
- PITR by bootstrapping a new cluster from a base backup + WAL replay to a target time, LSN, or timeline
- Recovery is never in-place — always bootstraps a new cluster (safe, idempotent pattern)
- Safety checks prevent accidental overwrite of existing data in shared storage buckets
- Configurable retention policies based on recovery windows

**Observability:**
- Built-in Prometheus metrics exporter on port 9187 (HTTP/HTTPS)
- Predefined metrics in `cnpg-default-monitoring` ConfigMap
- Custom metrics via SQL queries in ConfigMap/Secret resources
- PodMonitor support (automatic via `.spec.monitoring.enablePodMonitor` or manual)
- PrometheusRule samples for alerts: `CNPGClusterNotHealthy`, `CNPGClusterDown`, replication lag warnings
- Metrics caching (30s default, configurable via `metricsQueriesTTL`)
- Default metrics: `lag`, `in_recovery`, `is_wal_receiver_up`, `streaming_replicas`
- Grafana dashboards available in dedicated repository

**Security:**
- Non-privileged container execution (no privileged mode required)
- Automatic TLS certificate provisioning and management (self-signed CA per cluster)
- mTLS support for client certificate authentication
- Least-privilege RBAC: operator service account (`cnpg-manager`) has ClusterRoleBinding; instance service accounts have read-only access scoped to their own cluster resources
- Network policy support for fine-grained pod-to-pod traffic control
- Operator communicates on TCP port 8000 (status) and 5432 (PostgreSQL)
- Signed container images with SBOM and provenance attestations
- Weekly automated CVE patch builds
- Fencing capability to isolate instances

**Multi-tenancy:**
- Operator runs in dedicated namespace (`cnpg-system`), clusters in separate namespaces
- Namespace-level RBAC isolation between clusters
- NetworkPolicy required to allow cross-namespace operator-to-cluster communication
- No built-in multi-tenancy abstraction — tenancy is managed at the Kubernetes namespace level

**Strengths:**
- Truly Kubernetes-native (no external HA dependencies)
- CNCF Sandbox project with rapidly growing community (4,300+ GitHub stars)
- Clean declarative API with comprehensive CRD coverage
- Plugin architecture future-proofs backup/recovery integrations
- Fastest-growing community among PG operators

**Weaknesses:**
- No built-in web UI for management
- Recovery always requires bootstrapping a new cluster (no in-place restore)
- Major version upgrades use pg_dump import method (slower for large databases)
- Multi-tenancy requires manual namespace/RBAC/NetworkPolicy setup

**What DAAP can learn:**
- CNPG's CRD design is the gold standard for declarative PostgreSQL on Kubernetes. DAAP already uses CNPG — the platform layer should abstract CNPG Cluster CRDs behind its own product-level API.
- The CNPG-I plugin architecture is worth tracking — it will enable DAAP to swap backup providers without changing platform code.
- Recovery-as-new-cluster pattern aligns with DAAP's manifesto: databases are long-lived assets, and recovery creates a new "physical" cluster while preserving the "logical" database identity.

**Sources:**
- https://cloudnative-pg.io/docs/1.28/
- https://cloudnative-pg.io/docs/devel/backup/
- https://cloudnative-pg.io/documentation/current/monitoring/
- https://cloudnative-pg.io/documentation/preview/security/
- https://github.com/cloudnative-pg/cloudnative-pg

---

### 2. Crunchy Data PGO (Postgres Operator)

**What it is:** The oldest PostgreSQL operator for Kubernetes (since 2017), developed by Crunchy Data. Uses Patroni for HA and pgBackRest for backup/recovery. Fully declarative in v5 with comprehensive day-2 operations.

**Key features relevant to DAAP:**
- Declarative PostgresCluster CRD with GitOps-friendly design
- Self-healing: continuously monitors and recreates missing components (poolers, replicas)
- Can watch all namespaces or be scoped to individual namespaces
- Includes PgBouncer for connection pooling, pgAdmin for management

**Backup/Restore:**
- pgBackRest integration (default, always enabled) — supports full, incremental, differential backups
- Multi-repository support: up to 4 simultaneous backup locations (local + cloud mixed)
- Scheduled backups via Kubernetes CronJobs
- PITR with delta restore for efficient recovery
- Backup encryption with user-provided passwords
- Cloud storage: S3, GCS, Azure Blob Storage
- Efficient replica provisioning from backups (delta restore)
- Failed primaries auto-heal using delta restore

**Observability:**
- pgMonitor integration: Prometheus + Grafana + Alertmanager stack
- Exporter sidecar automatically added to all Postgres pods with TLS support
- Pre-built Grafana dashboards from pgMonitor
- Container-level metrics via pgnodemx (CPU, memory, disk from Kubernetes fields)
- pgBackRest monitoring dashboard
- Pre-configured alerts: PGIsUp, PGIdleTxn, PGQueryTime, PGConnPerc, PGDiskSize, PGReplicationByteLag
- OpenTelemetry support for unified logs/metrics (recent addition)

**Security:**
- Built-in TLS/SSL with automated certificate management
- Supports cert-manager integration for PKI flexibility
- Enterprise compliance certifications available (commercial offering)
- Non-opinionated about PKI — loads TLS key pairs and CA from Kubernetes Secrets

**Multi-tenancy:**
- Namespace-scoped or cluster-wide operation modes
- Can be isolated to individual namespaces for multi-tenant deployments

**Strengths:**
- Most mature and battle-tested operator (8+ years in production)
- pgBackRest is extremely powerful: multi-repo, delta restore, parallel backup, encryption
- Best reliability in testing (minimal downtime during operations)
- Pre-configured monitoring with actionable alerts
- Strong enterprise backing with commercial support path

**Weaknesses:**
- Container images under Crunchy Data Developer Program (not fully open-source images)
- Major version upgrades are a multi-stage process (PGUpgrade CRD + annotations)
- Patroni dependency adds operational complexity
- Heavier footprint than CloudNativePG

**What DAAP can learn:**
- pgBackRest's multi-repository approach is excellent for disaster recovery: DAAP should ensure backup data exists in at least 2 locations.
- The pre-configured alert set (PGReplicationByteLag, PGDiskSize, PGConnPerc) is a good baseline for DAAP's monitoring requirements.
- Self-healing behavior (automatic recreation of missing components) is a pattern DAAP's reconciler should adopt for all managed resources, not just CNPG clusters.

**Sources:**
- https://github.com/CrunchyData/postgres-operator
- https://access.crunchydata.com/documentation/postgres-operator/v5/
- https://access.crunchydata.com/documentation/postgres-operator/latest/architecture/backups
- https://access.crunchydata.com/documentation/postgres-operator/latest/architecture/monitoring
- https://www.crunchydata.com/blog/opentelemetry-observability-in-crunchy-postgres-for-kubernetes

---

### 3. Zalando Postgres Operator

**What it is:** An operator developed at Zalando, used in production for 5+ years. Built on Patroni for HA, with Spilo as the PostgreSQL container image. Battle-tested in a large-scale e-commerce environment.

**Key features relevant to DAAP:**
- `postgresql` CRD for cluster definition
- Patroni-powered HA using Kubernetes ConfigMaps/Endpoints as DCS (no external consensus system needed)
- Rich PostgreSQL extension support (pg_cron, pgvector, pg_audit, pg_repack, pgq, etc.)
- Deployment via Helm, Kustomize, or manual YAML

**Backup/Restore:**
- Logical backups to S3/GCS (scheduled)
- WAL-G continuous archiving to S3/GCS (deprecated WAL-E removed in v1.15)
- Backups persist after cluster deletion (secrets and backups retained)
- Less sophisticated than CNPG/PGO backup capabilities

**Observability:**
- Prometheus integration via sidecar agents (e.g., coroot-pg-agent)
- kube-state-metrics integration
- Requires additional configuration for modern monitoring tooling
- No pre-built dashboards or alerts (DIY approach)

**Security:**
- Standard Kubernetes Secret management for credentials
- TLS support through Spilo configuration
- Less built-in security automation compared to CNPG/PGO

**Multi-tenancy:**
- Namespace-based isolation
- Cost-efficient multi-tenancy through resource sharing
- AWS-optimized (live volume resize for EBS, gp2-to-gp3 migration)

**Strengths:**
- Simple and pragmatic design
- Battle-tested at Zalando scale
- MIT license (most permissive)
- Strong AWS integration
- Rich extension ecosystem in Spilo image

**Weaknesses:**
- Community activity declining
- Limited official support
- Backup capabilities less mature than CNPG/PGO
- Monitoring requires more DIY effort
- Less Kubernetes-native than CloudNativePG

**What DAAP can learn:**
- Zalando's approach of hiding Patroni complexity behind CRDs validates DAAP's manifesto: infrastructure is an implementation detail. However, the declining community is a warning about operator sustainability.
- The extension ecosystem (150+ in StackGres, many in Zalando) highlights that extension management will be a user expectation. DAAP should track which extensions are enabled per database.

**Sources:**
- https://github.com/zalando/postgres-operator
- https://opensource.zalando.com/postgres-operator/docs/quickstart.html
- https://github.com/zalando/postgres-operator/releases

---

### 4. StackGres

**What it is:** A "full-stack" PostgreSQL distribution for Kubernetes by OnGres. Goes beyond just the operator — it bundles connection pooling, HA (Patroni), monitoring, backups, and logging into a single deployment unit. Licensed under AGPLv3.

**Key features relevant to DAAP:**
- Multiple management interfaces: CRDs, CLI, and web UI (all interchangeable)
- `SGCluster` and `SGShardedCluster` CRDs
- 150+ PostgreSQL extensions supported out of the box
- Day 2 operations: HA, backups, minor/major upgrades automated
- Automatic Prometheus integration with built-in Grafana dashboards
- Scale to 0 instances and HPA/KEDA scaling support
- GitOps-ready high-level CRDs
- External DCS support (etcd, ZooKeeper) for multi-Kubernetes-cluster HA

**Backup/Restore:**
- `SGObjectStorage` CRD for backup storage configuration
- Scheduled backups with cron expressions and retention policies
- S3, GCS, Digital Ocean, and other object store support
- Backup configuration at both cluster and sharded cluster level

**Sharding (Citus integration):**
- `SGShardedCluster` CRD with `type: citus`
- Coordinator node (HA with replicas) + shard clusters (each with HA)
- Horizontal scaling by adding shards
- Resilience: failure of a single machine does not bring down the database

**Strengths:**
- Most comprehensive "batteries-included" operator
- Web UI for management (unique among open-source operators)
- Sharding support via Citus (unique feature)
- Largest extension catalog
- Active, veteran PostgreSQL development team

**Weaknesses:**
- AGPLv3 license (more restrictive than Apache 2.0)
- Smaller community than CNPG/PGO
- Heavier deployment footprint (full stack bundled)
- Company-driven project

**What DAAP can learn:**
- StackGres' multi-interface approach (CRD + CLI + Web UI) aligns with DAAP's goal of abstracting infrastructure. The web UI pattern is what DAAP will eventually need for self-service.
- Sharding via Citus is a future consideration if DAAP needs to support horizontally-scaled databases.
- The `SGObjectStorage` CRD pattern (separating storage config from cluster config) is a good design for reusability.

**Sources:**
- https://stackgres.io/
- https://github.com/ongres/stackgres/
- https://stackgres.io/doc/1.5/administration/sharded-cluster-creation/

---

### 5. Percona Operator for PostgreSQL + Percona Everest (OpenEverest)

**What it is:** Two related products from Percona. The Percona Operator for PostgreSQL (based on Crunchy Data PGO v4.7) provides individual database cluster management. Percona Everest (now OpenEverest) is a DBaaS platform layer on top of Percona operators for MySQL, MongoDB, and PostgreSQL.

**Percona Operator key features:**
- Apache 2.0 licensed, fully open source with no feature tiers
- Automated and safe operator upgrades (unique among operators)
- PostgreSQL 18 support
- OpenShift certified
- Native PMM (Percona Monitoring and Management) integration
- Scheduled backups, PITR, built-in HA

**Percona Everest / OpenEverest key features:**
- Unified UI and REST API for MySQL, MongoDB, and PostgreSQL
- Self-service database provisioning (1-command install via `everestctl`)
- Multi-cloud and on-premises deployment
- Automated Day 2 operations: updates, backup, recovery, monitoring, patching
- Custom resource allocation with no restrictions per node
- PMM integration for observability
- Red Hat OpenShift support (since April 2025)
- EKS, GKE, and vanilla Kubernetes support

**Strengths:**
- Everest is the closest open-source analog to what DAAP is building
- Multi-database support (PostgreSQL + MySQL + MongoDB)
- No vendor lock-in, self-hosted
- Claims 50%+ cost reduction vs public DBaaS
- Moving toward CNCF incubation
- Community-led governance (OpenEverest)

**Weaknesses:**
- PostgreSQL operator community is small
- Based on older Crunchy Data PGO v4.7 (not the latest PGO v5)
- Less Kubernetes-native than CloudNativePG
- Everest is still maturing as a platform

**What DAAP can learn:**
- **Everest's architecture is the closest reference architecture for DAAP**: a product-level API and UI over Kubernetes operators. Study its REST API design, provisioning flows, and backup management patterns.
- The separation of "operator" (cluster lifecycle) from "platform" (self-service, multi-tenancy, monitoring aggregation) is exactly the architecture DAAP should follow.
- However, DAAP's manifesto is more opinionated than Everest — DAAP enforces responsibility models and ownership, while Everest is a generic provisioning tool.

**Sources:**
- https://www.percona.com/software/percona-everest
- https://docs.percona.com/everest/features.html
- https://github.com/openeverest/openeverest
- https://github.com/percona/percona-postgresql-operator

---

### 6. KubeDB by AppsCode

**What it is:** A commercial multi-database operator for Kubernetes supporting PostgreSQL, MySQL, MongoDB, Elasticsearch, Redis, Memcache, and more. In development since 2017.

**Key features relevant to DAAP:**
- Multi-database provisioning from a single operator
- Well-documented with thorough CRD configuration examples
- Strong commercial support

**Strengths:**
- All-in-one multi-database solution
- Well-documented, battle-tested since 2017
- Comprehensive feature set (backups, connection pooling, snapshots, dormant databases)
- Strong commercial support

**Weaknesses:**
- **Commercial license required** (not truly open source)
- Many critical features (backups, connection pooling, snapshots) are enterprise-only
- Vendor lock-in risk
- License tied to cluster-ID

**What DAAP can learn:**
- The "dormant database" concept (pausing a database without deleting it) aligns with DAAP's lifecycle management goals (archival/deprecation).
- However, KubeDB's commercial model is antithetical to DAAP's open-source manifesto.

**Sources:**
- https://kubedb.com/
- https://www.simplyblock.io/blog/choosing-a-kubernetes-postgres-operator/

---

## Backup & Restore Patterns

### Backup Strategy Best Practices

| Pattern | Description | RPO | RTO |
|---------|-------------|-----|-----|
| **Full + WAL archiving** | Periodic base backup + continuous WAL to object store | Seconds (WAL lag) | Minutes to hours (depends on DB size) |
| **Incremental/Differential** | Only changed blocks since last full backup | Seconds | Faster than full restore |
| **Volume Snapshots** | CSI-based snapshot of PV + WAL archive | Seconds | Fastest (snapshot restore) |
| **Delayed Replica** | Streaming replica with configured delay (e.g., 1 hour) | Delay window | Very fast (promote replica) |
| **Multi-repository** | Backups to 2+ locations simultaneously | Seconds | Depends on location |

### The 3-2-1 Rule
Maintain 3 copies of data, on 2 different media types, with 1 copy off-site (geographically separated). In Kubernetes context:
1. Live data on PersistentVolumes
2. Backups in primary object store (same region)
3. Backups replicated to secondary object store (different region)

### PITR Architecture
All major operators implement PITR by combining base backups with continuous WAL archiving:
1. WAL files are archived continuously to object storage
2. Base backups are taken periodically (daily recommended)
3. Recovery replays WALs from the nearest base backup to the target time
4. Recovery always bootstraps a new cluster (CNPG pattern) or restores to existing (PGO pattern)

### Backup Tool Comparison

| Tool | Used By | Full | Incremental | Differential | Parallel | Encryption | Multi-repo |
|------|---------|------|-------------|--------------|----------|------------|------------|
| **Barman Cloud** | CloudNativePG | Yes | Yes | No | Yes | Yes (via object store) | No |
| **pgBackRest** | Crunchy PGO | Yes | Yes | Yes | Yes | Yes (native) | Yes (up to 4) |
| **WAL-G** | Zalando | Yes | Yes (delta) | No | Yes | Yes | No |

### Recommendations for DAAP
- **Use CNPG's Barman Cloud Plugin** as the primary backup mechanism (already the foundation)
- **Implement scheduled backups** via `ScheduledBackup` CRD with configurable retention
- **Enforce WAL archiving** for all databases (non-optional — required for PITR)
- **Store backups in object storage** (S3/GCS) with bucket-per-team or prefix-per-database isolation
- **Define platform-level RPO/RTO targets** (e.g., RPO < 5 minutes, RTO < 30 minutes for standard tier)
- **Regularly test restores** automatically as part of platform health checks
- **Consider Volume Snapshots** for faster RTO on large databases (future iteration)

**Sources:**
- https://dev.to/dean_dautovich/13-postgresql-backup-best-practices-for-developers-and-dbas-3oi5
- https://stormatics.tech/blogs/understanding-disaster-recovery-in-postgresql
- https://www.percona.com/blog/postgresql-backup-strategy-enterprise-grade-environment/
- https://www.enterprisedb.com/blog/postgresql-database-backup-recovery-what-works-wal-pitr

---

## Observability Patterns

### The Standard Stack: Prometheus + Grafana + Alertmanager

Every major Kubernetes PostgreSQL operator integrates with this stack. The pattern is:
1. **Metrics exporter sidecar** (or built-in) exposes PostgreSQL metrics on a `/metrics` endpoint
2. **PodMonitor/ServiceMonitor** tells Prometheus to scrape the metrics
3. **Grafana dashboards** visualize metrics (pre-built dashboards available for all operators)
4. **PrometheusRules** define alerting conditions
5. **Alertmanager** routes alerts to notification channels

### Key PostgreSQL Metrics to Monitor

| Category | Metrics | Why It Matters |
|----------|---------|----------------|
| **Connections** | Active connections, idle in transaction, connection pool utilization | Prevent connection exhaustion |
| **Replication** | Replication lag (bytes and time), streaming replica count | Data durability and HA readiness |
| **Performance** | Query latency, sequential vs index scans, transactions per second | Identify slow queries and missing indexes |
| **Storage** | Disk usage, WAL volume, table/index bloat | Prevent disk exhaustion |
| **Vacuum** | Dead rows, autovacuum activity, last vacuum time | Prevent transaction ID wraparound |
| **Locks** | Lock waits, deadlocks | Identify contention issues |
| **Backup** | Last backup time, backup duration, backup size, WAL archive lag | Ensure backup SLA compliance |

### Recommended Alerts for a DBaaS Platform

| Alert | Severity | Condition |
|-------|----------|-----------|
| `ClusterNotHealthy` | Critical | Cluster status != Healthy for > 5m |
| `ClusterDown` | Critical | All instances unreachable |
| `ReplicationLagHigh` | Warning | Replication lag > 100MB or > 30s |
| `DiskUsageHigh` | Warning/Critical | Disk usage > 75% (warn) / > 90% (critical) |
| `ConnectionPoolExhausted` | Warning | Connection usage > 80% |
| `BackupFailed` | Critical | Scheduled backup did not complete |
| `BackupStale` | Warning | No successful backup in > 25 hours |
| `WALArchiveLag` | Warning | WAL archive lag > configured threshold |
| `HighQueryLatency` | Warning | p99 query latency > threshold |
| `VacuumNotRunning` | Warning | No autovacuum for > 24 hours on active table |

### Emerging Trend: OpenTelemetry

Crunchy Data's PGO now supports OpenTelemetry for unified logs and metrics. This is the direction the industry is moving:
- Vendor-neutral collection of logs, metrics, and traces
- Single standard for all observability signals
- Compatible with Prometheus, Grafana, Datadog, Elastic, and other backends
- Avoids vendor lock-in at the collection layer

### eBPF-Based Observability

Tools like Coroot use eBPF to capture PostgreSQL wire protocol traffic for deep observability:
- Captures every query with latency heatmaps and error rates
- No database instrumentation or extensions required
- Works across Kubernetes, RDS, Aurora, and Cloud SQL
- Useful for platform-level observability across all managed databases

### Recommendations for DAAP
- **Start with Prometheus + Grafana + Alertmanager** (CNPG has native support)
- **Enable PodMonitor for every database** (non-optional platform behavior)
- **Ship a default set of PrometheusRules** with the platform (based on the alert table above)
- **Expose database health via the DAAP API** (aggregate CNPG cluster status + custom metrics)
- **Track OpenTelemetry adoption** for future migration from pure Prometheus
- **Consider per-database Grafana dashboards** auto-provisioned on database creation

**Sources:**
- https://cloudnative-pg.io/documentation/current/monitoring/
- https://www.crunchydata.com/blog/opentelemetry-observability-in-crunchy-postgres-for-kubernetes
- https://www.crunchydata.com/blog/setup-postgresql-monitoring-in-kubernetes
- https://coroot.com/postgres/
- https://www.datadoghq.com/blog/postgresql-monitoring/

---

## Security & Compliance Patterns

### Encryption

| Layer | Pattern | Tools |
|-------|---------|-------|
| **In transit** | TLS/SSL for all client connections | CNPG auto-provisioned certs, cert-manager |
| **In transit (internal)** | mTLS between operator and instances, replication encryption | CNPG built-in, separate CA per cluster |
| **At rest (storage)** | Volume encryption via storage class or CSI driver | Cloud provider KMS, LUKS |
| **At rest (database)** | Transparent Data Encryption (TDE) | EDB Postgres Advanced (commercial), pgcrypto for column-level |
| **At rest (backups)** | Encrypted backups in object store | pgBackRest native encryption, object store server-side encryption |

### Certificate Management
- **CNPG approach**: Self-provisions random TLS certificates and CA per cluster by default (convention over configuration). Can be overridden with user-provided certificates.
- **cert-manager integration**: All major operators support cert-manager for automated certificate issuance and renewal. Recommended for production.
- **Certificate rotation**: Use Reloader or similar tools to watch certificate Secrets and trigger pod restarts on renewal.

### RBAC Best Practices for a DBaaS Platform
1. **Operator service account**: Cluster-wide role with minimum required permissions
2. **Instance service accounts**: Namespace-scoped, read-only access to own cluster resources only
3. **User access**: Never grant direct access to CNPG CRDs — mediate through DAAP API
4. **Admin access**: Separate admin and user roles in the DAAP API; admin can manage all databases, users can only manage their own
5. **Audit**: Log all DAAP API actions (who created/modified/deleted which database)

### Network Security
- **NetworkPolicies**: Restrict pod-to-pod communication. Allow operator namespace to cluster namespace on ports 8000 (status) and 5432 (PostgreSQL). Block cross-tenant database traffic.
- **Ingress**: Never expose PostgreSQL ports directly to the internet. Use internal services or VPN/bastion.
- **Service mesh**: Consider Istio or Linkerd for mTLS between all services (not just database connections).

### Compliance Patterns

| Requirement | PostgreSQL Feature | Platform Responsibility |
|-------------|-------------------|------------------------|
| **Audit logging** | pgAudit extension | Enable by default on all databases, centralize logs |
| **Access control** | ROLE-based permissions | Enforce via platform API, never share superuser |
| **Data encryption** | TLS + storage encryption | Enforce TLS-only connections, encrypted storage classes |
| **Backup retention** | Configurable retention policies | Enforce minimum retention per compliance tier |
| **Data residency** | Namespace + node affinity | Control which regions/zones databases are deployed to |
| **Change tracking** | WAL + audit triggers | Maintain immutable audit trail |
| **Secret management** | K8s Secrets, external vault | Never store plaintext credentials in database tables |

### Recommendations for DAAP
- **Enforce TLS for all database connections** (CNPG does this by default — keep it non-optional)
- **Integrate pgAudit** as a default extension on all databases
- **Never expose infrastructure credentials via the API** (current rule — keep enforcing)
- **Use K8s Secrets for credential references** (current pattern — continue)
- **Add audit logging to the DAAP API layer** (who did what, when)
- **Implement NetworkPolicies per database namespace** to prevent cross-tenant access
- **Plan for cert-manager integration** to replace self-signed certificates in production

**Sources:**
- https://cloudnative-pg.io/documentation/preview/security/
- https://www.crunchydata.com/blog/set-up-tls-for-postgresql-in-kubernetes
- https://www.enterprisedb.com/postgresql-compliance-gdpr-soc-2-data-privacy-security
- https://www.bytebase.com/blog/postgres-audit-logging/
- https://www.liquibase.com/resources/guides/soc-2-compliance-for-database-security-trust-services-criteria-best-practices

---

## Multi-Tenancy Patterns

### Isolation Models

| Model | Description | Isolation Level | Cost | Complexity |
|-------|-------------|----------------|------|------------|
| **Namespace per tenant** | Each tenant gets a K8s namespace with RBAC + NetworkPolicies | Medium (soft) | Low | Low |
| **Namespace + ResourceQuotas** | Above + CPU/memory/storage limits per tenant | Medium | Low | Medium |
| **Virtual cluster per tenant** | vcluster provides full virtual control plane per tenant | High (hard) | Medium | Medium |
| **Physical cluster per tenant** | Dedicated K8s cluster per tenant | Maximum | High | High |

### Defense-in-Depth Layers

1. **Namespace isolation**: Logical boundary — necessary but not sufficient
2. **RBAC**: Prevent cross-tenant API access — Roles scoped to tenant namespace
3. **NetworkPolicies**: Block cross-namespace pod-to-pod traffic (requires Calico or Cilium)
4. **ResourceQuotas**: Prevent resource monopolization (CPU, memory, storage, PVC count)
5. **LimitRanges**: Set per-pod resource boundaries within tenant namespaces
6. **Pod Security Standards**: Enforce security baselines (no privileged containers, no host networking)
7. **Node isolation**: Taints/tolerations to schedule tenants on dedicated nodes (for high-security tenants)
8. **Container sandboxing**: gVisor or Firecracker for strongest isolation (e.g., EKS Fargate)

### DAAP-Specific Multi-Tenancy Design

Given the DAAP manifesto's emphasis on ownership and explicit responsibility:

- **Database = Kubernetes Namespace mapping**: Each database (or team's database set) should map to a namespace
- **Owner enforcement**: Every namespace must have owner labels/annotations matching the DAAP database owner
- **Platform-level quotas**: DAAP API enforces per-team or per-owner quotas (number of databases, total storage, total CPU)
- **Network isolation by default**: Default-deny NetworkPolicy in every database namespace; explicit allow only for owner's application namespaces
- **Credential isolation**: Each database's credentials in a dedicated K8s Secret, accessible only within its namespace

### Recommendations for DAAP
- **Start with namespace-per-database** with RBAC + NetworkPolicies (sufficient for v1)
- **Add ResourceQuotas per team/owner** to prevent noisy-neighbor problems
- **Implement default-deny NetworkPolicies** from the start — harder to add later
- **Track vcluster** for hard multi-tenancy requirements in future iterations
- **Never rely on namespace isolation alone** — always layer with RBAC + NetworkPolicies

**Sources:**
- https://atmosly.com/blog/kubernetes-multi-tenancy-complete-implementation-guide-2025
- https://www.vcluster.com/blog/kubernetes-multi-tenancy-and-rbac-implementation-and-security-considerations
- https://aws.github.io/aws-eks-best-practices/security/docs/multitenancy/
- https://docs.google.com/kubernetes-engine/docs/concepts/multitenancy-overview

---

## Feature Comparison Matrix

| Feature | CloudNativePG | Crunchy PGO | Zalando | StackGres | Percona | KubeDB |
|---------|:------------:|:-----------:|:-------:|:---------:|:-------:|:------:|
| **License** | Apache 2.0 | Apache 2.0* | MIT | AGPLv3 | Apache 2.0 | Commercial |
| **CNCF Status** | Sandbox | No | No | No | No | No |
| **HA Mechanism** | Built-in (Instance Manager) | Patroni | Patroni | Patroni | Patroni (via PGO) | Custom |
| **StatefulSets** | No (standalone Pods) | Yes | Yes | Yes | Yes | Yes |
| **Full Backups** | Yes (Barman) | Yes (pgBackRest) | Yes (WAL-G) | Yes | Yes | Enterprise only |
| **Incremental Backups** | Yes | Yes | Yes (delta) | Yes | Yes | Enterprise only |
| **PITR** | Yes | Yes | Yes | Yes | Yes | Enterprise only |
| **Volume Snapshots** | Yes (CSI) | Yes | No | No | No | No |
| **Multi-repo Backups** | No | Yes (up to 4) | No | No | No | No |
| **Backup Encryption** | Via object store | Native (pgBackRest) | Via object store | Via object store | Via object store | N/A |
| **Prometheus Metrics** | Built-in exporter | pgMonitor sidecar | DIY sidecar | Built-in | PMM integration | Custom |
| **Pre-built Dashboards** | Yes (Grafana repo) | Yes (pgMonitor) | No | Yes (built-in) | Yes (PMM) | No |
| **Pre-built Alerts** | Yes (sample rules) | Yes (pgMonitor) | No | Yes | Yes (PMM) | No |
| **OpenTelemetry** | Not yet | Yes | No | No | No | No |
| **Auto TLS** | Yes (self-signed CA) | Yes | Manual config | Yes | Yes (cert-manager) | Manual |
| **mTLS Client Auth** | Yes | Yes | No | No | Yes | No |
| **Connection Pooling** | PgBouncer (Pooler CRD) | PgBouncer | PgBouncer | Built-in | PgBouncer | Enterprise only |
| **Web UI** | No | No (pgAdmin optional) | No | Yes | Yes (Everest) | Yes |
| **REST API** | No | No | No | No | Yes (Everest) | No |
| **Sharding** | No | No | No | Yes (Citus) | No | No |
| **Multi-DB Support** | PostgreSQL only | PostgreSQL only | PostgreSQL only | PostgreSQL only | PG + MySQL + Mongo | 8+ databases |
| **Major Version Upgrades** | pg_dump import | PGUpgrade CRD | Supported | Automated | Supported | Supported |
| **Scale to Zero** | No | No | No | Yes | No | No |
| **Declarative Roles** | Yes (.spec.managed.roles) | Yes | Yes | Yes | Yes | Yes |
| **Custom Metrics** | Yes (SQL in ConfigMap) | Yes (pgMonitor) | DIY | Yes | Yes (PMM) | No |
| **Community Activity** | Very high, growing | High, company-backed | Declining | Medium | Growing | Company-backed |

\* Crunchy PGO source is Apache 2.0 but container images are under Crunchy Data Developer Program

---

## Key Insights

1. **CloudNativePG is the right foundation for DAAP.** Its Kubernetes-native architecture (no Patroni, no StatefulSets), CNCF backing, plugin architecture, and rapid community growth make it the strongest long-term bet. DAAP already uses CNPG — this is validated.

2. **DAAP's value is the product layer above the operator.** Every operator manages PostgreSQL clusters. None of them manage "databases as products" with ownership, lifecycle, responsibility models, and self-service. DAAP fills this gap — similar to what Percona Everest does for provisioning, but with DAAP's opinionated product philosophy.

3. **Backup must be non-optional and platform-managed.** Users should not configure backups — the platform should enforce WAL archiving + scheduled base backups for every database, with PITR as a default capability. Backup failure should be a critical platform alert.

4. **Observability should be "on by default."** Enable PodMonitor, deploy PrometheusRules, and auto-provision Grafana dashboards for every database. Users should see database health in the DAAP API without configuring monitoring tools.

5. **Multi-tenancy needs early NetworkPolicy investment.** Adding default-deny NetworkPolicies later is painful. Start with namespace-per-database + default-deny + explicit allow rules from v0.3 or v0.4.

6. **pgAudit should be a platform default.** For any organization that might need SOC 2 or GDPR compliance, audit logging is essential. Enabling pgAudit by default (with centralized log aggregation) makes compliance a platform feature, not a per-team effort.

7. **Credential management is already correct.** DAAP's current pattern of storing K8s Secret references (not plaintext credentials) is aligned with industry best practices. Continue and extend with cert-manager integration for TLS certificate lifecycle.

8. **Self-healing reconciliation is table stakes.** Crunchy PGO's self-healing (automatic recreation of missing components) and DAAP's existing reconciler are aligned. Extend the reconciler to cover all managed resources: backups, monitoring, network policies, and secrets — not just CNPG clusters.

9. **OpenTelemetry is the future of database observability.** While Prometheus is the current standard, plan for OpenTelemetry adoption. Crunchy Data is already there; CNPG will follow. Design DAAP's observability layer to be backend-agnostic.

10. **Extension management will become a user expectation.** StackGres (150+ extensions) and Zalando (rich Spilo image) show that users expect to enable/disable PostgreSQL extensions. DAAP should track enabled extensions per database and enforce an allow-list.

---

## Recommendations

### Adopt

| Pattern | Rationale | Priority |
|---------|-----------|----------|
| **CNPG ScheduledBackup CRD** for all databases | Non-optional backup; aligns with manifesto's durability guarantee | High (v0.3-v0.4) |
| **PodMonitor + PrometheusRules** for all databases | Observability as platform feature, not user configuration | High (v0.3-v0.4) |
| **Default-deny NetworkPolicies** per database namespace | Multi-tenancy security foundation; harder to add retroactively | High (v0.4) |
| **pgAudit as default extension** | Compliance readiness without per-team effort | Medium (v0.4-v0.5) |
| **cert-manager integration** | Production-grade TLS certificate lifecycle | Medium (v0.5) |
| **Platform-level health API** | Aggregate CNPG status + metrics into DAAP API responses | Medium (v0.4) |

### Adapt

| Pattern | Adaptation | Priority |
|---------|-----------|----------|
| **Percona Everest's REST API model** | DAAP already has an API; study Everest's provisioning/backup/restore endpoints for feature parity | Medium |
| **Crunchy PGO's alert set** | Use as baseline for DAAP's PrometheusRules, adapted to DAAP's SLA tiers | Medium |
| **pgBackRest multi-repo pattern** | CNPG doesn't support multi-repo yet; implement at platform level by configuring 2 backup destinations | Low (future) |
| **StackGres' management interfaces** | Web UI for DAAP self-service (CLI and UI interchangeable with API) | Low (future) |

### Avoid

| Pattern | Reason |
|---------|--------|
| **Patroni-based operators** for new development | CNPG's native HA is simpler and more Kubernetes-native; Patroni adds operational complexity |
| **KubeDB's commercial model** | Contradicts DAAP's open-source manifesto |
| **Exposing infrastructure parameters in the API** | Already prohibited by DAAP rules; validated by all operator designs |
| **In-place cluster recovery** | CNPG's "recovery bootstraps a new cluster" pattern is safer and aligns with immutable infrastructure |
| **Relying on namespace isolation alone** | Industry consensus: namespace isolation is necessary but not sufficient for multi-tenancy |
