---
description: Code review workflow — read-only analysis for bugs, security, and convention adherence
user-invocable: true
disable-model-invocation: true
tools:
  - Read
  - Grep
  - Glob
  - Bash
allowed-tools:
  - Bash(git diff *)
  - Bash(git log *)
  - Bash(git show *)
---

# /review-code

Perform a thorough code review on recent changes. This is a **read-only** operation — no files are modified.

## Usage
```
/review-code                    # Review all uncommitted changes
/review-code --branch feature-x # Review all changes on a branch vs main
/review-code --pr 42            # Review a specific PR
```

## Steps

### 1. Identify changes to review
Based on the invocation:
- **Default**: `git diff HEAD` for uncommitted changes
- **Branch**: `git diff main...<branch>` for branch changes
- **PR**: `gh pr diff <number>` for PR changes

### 2. Categorize changed files
Group files by area:
- Production code (`src/`)
- Tests (`tests/`, `*.test.*`, `*.spec.*`)
- Infrastructure (`Dockerfile`, `docker-compose.*`, workflows)
- Documentation (`docs/`)

### 3. Review checklist
For each changed file, check:

**Correctness**
- [ ] Logic is correct and handles all cases
- [ ] Edge cases are handled (null, empty, overflow)
- [ ] Error handling is complete and appropriate
- [ ] No off-by-one errors or race conditions

**Security**
- [ ] No SQL/command injection vulnerabilities
- [ ] Input validation on all external data
- [ ] No hardcoded secrets or credentials
- [ ] Authentication/authorization checks in place
- [ ] No sensitive data in logs

**Convention adherence**
- [ ] Follows project rules (`.claude/rules/`)
- [ ] Consistent naming and code style
- [ ] Conventional commit messages
- [ ] API responses use the standard envelope

**Performance**
- [ ] No N+1 query patterns
- [ ] No unnecessary database queries in loops
- [ ] Appropriate use of indexes (check migrations)
- [ ] No blocking operations in request handlers

**Testability**
- [ ] New code has corresponding tests
- [ ] Tests cover edge cases and error paths
- [ ] No flaky test patterns (timeouts, shared state)

### 4. Produce review report
Output a structured review:

```markdown
## Code Review Summary

### Overall Assessment
[APPROVE | REQUEST CHANGES | NEEDS DISCUSSION]

### Critical Issues (must fix)
- [file:line] Description of the issue

### Suggestions (should fix)
- [file:line] Description of the suggestion

### Nits (optional)
- [file:line] Minor style or preference items

### Positive Highlights
- Things done well worth noting
```

## Output
- Structured review report (not written to file, output directly)
- Categorized findings: critical, suggestions, nits
- Overall assessment with recommendation
