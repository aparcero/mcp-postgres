package postgres

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/aparcero/mcp-postgres/internal/config"
	"github.com/aparcero/mcp-postgres/internal/policy"
	"github.com/aparcero/mcp-postgres/internal/types"
)

type integrationDSN struct {
	value    string
	database string
}

func requireIntegrationDSN(t *testing.T) integrationDSN {
	t.Helper()

	dsn := os.Getenv("POSTGRES_INTEGRATION_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_INTEGRATION_DSN is not set")
	}

	poolCfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("ParseConfig() error = %v", err)
	}

	database := poolCfg.ConnConfig.Database
	if database == "" {
		t.Fatal("integration DSN database is empty")
	}

	return integrationDSN{
		value:    dsn,
		database: database,
	}
}

func integrationConfig(t *testing.T, mode policy.Mode, mutationDatabases []string) (config.Config, string) {
	t.Helper()

	dsn := requireIntegrationDSN(t)
	cfg := config.Config{
		BaseDSN:           dsn.value,
		BootstrapDatabase: dsn.database,
		Mode:              mode,
		DeniedSchemas:     policy.DefaultDeniedSchemas,
		MutationDatabases: mutationDatabases,
		ConfirmationTTL:   2 * time.Minute,
		QueryTimeout:      5 * time.Second,
		ExecTimeout:       5 * time.Second,
		IdlePoolTTL:       10 * time.Minute,
		IdlePoolCleanup:   1 * time.Minute,
		DefaultMaxRows:    100,
		MaxMaxRows:        1000,
		DefaultSampleRows: 10,
		MaxSampleRows:     100,
		PoolMaxConns:      4,
		MaxCachedPools:    16,
	}

	return cfg, dsn.database
}

func integrationManager(t *testing.T, mode policy.Mode, mutationDatabases []string) (*Manager, string) {
	t.Helper()

	cfg, database := integrationConfig(t, mode, mutationDatabases)
	manager, err := NewManager(cfg, nil)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	t.Cleanup(manager.Close)

	return manager, database
}

func integrationPool(t *testing.T, ctx context.Context, manager *Manager, database string) *pgxpool.Pool {
	t.Helper()

	pool, err := manager.PoolForDatabase(ctx, database)
	if err != nil {
		t.Fatalf("PoolForDatabase() error = %v", err)
	}
	return pool
}

func createIntegrationSchema(t *testing.T, ctx context.Context, pool *pgxpool.Pool, prefix string) string {
	t.Helper()

	schema := uniqueIntegrationName(prefix)
	if _, err := pool.Exec(ctx, fmt.Sprintf("create schema %s", integrationIdentifier(schema))); err != nil {
		t.Fatalf("create test schema error = %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), fmt.Sprintf("drop schema if exists %s cascade", integrationIdentifier(schema)))
	})

	return schema
}

func uniqueIntegrationName(prefix string) string {
	prefix = strings.ToLower(strings.TrimSpace(prefix))
	var b strings.Builder
	for _, r := range prefix {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}

	normalized := strings.Trim(b.String(), "_")
	if normalized == "" {
		normalized = "mcp_pg_it"
	}
	if len(normalized) > 32 {
		normalized = normalized[:32]
	}

	return fmt.Sprintf("%s_%d", normalized, time.Now().UnixNano())
}

func integrationIdentifier(parts ...string) string {
	return pgx.Identifier(parts).Sanitize()
}

func databaseListed(databases []types.DatabaseInfo, name string) bool {
	for _, database := range databases {
		if database.Name == name {
			return true
		}
	}
	return false
}

func schemaListed(schemas []types.SchemaInfo, name string) bool {
	for _, schema := range schemas {
		if schema.Name == name {
			return true
		}
	}
	return false
}

func tableListed(tables []types.TableInfo, schema, name string) bool {
	for _, table := range tables {
		if table.Schema == schema && table.Name == name {
			return true
		}
	}
	return false
}

func columnListed(columns []types.ColumnInfo, name string) bool {
	for _, column := range columns {
		if column.Name == name {
			return true
		}
	}
	return false
}

func relationExists(t *testing.T, ctx context.Context, pool *pgxpool.Pool, schema, table string) bool {
	t.Helper()

	var exists bool
	if err := pool.QueryRow(ctx, `
		select exists (
			select 1
			from information_schema.tables
			where table_schema = $1 and table_name = $2
		)`, schema, table).Scan(&exists); err != nil {
		t.Fatalf("check relation exists error = %v", err)
	}
	return exists
}
