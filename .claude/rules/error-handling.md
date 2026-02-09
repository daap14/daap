---
globs: ["**/*.go"]
---

# Error Handling Conventions

## Database Driver Errors
- Use structured error types from the database driver (e.g., `*pgconn.PgError`) to detect constraint violations
- NEVER use `strings.Contains(err.Error(), ...)` for error classification — error messages are driver-version-dependent and unstable
- Check PostgreSQL error codes (e.g., `23505` for unique violation) via `pgconn.PgError.Code`

## Sentinel Errors and Layering
- Define sentinel errors (e.g., `ErrDuplicateName`, `ErrNotFound`) in the repository layer
- Handlers check sentinel errors via `errors.Is()` — they must never inspect driver-specific error types
- Keep driver-specific logic confined to the repository implementation

## Boolean Logic in Status Checks
- When writing status-checking functions (like `isFailedPhase`), explicitly list BOTH the true and false cases in comments or code
- Verify that terminal states ("Failed", "Error") are handled correctly — never accidentally exclude them
- Default return value should be the safe choice (e.g., `return false` means "not failed" — make sure unknown phases aren't silently treated as healthy)
