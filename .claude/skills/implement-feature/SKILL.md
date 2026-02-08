---
description: Structured workflow for implementing a feature from spec to working code
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

Structured workflow for implementing a feature based on the architect's design and task specification.

## Prerequisites
- Architecture design exists for this feature
- Dev environment is set up and working
- Task is assigned and dependencies are resolved

## Steps

### 1. Understand the requirements
- Read the GitHub issue for this task
- Read the related architecture docs (ADR, API spec, schema)
- Read the iteration spec for context
- Identify acceptance criteria

### 2. Explore the existing codebase
- Use Glob/Grep to understand the current project structure
- Identify where the new code should live
- Understand existing patterns (how other features are implemented)
- Check for shared utilities or abstractions to reuse

### 3. Plan the implementation
Before writing code, outline:
- Which files to create/modify
- The public interface (function signatures, API contract)
- Data flow through the system
- Error cases to handle

### 4. Implement incrementally
Write code in this order:
1. **Data layer**: models, migrations, database queries
2. **Business logic**: service/use-case layer
3. **API layer**: routes, controllers, validation, serialization
4. **Wiring**: dependency injection, configuration

For each piece:
- Follow the project rules (`.claude/rules/`)
- Handle errors explicitly
- Add input validation at system boundaries
- Keep functions focused and testable

### 5. Verify the build
```bash
make build  # or equivalent
```
Fix any compilation/type errors before marking complete.

### 6. Self-check
Before completing, verify:
- [ ] Code follows project conventions (check `.claude/rules/`)
- [ ] No hardcoded secrets or magic numbers
- [ ] Error handling is complete
- [ ] Input validation on external inputs
- [ ] Functions are under 40 lines
- [ ] No `SELECT *` or unparameterized queries

## Output
- Working production code in `src/`
- Build passes with no errors
- Ready for qa agent to write tests
