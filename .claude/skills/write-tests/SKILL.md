---
description: Structured workflow for adding tests to existing code
user-invocable: false
tools:
  - Read
  - Write
  - Edit
  - Bash
  - Glob
  - Grep
---

# Write Tests

Structured workflow for adding tests to existing code that lacks coverage.

## Prerequisites
- Feature code exists in `internal/` or `cmd/`
- Dev environment is set up with `go test` working

## Steps

### 1. Analyze the code under test
- Read the implementation code thoroughly
- Identify public interfaces, business logic, and edge cases
- Read the related API spec and acceptance criteria
- Understand dependencies (what needs mocking via interfaces)

### 2. Plan test coverage
Create a test plan covering:
- **Happy paths**: normal, expected usage
- **Edge cases**: boundary values, empty inputs, max lengths, zero values
- **Error cases**: invalid input, missing data, service failures
- **Security cases**: injection attempts, unauthorized access

### 3. Write unit tests
Colocated `*_test.go` files in the same package:

```go
// internal/handler/database_test.go
func TestCreateDatabase_ValidInput(t *testing.T) {
    // Arrange — set up test data and mocks
    // Act — call the function under test
    // Assert — verify the result
}
```

Guidelines:
- Use table-driven tests for multiple input/output combinations
- Use `t.Run()` for subtests with descriptive names
- Mock external services via interfaces, not internal modules
- Use helper functions for test data (e.g., `newTestDatabase(t, opts...)`)
- Don't test implementation details (private functions, internal state)

### 4. Write integration tests
In `tests/integration/`:

- Test the full request/response cycle
- Use a real test database (not mocks)
- Set up and tear down test data per test with `t.Cleanup()`
- Test authentication and authorization paths
- Verify response shape matches the API envelope `{data, error, meta}`

### 5. Write regression tests
If adding tests for a bug fix:
- Write a test that reproduces the original bug
- Verify the test fails without the fix and passes with it

### 6. Run and verify
```bash
make test                    # Run all tests
go test -cover ./internal/...  # Check coverage
```

- All tests must pass
- Coverage should meet or exceed 80% for new code
- Run twice to verify no flaky tests

### 7. Report
Summarize:
- Number of tests added (unit + integration)
- Coverage percentage for new code
- Any gaps or areas that couldn't be tested
- Recommendations for code changes that would improve testability

## Output
- Unit test files: `*_test.go` colocated with source
- Integration test files: `tests/integration/`
- All tests passing
- Coverage report
