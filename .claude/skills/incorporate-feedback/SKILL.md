---
description: Process feedback into categorized requirements, updating iteration specs and GitHub issues
user-invocable: true
disable-model-invocation: true
tools:
  - Read
  - Write
  - Edit
  - Bash
  - Glob
  - Grep
allowed-tools:
  - Bash(gh *)
---

# /incorporate-feedback

Process user/stakeholder feedback into actionable requirement changes.

## Usage
```
/incorporate-feedback docs/feedback/v0.1-feedback.md
```

## Prerequisites
- Feedback file exists with raw feedback entries
- Current iteration spec exists

## Steps

### 1. Read and parse feedback
Read the feedback file and categorize each item:
- **Bug**: something is broken or behaving incorrectly
- **UX**: usability improvement, confusing behavior
- **Feature Request**: new functionality not in current spec
- **Performance**: speed, responsiveness, resource usage
- **Security**: vulnerability or concern

### 2. Prioritize feedback items
For each item, assess:
- **Impact**: how many users affected, severity
- **Effort**: estimated complexity to address (S/M/L)
- **Urgency**: blocks usage vs. nice-to-have

Priority matrix:
- P0 (Critical): bugs that block core functionality → fix in current iteration
- P1 (High): significant UX issues, security concerns → next iteration
- P2 (Medium): feature requests aligned with vision → plan for upcoming iteration
- P3 (Low): nice-to-haves, minor polish → backlog

### 3. Update iteration specs
Based on priority:
- **P0 items**: add to current iteration spec as hotfix tasks
- **P1/P2 items**: add to next iteration spec, or create new iteration if needed
- **P3 items**: note in backlog section of the relevant iteration spec

For each addition, include:
- Source feedback reference
- Acceptance criteria
- Priority label

### 4. Create/update GitHub issues
For each actionable feedback item:
```bash
gh issue create \
  --title "type(scope): description" \
  --body "## Context\nFrom feedback: [reference]\n\n## Acceptance Criteria\n- [ ] criterion" \
  --label "feedback,priority:PX,type:bug|feature|ux" \
  --milestone "vX.Y"
```

For existing issues that need updating:
```bash
gh issue edit <number> --add-label "priority:P1"
gh issue comment <number> --body "Updated based on feedback: [details]"
```

### 5. Create feedback summary
Write/update `docs/feedback/vX.Y-feedback-summary.md`:

```markdown
# Feedback Summary — vX.Y

## Stats
- Total items: N
- Bugs: N | UX: N | Features: N | Performance: N | Security: N
- P0: N | P1: N | P2: N | P3: N

## Actions Taken
- N issues created
- N issues updated
- N items added to iteration vX.Y spec
- N items added to backlog

## Deferred Items
Items not actioned and rationale.
```

### 6. Notify
Output summary of changes made:
- Which iteration specs were modified
- Which issues were created/updated
- Any P0 items that need immediate attention

## Output
- Updated iteration spec files
- New/updated GitHub issues
- Feedback summary document
- Action report for the lead
