---
description: Convert an iteration spec into a detailed technical task plan with dependencies
user-invocable: true
disable-model-invocation: true
tools:
  - Read
  - Write
  - Glob
  - Grep
  - Bash
allowed-tools:
  - Bash(gh *)
  - Bash(git *)
---

# /plan-iteration

Convert an iteration spec into a detailed technical plan with ordered tasks and dependencies.

## Usage
```
/plan-iteration docs/iterations/v0.1.md
```

## Steps

### 1. Read the iteration spec
- Read the specified iteration spec file
- Read existing architecture docs (`docs/architecture/decisions/`)
- Read the current codebase structure (if code exists)
- Understand what already exists vs. what needs to be built

### 2. Break features into technical tasks
For each feature in the iteration:
- Identify the implementation tasks (design, code, test, infra)
- Assign each task to a teammate: `architect`, `implementer`, `qa`, `local-devops`
- Identify dependencies between tasks (what must be done first)
- Estimate complexity: S (< 1 hour), M (1-3 hours), L (3-8 hours)

### 3. Create the technical plan
Write `docs/iterations/vX.Y-plan.md`:

```markdown
# Technical Plan — vX.Y

## Task Order

### Phase 1: Design
| # | Task | Owner | Complexity | Depends On |
|---|------|-------|-----------|-----------|
| 1 | Design API endpoints for Feature A | architect | M | — |
| 2 | Design database schema | architect | S | — |

### Phase 2: Infrastructure
| # | Task | Owner | Complexity | Depends On |
|---|------|-------|-----------|-----------|
| 3 | Set up dev environment | local-devops | L | 1, 2 |

### Phase 3: Implementation
| # | Task | Owner | Complexity | Depends On |
|---|------|-------|-----------|-----------|
| 4 | Implement Feature A endpoint | implementer | M | 1, 3 |
| 5 | Write tests for Feature A | qa | M | 4 |

### Phase 4: Polish
| # | Task | Owner | Complexity | Depends On |
|---|------|-------|-----------|-----------|
| 6 | Code review | qa | S | 4, 5 |
| 7 | Set up CI/CD | local-devops | M | 4, 5 |

## Open Questions
- List any decisions that need to be made
- Link to ADRs that need to be written

## Risks
- Technical risks and mitigation strategies
```

### 4. Create/update GitHub issues
- Create a GitHub issue for each task
- Set labels: `teammate:<name>`, `complexity:<S|M|L>`, `iteration:vX.Y`
- Add dependency references in issue body
- Assign to the appropriate milestone
- Add `status:ready` label to tasks with no unresolved dependencies

### 5. Summary
Output the plan summary with:
- Total task count by teammate
- Critical path (longest dependency chain)
- Recommended execution order
