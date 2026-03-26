# mcp-postgres

An MCP server that gives AI agents controlled access to PostgreSQL databases for inspection, read-only querying, guarded DML, and administrative operations.

It helps agents answer and act on questions such as:

- Which databases, schemas, tables, views, columns, indexes, and constraints exist?
- What does a safe sample of this table look like?
- How many rows match these simple equality filters?
- Can I run this single read-only SQL statement and get JSON-friendly rows back?
- Can an operator run a scoped `INSERT`, `UPDATE`, `DELETE`, or `MERGE` under explicit policy?
- Can an administrator run selected DDL/DCL with confirmation for destructive or privilege-changing statements?

The server supports STDIO transport for desktop agents, IDE agents, and CLI agents. It does not expose HTTP by itself.

## Status

This project is ready to build from source. Release binaries and package-manager distribution can be added later without changing the MCP tool surface.

## Features

- Explicit database targeting on every database-specific tool
- No mutable current-database session state
- PostgreSQL parser-backed SQL classification
- Read-only query tool with single-statement admission, row caps, timeout controls, and read-only transactions
- JSON-friendly query output with column metadata and duplicate-column handling
- Dedicated inspection tools for databases, schemas, tables, columns, indexes, constraints, sampling, and counts
- Optional controlled DML mode with database allowlisting, schema-qualified targets, denied schemas, and broad `UPDATE`/`DELETE` blocking
- Optional admin mode with confirmation tokens for destructive and privilege-changing operations
- Per-database connection pools with idle eviction and configurable pool caps
- Runtime metrics through an MCP tool
- Audit logs for DML and admin operations without logging raw SQL

## Requirements

- Go 1.26 or newer
- Access to a PostgreSQL server
- A PostgreSQL role with the least privileges needed for the enabled tools
- Optional: `mise` for the task commands in `mise.toml`

## Quick Start

Clone the repository and build the server:

```bash
git clone https://github.com/aparcero/mcp-postgres.git
cd mcp-postgres
mkdir -p bin
go build -o bin/mcp-postgres ./cmd/mcp-postgres
```

Or install it with Go:

```bash
go install github.com/aparcero/mcp-postgres/cmd/mcp-postgres@latest
```

With `mise`:

```bash
mise install
mise run build
```

Create local configuration:

```bash
cp .env.example .env
```

At minimum, set `POSTGRES_BASE_DSN`:

```bash
POSTGRES_BASE_DSN='postgresql://readonly_user:change-me@127.0.0.1:5432/postgres?sslmode=disable'
```

Use a PostgreSQL role that matches the mode you enable. For general agent use, start with `POSTGRES_MODE=readonly`.

## Configure an MCP Client

Most MCP clients that support STDIO servers accept a JSON block similar to this. Use an absolute path because agents often launch servers from a different working directory.

```json
{
  "mcpServers": {
    "postgres": {
      "command": "/absolute/path/to/mcp-postgres/bin/mcp-postgres",
      "args": [],
      "env": {
        "POSTGRES_BASE_DSN": "postgresql://readonly_user:change-me@127.0.0.1:5432/postgres?sslmode=disable",
        "POSTGRES_MODE": "readonly",
        "POSTGRES_LOG_LEVEL": "warn"
      }
    }
  }
}
```

For development without building a binary:

```json
{
  "mcpServers": {
    "postgres": {
      "command": "go",
      "args": ["run", "/absolute/path/to/mcp-postgres/cmd/mcp-postgres"],
      "env": {
        "POSTGRES_BASE_DSN": "postgresql://readonly_user:change-me@127.0.0.1:5432/postgres?sslmode=disable",
        "POSTGRES_MODE": "readonly",
        "POSTGRES_LOG_LEVEL": "warn"
      }
    }
  }
}
```

Client notes:

- Claude Desktop, Claude Code, Cline, Continue, VS Code MCP extensions, Gemini CLI, Codex, OpenCode, IDE agents, and other CLI agents usually use a variant of the `mcpServers` block above.
- If a client has separate fields for command, arguments, and environment, copy the same values into those fields.
- STDIO mode is intended to be launched by the MCP client. Running it directly in a terminal will wait for MCP protocol messages on stdin.
- Prefer `POSTGRES_LOG_LEVEL=warn` or `POSTGRES_LOG_LEVEL=error` in STDIO mode so diagnostic logs do not distract from agent output.
- Do not commit MCP client config that contains real database credentials.

### Client-Specific Hints

Claude Code:

```bash
claude mcp add --scope local \
  --env POSTGRES_BASE_DSN=postgresql://readonly_user:change-me@127.0.0.1:5432/postgres?sslmode=disable \
  --env POSTGRES_MODE=readonly \
  postgres -- /absolute/path/to/mcp-postgres/bin/mcp-postgres
```

Codex:

```bash
codex mcp add postgres \
  --env POSTGRES_BASE_DSN=postgresql://readonly_user:change-me@127.0.0.1:5432/postgres?sslmode=disable \
  --env POSTGRES_MODE=readonly \
  -- /absolute/path/to/mcp-postgres/bin/mcp-postgres
```

Shared project config should reference environment variables instead of embedding secrets:

```json
{
  "mcpServers": {
    "postgres": {
      "command": "/absolute/path/to/mcp-postgres/bin/mcp-postgres",
      "env": {
        "POSTGRES_BASE_DSN": "${POSTGRES_BASE_DSN}",
        "POSTGRES_MODE": "readonly"
      }
    }
  }
}
```

## Tools

| Tool | Availability | Purpose |
| --- | --- | --- |
| `postgres.list_databases` | Always | List visible non-template databases. |
| `postgres.get_connection_status` | Always | Return bootstrap connection health and instance details. |
| `postgres.get_server_metrics` | Always | Return in-process metrics, pool stats, cleanup state, and pending confirmation count. |
| `postgres.list_schemas` | Always | List schemas in a target database. |
| `postgres.list_tables` | Always | List tables and views in a target database, optionally filtered by schema. |
| `postgres.describe_table` | Always | Return columns, indexes, and constraints for a table. |
| `postgres.sample_table` | Always | Return a capped sample of rows from a schema-qualified table. |
| `postgres.count_rows` | Always | Count rows with optional simple equality filters. |
| `postgres.query` | Always | Execute one parser-verified read-only SQL statement. |
| `postgres.exec_dml` | `operator` or `admin` mode, allowlisted database only | Execute one `INSERT`, `UPDATE`, `DELETE`, or `MERGE` under mutation policy. |
| `postgres.exec_admin` | `admin` mode, allowlisted database only | Execute one administrative SQL statement; destructive and privilege-changing statements require confirmation. |

### Common Arguments

Most database-specific tools use explicit database targeting:

```json
{
  "database": "app_db"
}
```

Table tools require schema-qualified identity:

```json
{
  "database": "app_db",
  "schema": "public",
  "table": "orders"
}
```

`postgres.query`, `postgres.exec_dml`, and `postgres.exec_admin` accept exactly one SQL statement:

```json
{
  "database": "app_db",
  "sql": "select id, status from public.orders order by id desc limit 5"
}
```

## Example Tool Calls

List tables:

```json
{
  "name": "postgres.list_tables",
  "arguments": {
    "database": "app_db",
    "schema": "public"
  }
}
```

Describe a table:

```json
{
  "name": "postgres.describe_table",
  "arguments": {
    "database": "app_db",
    "schema": "public",
    "table": "orders"
  }
}
```

Sample rows:

```json
{
  "name": "postgres.sample_table",
  "arguments": {
    "database": "app_db",
    "schema": "public",
    "table": "orders",
    "limit": 10
  }
}
```

Count rows with equality filters:

```json
{
  "name": "postgres.count_rows",
  "arguments": {
    "database": "app_db",
    "schema": "public",
    "table": "orders",
    "where": {
      "status": "pending",
      "deleted_at": null
    }
  }
}
```

Run a read-only query:

```json
{
  "name": "postgres.query",
  "arguments": {
    "database": "app_db",
    "sql": "select id, status, created_at from public.orders order by created_at desc limit 20",
    "max_rows": 20,
    "timeout_ms": 5000
  }
}
```

Run controlled DML in `operator` mode:

```json
{
  "name": "postgres.exec_dml",
  "arguments": {
    "database": "app_db",
    "sql": "update public.orders set status = 'archived' where status = 'closed' and closed_at < now() - interval '90 days'",
    "timeout_ms": 5000
  }
}
```

Run an admin operation that requires confirmation:

1. Call `postgres.exec_admin` without `confirmation_token`.

```json
{
  "name": "postgres.exec_admin",
  "arguments": {
    "database": "app_db",
    "sql": "drop table public.old_orders"
  }
}
```

2. If the response status is `confirmation_required`, call the same tool again with the returned token and the exact same SQL.

```json
{
  "name": "postgres.exec_admin",
  "arguments": {
    "database": "app_db",
    "sql": "drop table public.old_orders",
    "confirmation_token": "returned-token"
  }
}
```

## Response Format

Successful tools return structured JSON through MCP content. The exact fields vary by tool.

`postgres.query` returns rows as JSON objects keyed by column name:

```json
{
  "database": "app_db",
  "columns": [
    {"name": "id", "db_type": "int8"},
    {"name": "status", "db_type": "text"}
  ],
  "rows": [
    {"id": 123, "status": "pending"}
  ],
  "row_count": 1,
  "truncated": false,
  "duration_ms": 4
}
```

When duplicate column names appear, later duplicates are renamed (`id_2`, `id_3`) and include `source_name`.

Tool failures are returned as MCP tool errors with a human-readable message. Common causes include invalid arguments, rejected SQL classification, denied schema targets, mutation database allowlist failures, timeouts, and PostgreSQL execution errors.

## Safety Model

This server is designed for controlled database access, not as a substitute for PostgreSQL permissions.

Recommended baseline:

- Use a dedicated PostgreSQL role for the MCP server.
- Start with `POSTGRES_MODE=readonly`.
- Grant access only to databases and schemas agents should inspect.
- Use `POSTGRES_DENIED_SCHEMAS` to block sensitive schemas from write/admin targets.
- Enable `operator` or `admin` mode only for trusted local workflows.
- Configure `POSTGRES_MUTATION_DATABASES` before any write/admin mode.
- Keep backups, audit logging, row-level security, and network controls outside this server.

Mode behavior:

| Mode | Behavior |
| --- | --- |
| `readonly` | Inspection tools and `postgres.query`. Queries are parser-checked as read-only and executed in a PostgreSQL read-only transaction. |
| `operator` | Adds `postgres.exec_dml` for allowlisted mutation databases. Non-read-only targets must be schema-qualified. `UPDATE` and `DELETE` require a `WHERE` clause. |
| `admin` | Adds `postgres.exec_admin` for allowlisted mutation databases. Destructive and privilege-changing statements require a single-use confirmation token. |

Important limitations:

- PostgreSQL functions can have side effects. A read-only transaction blocks writes through normal SQL, but least-privilege database roles are still required.
- Parser-backed checks reject unsupported or session-changing statements rather than trying to guess intent.
- The server does not inspect PostgreSQL role grants or prove that a DSN is safe.
- Confirmation tokens reduce accidental execution risk; they are not multi-user authorization.

## Configuration

Copy `.env.example` to `.env` for local development if you use `mise`. Real `.env` files are ignored by git.

| Variable | Default | Description |
| --- | --- | --- |
| `POSTGRES_BASE_DSN` | required | Base PostgreSQL connection string. Its database is replaced per tool call while preserving user, host, SSL, application name, and other options. |
| `POSTGRES_BOOTSTRAP_DATABASE` | `postgres` | Database used for bootstrap connection, health, and database listing. |
| `POSTGRES_MODE` | `readonly` | `readonly`, `operator`, or `admin`. |
| `POSTGRES_DENIED_SCHEMAS` | `pg_catalog,information_schema` | Comma-separated schemas blocked for non-read-only targets. |
| `POSTGRES_MUTATION_DATABASES` | empty | Comma-separated database allowlist for `exec_dml` and `exec_admin`. Required for write/admin operations. |
| `POSTGRES_CONFIRMATION_TTL_SECONDS` | `120` | Lifetime for admin confirmation tokens. |
| `POSTGRES_QUERY_TIMEOUT_MS` | `15000` | Default timeout for `postgres.query`. |
| `POSTGRES_EXEC_TIMEOUT_MS` | `15000` | Default timeout for DML/admin execution. |
| `POSTGRES_IDLE_POOL_TTL_MS` | `600000` | Idle TTL for non-bootstrap database pools. Set `0` to disable idle eviction. |
| `POSTGRES_IDLE_POOL_CLEANUP_INTERVAL_MS` | `60000` | Background cleanup interval. Set `0` to disable idle cleanup. |
| `POSTGRES_DEFAULT_MAX_ROWS` | `100` | Default returned row cap for `postgres.query`. |
| `POSTGRES_MAX_MAX_ROWS` | `1000` | Hard upper bound for caller-provided `max_rows`. |
| `POSTGRES_DEFAULT_SAMPLE_ROWS` | `10` | Default sample size for `postgres.sample_table`. |
| `POSTGRES_MAX_SAMPLE_ROWS` | `100` | Hard upper bound for caller-provided sample limits. |
| `POSTGRES_POOL_MAX_CONNS` | `4` | Maximum connections per cached PostgreSQL pool. |
| `POSTGRES_MAX_CACHED_POOLS` | `16` | Maximum number of cached per-database pools. |
| `POSTGRES_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, or `error`. |
| `POSTGRES_LOG_JSON` | `false` | Write logs as JSON to stderr. |
| `POSTGRES_INTEGRATION_DSN` | empty | DSN used only by integration tests. |

Invalid numeric and boolean values fail startup instead of silently falling back to defaults.

DSN examples:

```bash
# TCP with password
POSTGRES_BASE_DSN='postgresql://mcp_reader:secret@db.example.com:5432/postgres?sslmode=require&application_name=mcp-postgres'

# Local PostgreSQL with Unix socket
POSTGRES_BASE_DSN='postgresql:///postgres?host=/var/run/postgresql&user=mcp_reader'
```

## Network and Privacy

The server connects only to the PostgreSQL instance described by `POSTGRES_BASE_DSN`. It does not call external APIs.

The server processes SQL and query results provided by the MCP client and PostgreSQL. Logs go to stderr and audit records intentionally use SQL fingerprints instead of raw SQL, but PostgreSQL error messages may still include object names or other operational detail.

Do not expose this server to untrusted MCP clients with privileged database credentials.

## What This Server Does Not Do

- It does not expose HTTP, SSE, or Streamable HTTP transport.
- It does not authenticate users or implement multi-tenant authorization.
- It does not manage database migrations.
- It does not infer safe database roles or create PostgreSQL users.
- It does not bypass PostgreSQL permissions.
- It does not parse application code, ORMs, or migration files.
- It does not provide backup, restore, or point-in-time recovery.
- It does not guarantee that an arbitrary read-only query is cheap to execute.

Agents should treat the output as database intelligence and combine it with application context, tests, backups, and human review before changing production databases.

## Development

Useful commands:

```bash
mise run test
mise run test-integration
mise run lint
mise run vulncheck
mise run build
mise run tidy
mise run clean
```

Equivalent Go commands:

```bash
env -u POSTGRES_INTEGRATION_DSN go test -v ./...
go vet ./...
mkdir -p bin
go build -o bin/mcp-postgres ./cmd/mcp-postgres
go mod tidy
```

Run a single package or test:

```bash
go test -v ./internal/sqlguard/...
go test -v -run TestParseAndClassify ./internal/sqlguard/
```

Run integration tests against a local PostgreSQL server:

```bash
POSTGRES_INTEGRATION_DSN='postgresql://postgres:postgres@127.0.0.1:5432/postgres?sslmode=disable' mise run test-integration
```

Run integration tests against a disposable Docker PostgreSQL fixture:

```bash
mise run test-integration-docker
```

The Docker fixture uses `127.0.0.1:55432` to avoid most local PostgreSQL port conflicts. CI still runs integration tests against PostgreSQL 15, 16, and 17; the Docker task is a local reproducibility helper.

Build a smaller release-style binary:

```bash
mkdir -p bin
go build -trimpath -ldflags "-s -w" -o bin/mcp-postgres ./cmd/mcp-postgres
```

## Troubleshooting

`server exits with POSTGRES_BASE_DSN must be set`

Set `POSTGRES_BASE_DSN` in the MCP client config or local `.env`.

`failed to initialize server` or `ping bootstrap database`

Check the DSN, network path, PostgreSQL authentication, SSL settings, and `POSTGRES_BOOTSTRAP_DATABASE`.

`database "X" is not allowed for mutation by policy`

Set `POSTGRES_MUTATION_DATABASES` to include the target database, and only do this for databases where write/admin operations are intended.

`non-read-only statements must use explicit schema-qualified targets`

Use targets like `public.orders`, not just `orders`, for DML and admin operations.

`UPDATE and DELETE statements must include a WHERE clause`

Add a `WHERE` clause or use an admin/destructive workflow where appropriate. Broad table changes should be reviewed outside the DML tool.

`confirmation token does not match this operation`

Call `postgres.exec_admin` again with the exact same database, SQL, mode, and returned token before the token expires.

`STDIO client does not show tools`

Use an absolute binary path, restart the client after changing MCP config, and reduce logs with `POSTGRES_LOG_LEVEL=warn`.

## Contributing

Issues and pull requests are welcome. See [CONTRIBUTING.md](CONTRIBUTING.md) for development expectations.

Before opening a pull request, run:

```bash
mise run tidy
mise run test
mise run lint
mise run vulncheck
mise run build
```

## Security

Please do not publish vulnerability details in public issues. See [SECURITY.md](SECURITY.md).

## License

MIT. See [LICENSE](LICENSE).

## Acknowledgments

- [Model Context Protocol](https://modelcontextprotocol.io/)
- [Go MCP SDK](https://github.com/modelcontextprotocol/go-sdk)
- [pgx](https://github.com/jackc/pgx)
- [pg_query_go](https://github.com/pganalyze/pg_query_go)
