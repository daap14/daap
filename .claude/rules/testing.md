---
globs:
  - "tests/**"
  - "**/*.test.*"
  - "**/*.spec.*"
---

# Testing Conventions

## Structure
- Mirror the source tree under `tests/`
- Unit tests: `tests/unit/`
- Integration tests: `tests/integration/`
- Test fixtures: `tests/fixtures/`

## Naming
- Describe behavior, not implementation: "returns 404 when user not found" not "calls findById"
- Group with describe/context blocks by feature or endpoint
- Prefix integration tests that need external services

## Pattern: Arrange-Act-Assert
```
// Arrange — set up test data and dependencies
// Act — call the function/endpoint under test
// Assert — verify the result
```

## Mocking
- Mock external services (HTTP APIs, email, payment)
- Mock the database at integration boundary (use test DB for integration tests)
- Never mock the module under test
- Prefer dependency injection over monkey-patching

## Coverage
- Target: 80% line coverage minimum
- Focus coverage on business logic, not boilerplate
- Every bug fix must include a regression test

## Test Data
- Use factories or builders for test data
- Don't share mutable state between tests
- Clean up after integration tests (use transactions or truncation)

## Performance
- Unit tests should complete in < 5 seconds total
- Integration tests may be slower but should use connection pooling
- Parallelize tests where safe (no shared mutable state)
