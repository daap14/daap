---
globs: ["**/validation/**/*.go", "**/handler/**/*.go"]
---

# Validation Conventions

## Zero Values
- Be aware of Go zero values: an omitted `int` JSON field defaults to 0, an omitted `string` defaults to ""
- Explicitly reject zero values when they are not valid inputs (e.g., `instances` must be >= 1, not >= 0)
- When the spec says "between X and Y", the validation condition MUST match: `< X || > Y`

## Error Message Accuracy
- Validation error messages must exactly match the actual validation logic
- If the message says "must be between 1 and 10", the code must reject values < 1 and > 10
- Review error messages after changing validation bounds

## Completeness
- Validate all user-provided fields before any database or external service call
- Return all field errors at once (don't stop at the first error)
- Use structured field errors with `field` and `message` for machine-readable responses
