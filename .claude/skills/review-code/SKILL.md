---
description: Review a pull request on GitHub — add comments, approve or request changes
user-invocable: true
disable-model-invocation: true
tools:
  - Read
  - Write
  - Edit
  - Grep
  - Glob
  - Bash
allowed-tools:
  - Bash(git diff *)
  - Bash(git log *)
  - Bash(git show *)
  - Bash(gh pr *)
  - Bash(gh api *)
---

# /review-code

Review a pull request on GitHub. Add line-level comments, approve or request changes, and capture reusable patterns as rules.

## Usage
```
/review-code <pr-number>
/review-code --branch feat/tier-system
```

## Steps

### 1. Fetch the PR context
```bash
gh pr view <number> --json title,body,files,baseRefName,headRefName
gh pr diff <number>
```

### 2. Read the full files
Don't review just the diff — read the full files for context. Use Glob/Grep to understand how the changed code fits into the larger codebase.

### 3. Read the related issue/task
Check the PR description for linked issues. Read the acceptance criteria.

### 4. Review checklist

**Correctness**
- Logic handles all cases, including edge cases
- Error handling is complete (no swallowed errors, uses structured errors)
- No off-by-one errors or race conditions
- Database queries use parameterized inputs
- Go zero values are handled correctly (see `.claude/rules/validation.md`)

**Security**
- No SQL/command injection vulnerabilities
- Input validation on all external data
- No hardcoded secrets or credentials
- No sensitive data in logs or API responses
- Request body size limits applied

**Convention Adherence**
- Follows all `.claude/rules/` conventions
- Consistent naming and code style
- Conventional commit messages
- API responses use the standard `{data, error, meta}` envelope

**Testing**
- New code has colocated unit tests (`*_test.go`)
- Integration tests cover API endpoints
- Tests cover edge cases and error paths
- Table-driven tests used where appropriate
- No flaky test patterns

**Documentation & Rules**
- `README.md` updated if API endpoints added/changed
- `.claude/rules/` updated if new conventions established
- If a review comment applies broadly, suggest creating a rule

**Performance**
- No N+1 query patterns
- No unnecessary database queries in loops
- Appropriate indexes in migrations

### 5. Add comments on GitHub

For specific line issues:
```bash
gh api repos/{owner}/{repo}/pulls/{number}/comments \
  -f body="..." -f path="internal/handler/database.go" \
  -f line=42 -f side="RIGHT" -f commit_id="$(gh pr view <number> --json headRefOid -q .headRefOid)"
```

For general feedback:
```bash
gh pr review <number> --comment --body "## Review Summary
### Critical Issues (must fix)
- ...
### Suggestions (should fix)
- ...
### Nits (optional)
- ...
### Positive Highlights
- ..."
```

### 6. Submit verdict
```bash
# If everything looks good:
gh pr review <number> --approve --body "LGTM — ..."

# If issues found:
gh pr review <number> --request-changes --body "..."
```

### 7. Capture patterns as rules
If the review reveals a pattern that should be a rule (something that would apply to future PRs, not just this one):
- Create or update the relevant `.claude/rules/<topic>.md` file
- Commit the rule change directly to `master` or include in a separate PR

## Output
- GitHub PR comments (line-level and general)
- Review verdict (approve or request changes)
- Optional: new/updated `.claude/rules/` files
