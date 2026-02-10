# 005. OpenAPI Tooling

## Status
Accepted

## Context
DAAP's API has 6 endpoints (1 health + 5 database CRUD) with no machine-readable specification. An 869-line draft YAML spec exists at `docs/architecture/health.openapi.yaml`, but it has accumulated discrepancies with the actual implementation:

- **Casing**: The draft uses snake_case (`owner_team`, `cluster_name`, `created_at`) while the Go handlers serialize as camelCase (`ownerTeam`, `clusterName`, `createdAt`).
- **Removed fields**: ADR 004 removed `instances` and `storageSize` from the API, but the draft still includes them in `CreateDatabaseRequest`. `ConnectionDetails` (with `dbname`, `username`, `password`) was replaced by a `secretName` K8s Secret reference, per the security and reconciler conventions.
- **Missing health fields**: The health endpoint now returns a `database` connectivity status alongside `kubernetes`, but the draft spec only documents `kubernetes`.
- **Wrong error codes**: The draft uses `CONFLICT` for duplicate database names, but the code returns `DUPLICATE_NAME`.

We need to decide how to produce and maintain a correct OpenAPI 3.1 spec going forward.

### Options Considered

| Option | Pros | Cons |
|--------|------|------|
| **Hand-maintained YAML** | Full control over spec content, supports OpenAPI 3.1, works with any Go handler structure, small API surface (~6 now, ~20 at v1) | Must manually keep spec in sync with code |
| `swaggo/swag` (annotation-based) | Auto-generates from Go comments | Generates Swagger 2.0, not OpenAPI 3.x; handler types are unexported; annotation drift is as bad as manual drift |
| `oapi-codegen` (spec-first) | Generates Go server stubs from spec | Requires rewriting all handlers to match generated interfaces; high refactor cost for 6 endpoints |

## Decision
Hand-maintain an OpenAPI 3.1 YAML spec at `api/openapi.yaml`, with automated safeguards against drift:

1. **Spec location**: `api/openapi.yaml` — the canonical, version-controlled API contract.
2. **Serving**: Embed the YAML via Go `embed`, convert to JSON with `sigs.k8s.io/yaml` (already an indirect dependency, promoted to direct), and serve at `GET /openapi.json`.
3. **CI validation**: Lint the spec in CI using [`vacuum`](https://github.com/daveshanley/vacuum), a fast OpenAPI linter. New Makefile target: `make lint-openapi`.
4. **Drift mitigation**: A route coverage test compares every path+method in the spec against the registered Chi routes, failing if any route is missing from the spec or vice versa. This catches structural drift (missing/extra endpoints) at test time.

### Why not `swag`
- `swag` generates Swagger 2.0, not OpenAPI 3.x — the ecosystem is moving to 3.1 (e.g., `vacuum`, Redocly, Stoplight).
- DAAP's handler response types (`createDatabaseRequest`, `databaseResponse`) are unexported. `swag` requires exported types with doc comments.
- The API surface is small (~6 endpoints now, ~20 at v1.0). The overhead of maintaining annotations is comparable to maintaining a YAML file, but with less control over the output.

### Dependency note
`sigs.k8s.io/yaml` v1.6.0 is already in `go.sum` as an indirect dependency of client-go. This ADR promotes it to a direct dependency for YAML-to-JSON conversion.

## Consequences

### Positive
- Full OpenAPI 3.1 support, including `oneOf`, nullable types, and JSON Schema 2020-12 features.
- Spec is human-readable and reviewable in PRs — changes to the API contract are visible in diffs.
- Route coverage test catches structural drift (missing/extra endpoints) automatically.
- `vacuum` catches spec quality issues (missing descriptions, invalid schemas) in CI.
- `GET /openapi.json` enables tooling consumers (Postman, Redocly, client generators) without hosting a separate doc site.

### Negative
- Field-level drift (e.g., adding a JSON field to a Go struct without updating the spec) is not caught by the route coverage test. Catching this would require response-body validation tests, which are out of scope for v0.3.
- Manual spec maintenance adds a step to every API change. Mitigated by the `.claude/rules/openapi.md` rule requiring spec updates alongside code changes.

### Neutral
- The draft spec at `docs/architecture/health.openapi.yaml` will be superseded by `api/openapi.yaml`. The draft can be deleted after v0.3 is merged.
