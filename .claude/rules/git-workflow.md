---
globs:
  - "**"
---

# Git Workflow

## Branch-Per-Task
- Every task gets its own branch — no committing directly to `master`
- Branch naming: `feat/<description>`, `fix/<description>`, `chore/<description>`
- Keep branches focused on a single task — don't mix unrelated changes
- Branch from `master`, merge back to `master` via PR

## Pull Requests
- Every task results in a PR — no direct pushes to `master`
- PR title follows Conventional Commits: `type(scope): description`
- PR description includes: what changed, why, and how to test
- Link the related GitHub issue in the PR description
- PRs must be reviewed before merging

## Review Cycle
1. Dev creates PR and waits for CI to pass
2. If CI fails: dev fixes, pushes, waits for CI again
3. Once CI is green: dev notifies the reviewer that the PR is ready
4. Reviewer adds comments on GitHub (line-level and general)
5. Dev applies suggestions, pushes updates
6. Dev notifies the reviewer to re-review
7. Repeat steps 4-6 until the reviewer approves
8. Once approved, the dev or lead merges the PR

## Commits
- Use Conventional Commits: `type(scope): description`
- Types: feat, fix, refactor, test, docs, chore, ci, perf
- Keep subject line under 72 characters
- Use imperative mood ("add feature" not "added feature")
- Each commit should be a logical unit — don't mix unrelated changes in one commit

## Documentation in PRs
- If the PR adds or changes API endpoints, update `README.md`
- If the PR establishes a new convention, create/update `.claude/rules/`
- The reviewer checks for missing documentation as part of the review

## Rule Creation from Reviews
- When a reviewer identifies a pattern that applies beyond the current PR, it becomes a `.claude/rules/` file
- Rules capture "how we do things" — they prevent the same review comment from being repeated
- Both the dev and reviewer can create rules
