---
globs:
  - "src/routes/**"
  - "src/controllers/**"
---

# API Design Conventions

## RESTful Design
- Use plural nouns for resources: `/users`, `/orders`
- Use HTTP methods correctly: GET (read), POST (create), PUT (full update), PATCH (partial update), DELETE (remove)
- Nest sub-resources: `/users/:id/orders`
- Use query params for filtering/sorting/pagination: `?status=active&sort=-createdAt&page=2&limit=20`

## Response Envelope
All API responses use a consistent envelope:

```json
{
  "data": {},
  "error": null,
  "meta": {
    "requestId": "uuid",
    "timestamp": "ISO8601"
  }
}
```

- Success: `data` populated, `error` is null
- Error: `data` is null, `error` has `code`, `message`, and optional `details`
- List responses: `data` is array, `meta` includes `total`, `page`, `limit`

## Status Codes
- 200: Success (GET, PUT, PATCH)
- 201: Created (POST)
- 204: No Content (DELETE)
- 400: Bad Request (validation failure)
- 401: Unauthorized (missing/invalid auth)
- 403: Forbidden (insufficient permissions)
- 404: Not Found
- 409: Conflict (duplicate, state conflict)
- 422: Unprocessable Entity (business logic error)
- 500: Internal Server Error

## Input Validation
- Validate all request bodies before processing
- Return 400 with specific field errors
- Sanitize string inputs (trim whitespace, escape HTML)
- Enforce max lengths on string fields

## Documentation
- Add OpenAPI-compatible comments on route handlers
- Document request/response schemas
- Include example values for all fields
