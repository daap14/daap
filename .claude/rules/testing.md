---
globs:
  - "**/*_test.go"
  - "tests/**"
---

# Testing Conventions (Go)

## Structure
- Unit tests: colocated `*_test.go` files in the same package as the code under test
- Integration tests: `tests/integration/` — test the full request/response cycle
- Test helpers and fixtures: `tests/fixtures/`, `tests/helpers/`

## Naming
- Test functions: `TestFunctionName_Scenario` (e.g., `TestCreateDatabase_DuplicateName`)
- Subtests: `t.Run("returns 404 when database not found", func(t *testing.T) {...})`
- Describe behavior, not implementation: "returns 404 when not found" not "calls FindByID"
- Integration test files: prefix with the feature area (e.g., `database_lifecycle_test.go`)

## Pattern: Arrange-Act-Assert
```go
func TestCreateDatabase_ValidInput(t *testing.T) {
    // Arrange — set up test data and dependencies
    repo := NewMockRepository()
    handler := NewHandler(repo)

    // Act — call the function/endpoint under test
    resp := handler.Create(req)

    // Assert — verify the result
    assert.Equal(t, http.StatusCreated, resp.Code)
}
```

## Table-Driven Tests
Prefer table-driven tests for functions with multiple input/output combinations:
```go
tests := []struct {
    name     string
    input    CreateRequest
    wantCode int
    wantErr  string
}{
    {"valid input", validReq, 201, ""},
    {"missing name", noNameReq, 400, "name is required"},
}
for _, tt := range tests {
    t.Run(tt.name, func(t *testing.T) { ... })
}
```

## Mocking
- Use interfaces for dependencies — mock via interface implementation, not monkey-patching
- Mock external services (K8s client, HTTP APIs)
- Use a real test database for integration tests (not mocked)
- Never mock the module under test

## Coverage
- Target: 80% line coverage minimum
- Focus coverage on business logic and handlers, not boilerplate
- Every bug fix must include a regression test

## Test Data
- Use helper functions to build test data (e.g., `newTestDatabase(t, opts...)`)
- Don't share mutable state between tests
- Integration tests: clean up with `t.Cleanup()` or use transactions

## Performance
- Unit tests should complete in < 5 seconds total
- Use `t.Parallel()` for independent tests
- Integration tests: use connection pooling, clean up per test
