# General Conventions

## Commit Messages
- Use Conventional Commits: `type(scope): description`
- Types: feat, fix, refactor, test, docs, chore, ci, perf
- Keep subject line under 72 characters
- Use imperative mood ("add feature" not "added feature")

## Code Style
- Composition over inheritance
- Prefer pure functions where possible
- No hardcoded secrets — use environment variables
- No magic numbers — use named constants
- Keep functions under 40 lines; extract if longer
- Single responsibility: one function/class does one thing

## Error Handling
- Never swallow errors silently
- Use typed/structured errors, not generic strings
- Log errors with context (what operation, what input)
- Fail fast on invalid state

## Naming
- Use descriptive, unambiguous names
- Boolean variables: prefix with `is`, `has`, `should`, `can`
- Collections: use plural nouns
- Functions: use verb phrases

## Dependencies
- Pin exact versions in lockfiles
- Prefer well-maintained, minimal dependencies
- Evaluate bundle size impact before adding new deps

## Security
- Validate and sanitize all external input
- Use parameterized queries for database access
- Never log sensitive data (tokens, passwords, PII)
- Set appropriate CORS and security headers
