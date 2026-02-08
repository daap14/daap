---
description: Structured workflow for writing unit and integration tests
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

Structured workflow for writing comprehensive tests for implemented features.

## Prerequisites
- Feature code exists in `src/`
- Dev environment is set up with test runner configured

## Steps

### 1. Analyze the code under test
- Read the implementation code thoroughly
- Identify public interfaces, business logic, and edge cases
- Read the related API spec and acceptance criteria
- Understand dependencies (what needs mocking)

### 2. Plan test coverage
Create a test plan covering:
- **Happy paths**: normal, expected usage
- **Edge cases**: boundary values, empty inputs, max lengths
- **Error cases**: invalid input, missing data, service failures
- **Security cases**: injection attempts, unauthorized access

### 3. Write unit tests
For business logic and utilities:
```
tests/unit/<module>/
```

Follow Arrange-Act-Assert pattern:
```
// Arrange — set up test data and mocks
// Act — call the function under test
// Assert — verify the result and side effects
```

Guidelines:
- One assertion concept per test (may need multiple assert statements)
- Descriptive test names: "should return 404 when user not found"
- Mock external services, not internal modules
- Use factories/builders for test data
- Don't test implementation details (private methods, internal state)

### 4. Write integration tests
For API endpoints and database operations:
```
tests/integration/<feature>/
```

Integration test guidelines:
- Test the full request/response cycle
- Use a test database (not mocks)
- Set up and tear down test data per test
- Test authentication and authorization
- Verify response shape matches the API spec envelope

### 5. Write regression tests
If this feature fixes a bug:
- Write a test that reproduces the original bug
- Verify the test fails without the fix and passes with it

### 6. Run and verify
```bash
make test          # Run all tests
make test-coverage # Check coverage (target: 80%)
```

- All tests must pass
- Coverage should meet or exceed 80% for the new code
- No flaky tests (run twice to verify)

### 7. Report
Summarize to lead:
- Number of tests added (unit + integration)
- Coverage percentage for new code
- Any gaps or areas that couldn't be tested
- Recommendations for code changes that would improve testability

## Output
- Test files in `tests/` and/or `*.test.*` / `*.spec.*`
- All tests passing
- Coverage report
