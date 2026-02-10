---
globs:
  - "migrations/**"
  - "internal/database/**"
  - "internal/*/repository.go"
  - "internal/*/postgres_repository.go"
  - "internal/api/handler/**"
---

# Data Model Conventions

## Foreign Key Integrity
- Every cross-table reference MUST use a UUID FK column with `REFERENCES ... ON DELETE {RESTRICT|CASCADE|SET NULL}`
- Never use VARCHAR for cross-table references — store UUID, JOIN for display
- New tables: verify all relationship columns have FK constraints

## CHECK Constraints
- Columns with known value sets (status, role) MUST have `CHECK (col IN (...))`
- Business invariants MUST be enforced as CHECK constraints, not just application logic
- Temporal invariants (e.g., `revoked_at >= created_at`) MUST have CHECK constraints

## N+1 Query Prevention
- List endpoints MUST NOT call single-record queries in a loop
- Use JOINs to fetch related data in the List query itself
- Add transient fields to models populated via JOIN — never loop GetByID
