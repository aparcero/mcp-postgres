# Contributing

## Development Setup

Install Go 1.26 or newer. `mise` is optional, but the repository task commands use it.

```bash
go test -v ./...
mkdir -p bin
go build -o bin/mcp-postgres ./cmd/mcp-postgres
```

With `mise`:

```bash
mise install
mise run test
mise run build
```

For local environment overrides:

```bash
cp .env.example .env
```

Do not commit real `.env` files, DSNs, passwords, tokens, database dumps, or query output that may contain private data.

## Pull Requests

Before opening a pull request, run:

```bash
mise run tidy
mise run test
mise run lint
mise run vulncheck
mise run build
```

If you do not use `mise`, run the equivalent commands:

```bash
go mod tidy
env -u POSTGRES_INTEGRATION_DSN go test -v ./...
go vet ./...
go tool govulncheck ./...
mkdir -p bin
go build -o bin/mcp-postgres ./cmd/mcp-postgres
```

Run integration tests when a change affects PostgreSQL behavior:

```bash
POSTGRES_INTEGRATION_DSN='postgresql://postgres:postgres@127.0.0.1:5432/postgres?sslmode=disable' mise run test-integration
```

## Code Style

- Keep MCP tool handlers small and delegate database work to `internal/postgres`.
- Preserve explicit `database` arguments; do not add mutable current-database session state.
- Treat SQL admission and policy code as security-sensitive. Add focused tests for classification and authorization changes.
- Preserve JSON field names used by existing clients.
- Return logs through stderr only; stdout is reserved for the MCP protocol.
- Avoid introducing new external dependencies unless they remove meaningful complexity.

## Documentation

Update `README.md` when a change affects installation, configuration, tool arguments, tool outputs, safety behavior, or operational behavior.
