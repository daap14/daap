---
globs:
  - "api/openapi.yaml"
  - "internal/api/handler/openapi.go"
  - "internal/api/router.go"
---

# OpenAPI Conventions

## Spec Location
- Source of truth: `api/openapi.yaml` (embedded via `api/spec.go`)
- Served at runtime as JSON at `GET /openapi.json`

## Keeping the Spec in Sync
- Every PR that adds or changes API endpoints must update `api/openapi.yaml`
- Property names in the spec must use camelCase matching Go struct json tags
- The route coverage test (`tests/unit/api/openapi_coverage_test.go`) catches path-level drift between the spec and the Chi router
- Run `make test` before pushing to verify spec/router alignment

## Schema Conventions
- Response envelopes: `{data, error, meta}` — matches `response.Envelope`
- Validation errors use `[]FieldError` (array of `{field, message}`), not a map
- Error codes must match the constants used in handler code (e.g., `DUPLICATE_NAME`, not `CONFLICT`)
- Nullable fields use OpenAPI 3.1 type arrays: `type: [string, "null"]`
- Connection fields (`host`, `port`, `secretName`) are flat optional fields with `omitempty`, not a nested object
