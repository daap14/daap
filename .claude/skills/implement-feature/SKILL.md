---
description: Structured workflow for implementing a feature from spec to PR
user-invocable: false
tools:
  - Read
  - Write
  - Edit
  - Bash
  - Glob
  - Grep
---

# Implement Feature

Structured workflow for implementing a feature: code + tests + docs + PR.

## Prerequisites
- Architecture design exists for this feature (ADR, API spec)
- Dev environment is set up and working
- Task is assigned and dependencies are resolved

## Steps

### 1. Understand the requirements
- Read the GitHub issue for this task
- Read the related architecture docs (ADR, API spec, schema)
- Read the iteration spec for context
- Identify acceptance criteria

### 2. Create a feature branch
```bash
git checkout master && git pull
git checkout -b feat/<task-description>
```

### 3. Explore the existing codebase
- Use Glob/Grep to understand the current project structure
- Identify where the new code should live
- Understand existing patterns (how other features are implemented)
- Check for shared utilities or abstractions to reuse
- Read all relevant `.claude/rules/` files

### 4. Implement incrementally
Write code in this order:
1. **Data layer**: models, migrations, database queries (`internal/model/`, `internal/repository/`, `migrations/`)
2. **Business logic**: service/use-case layer (`internal/service/` if applicable)
3. **API layer**: routes, handlers, validation, serialization (`internal/handler/`)
4. **Wiring**: dependency injection, configuration (`cmd/`)
5. **Unit tests**: colocated `*_test.go` files alongside each layer
6. **Integration tests**: `tests/integration/` for API endpoint tests

For each piece:
- Follow the project rules (`.claude/rules/`)
- Handle errors explicitly
- Add input validation at system boundaries
- Keep functions focused and testable

### 5. Verify build and tests
```bash
make build   # Compile
make test    # Run all tests
make lint    # Run linter
```
Fix any errors before proceeding.

### 6. Update documentation
- If the PR adds/changes API endpoints → update `README.md`
- If the PR establishes a new convention → create/update `.claude/rules/<topic>.md`

### 7. Self-check
Before creating the PR, verify:
- [ ] Code follows project conventions (`.claude/rules/`)
- [ ] Unit tests cover happy path, edge cases, and error cases
- [ ] Integration tests cover API endpoints
- [ ] No hardcoded secrets or magic numbers
- [ ] Error handling is complete
- [ ] Input validation on external inputs
- [ ] Functions are under 40 lines
- [ ] No `SELECT *` or unparameterized queries
- [ ] README updated if API changed
- [ ] `.claude/rules/` updated if new convention established

### 8. Create the PR
```bash
git add <files>
git commit -m "feat(scope): description"
git push -u origin feat/<task-description>
gh pr create --title "feat(scope): description" --body "..."
```

### 9. Wait for CI to pass
```bash
gh pr checks <number> --watch
```
- If CI fails: read the failure logs (`gh pr checks <number>`), fix the issues, push updates
- Repeat until CI is green
- Do NOT request review until CI passes

### 10. Request review
Once CI is green, notify the reviewer via SendMessage that the PR is ready:
- Include the PR number and a brief summary of what was implemented
- The reviewer will add comments on GitHub

### 11. Apply review suggestions
When the reviewer requests changes:
1. Read the PR comments: `gh pr view <number> --comments`
2. Apply the suggestions in code
3. Push updates
4. Notify the reviewer via SendMessage that updates are pushed and request re-review
5. Repeat until the reviewer approves

## Output
- Feature branch with production code + tests
- CI passing
- PR reviewed and approved on GitHub
