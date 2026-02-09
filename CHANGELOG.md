# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [v0.2.0] — 2026-02-09

### Features
- **Database CRUD API**: POST, GET (list + single), PATCH, DELETE endpoints for managed databases
- **CNPG Cluster provisioning**: Template builder generates unstructured Cluster resources applied to Kubernetes
- **PgBouncer Pooler provisioning**: Companion pooler created alongside each database cluster
- **K8s resource manager**: Apply/delete clusters and poolers, read cluster status and secrets via dynamic client
- **Background reconciler**: Polls CNPG cluster health, transitions databases between provisioning/ready/error states
- **Connection details**: Ready databases expose host, port, and Kubernetes Secret reference (no plaintext credentials)
- **Input validation**: Name format, required fields, structured field-level error responses
- **Pagination and filtering**: List databases with `page`, `limit`, `owner_team`, `status`, `name` filters
- **Soft deletes**: DELETE marks records with `deleted_at`, allowing name reuse
- **Duplicate detection**: Unique constraint on active database names with conflict response

### Infrastructure
- PostgreSQL added to docker-compose and CI (service container with health check)
- Database migrations via golang-migrate (001 create table, 002 remove credentials, 003 drop dbname)
- Seed data script (`make seed`) for sample database records
- CNPG operator auto-install in cluster-setup.sh
- New Makefile targets: `migrate`, `seed`, `cluster-up`, `cluster-down`, `cnpg-install`

### Architecture Decisions
- ADR 002: Platform database — PostgreSQL with pgx driver
- ADR 003: Create handler rollback strategy
- ADR 004: Internalize infrastructure parameters (instances, storageSize, dbname removed from API)

### Other Changes
- New rules: error-handling, reconciler, security, validation conventions
- API design rule: infrastructure isolation principle
- Updated database-schema.md documentation

## [v0.1.0] — 2026-02-09

### Features
- Go API server with Chi router
- Health endpoint (`GET /health`) with Kubernetes and database connectivity checks
- Consistent response envelope (`{data, error, meta}`)
- Request ID middleware
- Configuration via environment variables
- ADR 001: Tech stack (Go, Chi, client-go, CNPG types)

### Infrastructure
- Dockerfile with multi-stage build
- docker-compose for local development
- GitHub Actions CI (lint, build, test)
- Makefile with standard targets
- Kind cluster setup script

[v0.2.0]: https://github.com/daap14/daap/compare/v0.1.0...v0.2.0
[v0.1.0]: https://github.com/daap14/daap/releases/tag/v0.1.0
