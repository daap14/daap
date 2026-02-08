---
globs:
  - "docs/**"
---

# Documentation Conventions

## Architecture Decision Records (ADRs)
Location: `docs/architecture/decisions/NNN-title.md`

Template:
```markdown
# NNN. Title

## Status
Proposed | Accepted | Deprecated | Superseded by [NNN]

## Context
What is the issue or question that motivates this decision?

## Decision
What is the change we are making?

## Consequences
What are the positive, negative, and neutral results of this decision?
```

- Number ADRs sequentially: 001, 002, 003...
- Never delete ADRs — mark as Deprecated or Superseded
- Link related ADRs to each other

## Iteration Specs
Location: `docs/iterations/vX.Y.md`

Template:
```markdown
# Iteration vX.Y — Title

## Goal
One-sentence summary of what this iteration achieves.

## Features
- [ ] Feature 1: description + acceptance criteria
- [ ] Feature 2: description + acceptance criteria

## Non-Goals
What is explicitly out of scope.

## Dependencies
External services, APIs, or decisions needed.

## Acceptance Criteria
How we know this iteration is complete.
```

## Feedback Logs
Location: `docs/feedback/vX.Y-feedback.md`

- Date and source of each feedback item
- Categorize: bug, UX, feature request, performance
- Link to resulting GitHub issues

## General
- Write for a developer audience
- Keep docs close to the code they describe
- Update docs when the code changes — stale docs are worse than no docs
