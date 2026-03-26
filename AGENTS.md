# AGENTS.md

## Build Commands

Build tool: **mise** (tasks defined in `mise.toml`)

```bash
mise run build          # mkdir -p bin && go build -o bin/mcp-postgres ./cmd/mcp-postgres
mise run run            # go run ./cmd/mcp-postgres (STDIO mode)
mise run tidy           # go mod tidy
mise run clean          # remove local build and coverage artifacts
```

## Test Commands

```bash
mise run test                         # unit tests; clears POSTGRES_INTEGRATION_DSN
mise run test-unit                    # short unit tests only
mise run test-integration             # requires POSTGRES_INTEGRATION_DSN and a reachable PostgreSQL server
mise run integration-up               # start local Docker PostgreSQL fixture
mise run integration-down             # stop and remove local Docker PostgreSQL fixture
mise run test-integration-docker      # run integration tests against disposable Docker PostgreSQL
go test -v ./...                      # all Go tests in the current environment
go test -v ./internal/postgres/...    # single package
go test -v -run TestParseAndClassify ./internal/sqlguard/
```

## Lint

```bash
mise run lint           # golangci-lint run ./...
mise run vulncheck      # govulncheck ./...
```

Run lint, vulncheck, and build after any code change.

## Project Structure

```text
cmd/mcp-postgres/main.go      # Entry point and STDIO transport bootstrap
internal/
  audit/                       # Structured audit logs for DML/admin operations
  config/                      # Env-var config with defaults and validation
  confirm/                     # Single-use confirmation tokens for risky admin statements
  logging/                     # slog setup, stderr only
  mcp/                         # MCP server and tool registration
  metrics/                     # In-process runtime counters
  policy/                      # Mode, schema, database, and mutation policy
  postgres/                    # Pool manager, introspection, query, DML, admin execution
  sqlguard/                    # PostgreSQL parser-backed SQL classification
  types/                       # MCP input/output structs
```

## Code Style

### Imports

Three groups separated by blank lines: stdlib, external dependencies, project packages.

```go
import (
    "context"
    "fmt"

    "github.com/jackc/pgx/v5"

    "github.com/aparcero/mcp-postgres/internal/types"
)
```

### Naming

- Go exports: PascalCase. Variables: camelCase.
- JSON tags: snake_case for existing MCP fields (`json:"max_rows"`, `json:"confirmation_token"`).
- Acronyms stay capitalized: `BaseDSN`, `SQLHash`, `DMLRecord`.
- MCP tool names use the `postgres.<action>` namespace.

### Error Handling

- Wrap errors with `fmt.Errorf("context: %w", err)`.
- Return policy and validation failures as Go errors from manager methods; the MCP SDK turns them into tool errors.
- Do not log raw SQL. Audit logs use parser fingerprints and statement metadata.

### Safety Rules

- Every database-specific tool takes an explicit `database` argument.
- Do not add `switch_database` or mutable current-database session state.
- Keep stdout protocol-only. Logs must go to stderr or another non-stdio protocol sink.
- SQL safety decisions must stay parser-backed; do not replace `sqlguard` with string-prefix checks.
- Mutating/admin changes need tests in both `internal/sqlguard` and `internal/policy` when classification or authorization changes.

## Testing Conventions

- Tests are in the same package.
- Stdlib only; use `t.Fatalf` for assertions.
- Integration tests are skipped unless `POSTGRES_INTEGRATION_DSN` is set.
- Docker-backed integration tests use `docker-compose.integration.yml` as a local convenience fixture; Go tests still rely only on `POSTGRES_INTEGRATION_DSN`.
- Public README feature claims should have coverage in unit, MCP surface, or PostgreSQL integration tests.
- Prefer focused unit tests for classification, policy, config validation, encoding, and query construction.

## Adding a New MCP Tool

1. Define input and output structs in `internal/types`.
2. Add the manager behavior in `internal/postgres` or the appropriate internal package.
3. Register the tool in `internal/mcp/server.go` via `mcp.AddTool`.
4. Add unit tests and integration tests when the tool talks to PostgreSQL.
5. Update `README.md` with tool purpose, arguments, safety notes, and examples.
