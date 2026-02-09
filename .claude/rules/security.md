---
globs: ["**/*.go", "**/*.sql", "**/*.yaml"]
---

# Security Conventions

## Secrets and Credentials
- NEVER store passwords, tokens, or credentials as plaintext in the database
- Use Kubernetes Secret references (secret name + namespace) instead of storing credential values
- If a credential must be persisted, encrypt it at rest — never VARCHAR plaintext
- API responses must NEVER include raw passwords or tokens; return secret references instead
- Seed data and fixtures must not contain realistic-looking credentials; use obvious placeholders

## Request Body Limits
- All HTTP handlers that parse request bodies MUST use `http.MaxBytesReader` to limit input size
- Default limit: 1MB (`1 << 20`) unless the endpoint explicitly requires more
- Apply the limit BEFORE calling `json.Decode` or reading the body
