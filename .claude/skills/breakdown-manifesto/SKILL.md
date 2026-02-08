---
description: Parse product manifesto into iteration specs, GitHub milestones, and issues
user-invocable: true
disable-model-invocation: true
tools:
  - Read
  - Write
  - Bash
  - Glob
  - Grep
allowed-tools:
  - Bash(gh *)
---

# /breakdown-manifesto

Transform the product manifesto (`docs/MANIFESTO.md`) into actionable iteration specs with GitHub tracking.

## Steps

### 1. Read and analyze the manifesto
- Read `docs/MANIFESTO.md`
- Identify core features, user stories, and acceptance criteria
- Identify technical requirements and constraints
- Note any dependencies between features

### 2. Group features into iterations
- **v0.1** — Minimum viable product: the smallest useful version
- **v0.2+** — Incremental additions, each building on the previous
- Each iteration should be completable in a focused development sprint
- Order by dependency (foundation first) and value (high-impact early)

### 3. Create iteration spec files
For each iteration, create `docs/iterations/vX.Y.md` using this template:

```markdown
# Iteration vX.Y — [Title]

## Goal
One-sentence summary.

## Features
- [ ] **Feature Name**: Description
  - Acceptance: specific, testable criteria
  - Priority: must-have | nice-to-have

## Non-Goals
What is explicitly out of scope for this iteration.

## Dependencies
- Previous iteration features needed
- External services or APIs
- Technical decisions needed (link to ADR if exists)

## Acceptance Criteria
How we know this iteration is complete.
```

### 4. Create GitHub milestones and issues
Using the `gh` CLI:
- Create a milestone for each iteration: `gh api repos/{owner}/{repo}/milestones -f title="vX.Y" -f description="..."`
- Create issues for each feature within each iteration
- Apply labels: `iteration:vX.Y`, `type:feature`, priority labels
- Assign issues to the appropriate milestone
- Add task dependencies as issue references

### 5. Update project state
- Update `CLAUDE.md` to reflect the current iteration pointer
- Output a summary of what was created

## Output
- `docs/iterations/v0.1.md`, `v0.2.md`, etc.
- GitHub milestones for each iteration
- GitHub issues for each feature
- Summary of the breakdown with issue counts per iteration
