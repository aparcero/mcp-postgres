package mcpserver

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/aparcero/mcp-postgres/internal/config"
	"github.com/aparcero/mcp-postgres/internal/policy"
	"github.com/aparcero/mcp-postgres/internal/postgres"
	"github.com/aparcero/mcp-postgres/internal/types"
)

const serverVersion = "0.6.0"

type App struct {
	Server  *mcp.Server
	manager *postgres.Manager
}

func New(ctx context.Context, cfg config.Config, logger *slog.Logger) (*App, error) {
	manager, err := postgres.NewManager(cfg, logger)
	if err != nil {
		return nil, err
	}

	if err := manager.PingBootstrap(ctx); err != nil {
		manager.Close()
		return nil, fmt.Errorf("ping bootstrap database: %w", err)
	}

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "mcp-postgres",
		Version: serverVersion,
	}, &mcp.ServerOptions{
		Instructions: fmt.Sprintf("PostgreSQL MCP server with explicit database targeting. Current execution mode: %s.", cfg.Mode),
		Logger:       logger,
	})

	registerTools(server, manager, cfg)

	return &App{
		Server:  server,
		manager: manager,
	}, nil
}

func (a *App) Close() {
	if a.manager != nil {
		a.manager.Close()
	}
}

func registerTools(server *mcp.Server, manager *postgres.Manager, cfg config.Config) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "postgres.list_databases",
		Description: "List visible PostgreSQL databases in the instance.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ types.EmptyInput) (*mcp.CallToolResult, types.ListDatabasesOutput, error) {
		out, err := manager.ListDatabases(ctx)
		return nil, out, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "postgres.get_connection_status",
		Description: "Return bootstrap PostgreSQL connection health and instance details.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ types.EmptyInput) (*mcp.CallToolResult, types.ConnectionStatusOutput, error) {
		out, err := manager.GetConnectionStatus(ctx)
		return nil, out, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "postgres.get_server_metrics",
		Description: "Return in-process runtime metrics, pooled connection stats, and idle cleanup state.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ types.EmptyInput) (*mcp.CallToolResult, types.ServerMetricsOutput, error) {
		out, err := manager.GetServerMetrics(ctx)
		return nil, out, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "postgres.list_schemas",
		Description: "List schemas in a PostgreSQL database.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in types.DatabaseInput) (*mcp.CallToolResult, types.ListSchemasOutput, error) {
		out, err := manager.ListSchemas(ctx, in.Database)
		return nil, out, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "postgres.list_tables",
		Description: "List tables and views in a PostgreSQL database, optionally filtered by schema.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in types.ListTablesInput) (*mcp.CallToolResult, types.ListTablesOutput, error) {
		out, err := manager.ListTables(ctx, in.Database, in.Schema)
		return nil, out, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "postgres.describe_table",
		Description: "Describe a PostgreSQL table including columns, indexes, and constraints.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in types.DescribeTableInput) (*mcp.CallToolResult, types.DescribeTableOutput, error) {
		out, err := manager.DescribeTable(ctx, in.Database, in.Schema, in.Table)
		return nil, out, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "postgres.sample_table",
		Description: "Return a safe sample of rows from a PostgreSQL table.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in types.SampleTableInput) (*mcp.CallToolResult, types.SampleTableOutput, error) {
		out, err := manager.SampleTable(ctx, in.Database, in.Schema, in.Table, sampleLimit(cfg, in.Limit))
		return nil, out, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "postgres.query",
		Description: "Execute a single read-only SQL statement with row truncation and timeout controls.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in types.QueryInput) (*mcp.CallToolResult, types.QueryOutput, error) {
		out, err := manager.Query(ctx, in.Database, in.SQL, queryMaxRows(cfg, in.MaxRows), toolTimeout(in.TimeoutMs, cfg.QueryTimeout))
		return nil, out, err
	})

	if cfg.Mode == policy.ModeOperator || cfg.Mode == policy.ModeAdmin {
		registerDMLTool(server, manager, cfg)
	}
	if cfg.Mode == policy.ModeAdmin {
		registerAdminTool(server, manager, cfg)
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "postgres.count_rows",
		Description: "Count rows in a PostgreSQL table with optional simple equality filters.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in types.CountRowsInput) (*mcp.CallToolResult, types.CountRowsOutput, error) {
		out, err := manager.CountRows(ctx, in.Database, in.Schema, in.Table, in.Where)
		return nil, out, err
	})
}

func registerDMLTool(server *mcp.Server, manager *postgres.Manager, cfg config.Config) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "postgres.exec_dml",
		Description: "Execute a single INSERT, UPDATE, DELETE, or MERGE statement under mutation policy controls.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in types.ExecDMLInput) (*mcp.CallToolResult, types.ExecDMLOutput, error) {
		out, err := manager.ExecDML(ctx, in.Database, in.SQL, toolTimeout(in.TimeoutMs, cfg.ExecTimeout))
		return nil, out, err
	})
}

func registerAdminTool(server *mcp.Server, manager *postgres.Manager, cfg config.Config) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "postgres.exec_admin",
		Description: "Execute a single administrative SQL statement. Destructive and privilege-changing operations require a confirmation token.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in types.ExecAdminInput) (*mcp.CallToolResult, types.ExecAdminOutput, error) {
		out, err := manager.ExecAdmin(ctx, in.Database, in.SQL, toolTimeout(in.TimeoutMs, cfg.ExecTimeout), in.ConfirmationToken)
		return nil, out, err
	})
}

func sampleLimit(cfg config.Config, requested int) int {
	if requested <= 0 {
		return cfg.DefaultSampleRows
	}
	if requested > cfg.MaxSampleRows {
		return cfg.MaxSampleRows
	}
	return requested
}

func queryMaxRows(cfg config.Config, requested int) int {
	if requested <= 0 {
		return cfg.DefaultMaxRows
	}
	if requested > cfg.MaxMaxRows {
		return cfg.MaxMaxRows
	}
	return requested
}

func toolTimeout(requestedMs int, fallback time.Duration) time.Duration {
	timeout := time.Duration(requestedMs) * time.Millisecond
	if timeout <= 0 {
		return fallback
	}
	return timeout
}
