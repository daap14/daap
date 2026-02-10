# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [v0.4.0] — 2026-02-10

### Features
- **Authentication**: API key authentication via `X-API-Key` header with bcrypt hashing and prefix-based lookup
- **Teams CRUD**: POST/GET/DELETE `/teams` with name uniqueness and platform/product role enforcement
- **Users CRUD**: POST/GET/DELETE `/users` with team association, API key generation, and soft revocation
- **Superuser bootstrap**: Auto-created on first startup when no users exist, API key logged once
- **Authorization middleware**: `RequireSuperuser()` for admin endpoints, `RequireRole()` for business endpoints
- **Ownership scoping**: Product users see only their own team's databases (404 for non-owned, not 403)
- **Owner team FK enforcement**: `databases.owner_team_id` is now a UUID FK referencing `teams(id)` — creating a database with a nonexistent team returns 404

### Bug Fixes
- **N+1 query eliminated**: User List endpoint now JOINs team columns in a single query instead of calling GetByID per user
- **TrimSpace consistency**: Handler inputs are trimmed before validation and storage (team names, user names, database fields)
- **CHECK constraints**: Database-level enforcement of status enum, superuser-team invariant, and revocation ordering

### Architecture Decisions
- ADR 006: Authentication strategy — bcrypt API keys, prefix lookup, superuser bootstrap

### Other Changes
- New packages: `internal/team/`, `internal/auth/`
- New rules: `auth.md`, `data-model.md` (FK integrity, CHECK constraints, N+1 prevention)
- OpenAPI spec updated for all auth, team, and user endpoints
- README updated with new API endpoints
- Migrations 004 (teams), 005 (users), 006 (CHECK constraints), 007 (owner_team FK)

## [v0.3.0] — 2026-02-10

### Features
- **OpenAPI 3.1 specification**: Hand-maintained YAML spec at `api/openapi.yaml`, embedded and served as JSON at `GET /openapi.json`
- **Route coverage test**: Automated test verifying spec paths match Chi router routes
- **Spec validation in CI**: `vacuum lint` via `make lint-openapi` as a parallel CI job

### Architecture Decisions
- ADR 005: OpenAPI strategy — hand-maintained YAML (not code-generated)

### Other Changes
- CI fix: upgraded `golangci-lint-action` from v6 to v7 for golangci-lint v2 compatibility
- Agent team redesign: dev + reviewer roles with PR-per-task workflow
- New rule: `git-workflow.md` (branch-per-task, PR workflow, review cycle)
- Product discovery: research, v1 vision, roadmap

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

[v0.4.0]: https://github.com/daap14/daap/compare/v0.3.0...v0.4.0
[v0.3.0]: https://github.com/daap14/daap/compare/v0.2.0...v0.3.0
[v0.2.0]: https://github.com/daap14/daap/compare/v0.1.0...v0.2.0
[v0.1.0]: https://github.com/daap14/daap/releases/tag/v0.1.0
