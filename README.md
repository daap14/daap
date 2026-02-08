# DAAP

AI-assisted product development workflow. Transforms a product manifesto into a working API/backend through iterative releases with feedback loops.

## Getting Started

1. Write your product vision in `docs/MANIFESTO.md`
2. Run `/breakdown-manifesto` to create iteration specs and GitHub issues
3. Discuss tech stack with Claude to produce an ADR
4. Run `/plan-iteration docs/iterations/v0.1.md` to create the first technical plan
5. Create an agent team to start building

## Project Structure

See `CLAUDE.md` for full project context and conventions.

## Development

```bash
make setup    # Initial project setup
make dev      # Start dev server
make test     # Run tests
make lint     # Run linter
```

## License

TBD
