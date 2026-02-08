---
description: Create a versioned release with tag, changelog entry, and GitHub release
user-invocable: true
disable-model-invocation: true
tools:
  - Read
  - Write
  - Edit
  - Bash
allowed-tools:
  - Bash(git *)
  - Bash(gh *)
  - Bash(npm version *)
---

# /create-release

Create a versioned release with git tag, changelog entry, and GitHub release.

## Usage
```
/create-release v0.1.0
/create-release v0.2.0
```

## Steps

### 1. Pre-release checks
Verify the release is ready:
```bash
# All tests pass
make test

# Build succeeds
make build

# No uncommitted changes
git status --porcelain
```

If any check fails, report the issue and stop.

### 2. Determine changes since last release
```bash
# Get the last tag
LAST_TAG=$(git describe --tags --abbrev=0 2>/dev/null || echo "")

# Get commits since last tag (or all commits if first release)
if [ -n "$LAST_TAG" ]; then
  git log "$LAST_TAG"..HEAD --oneline --no-merges
else
  git log --oneline --no-merges
fi
```

### 3. Generate changelog entry
Parse commit messages (Conventional Commits format) and categorize:

```markdown
## [vX.Y.Z] — YYYY-MM-DD

### Features
- feat(scope): description (#issue)

### Bug Fixes
- fix(scope): description (#issue)

### Other Changes
- refactor/docs/chore entries
```

### 4. Update CHANGELOG.md
- Insert the new version entry at the top (below the header)
- Link the version to a GitHub compare URL

### 5. Update version references
If applicable:
- `package.json` version field
- Any version constants in source code
- API version headers

### 6. Commit the release
```bash
git add CHANGELOG.md [other version files]
git commit -m "chore(release): vX.Y.Z"
```

### 7. Create git tag
```bash
git tag -a "vX.Y.Z" -m "Release vX.Y.Z"
```

### 8. Create GitHub release
```bash
gh release create "vX.Y.Z" \
  --title "vX.Y.Z" \
  --notes-file <(extract changelog for this version)
```

### 9. Close the milestone
```bash
# Find and close the milestone for this iteration
gh api repos/{owner}/{repo}/milestones --jq '.[] | select(.title=="vX.Y") | .number' | \
  xargs -I{} gh api repos/{owner}/{repo}/milestones/{} -X PATCH -f state=closed
```

### 10. Update CLAUDE.md
Update the "Current State" section to point to the next iteration.

## Output
- Updated `CHANGELOG.md`
- Git tag created
- GitHub release published
- Milestone closed
- Project state updated
