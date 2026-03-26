package postgres

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/aparcero/mcp-postgres/internal/config"
	"github.com/aparcero/mcp-postgres/internal/confirm"
)

func TestPoolConfigForDatabasePreservesConnectionOptions(t *testing.T) {
	cfg := config.Config{
		BaseDSN:           "postgresql://postgres:secret@localhost:5432/postgres?sslmode=disable&application_name=mcp-postgres",
		BootstrapDatabase: "postgres",
		PoolMaxConns:      3,
		DefaultSampleRows: 10,
		MaxSampleRows:     100,
	}

	manager, err := NewManager(cfg, nil)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	poolCfg, err := manager.poolConfigForDatabase("analytics_db")
	if err != nil {
		t.Fatalf("poolConfigForDatabase() error = %v", err)
	}

	if got, want := poolCfg.ConnConfig.Database, "analytics_db"; got != want {
		t.Fatalf("ConnConfig.Database = %q, want %q", got, want)
	}
	if got, want := poolCfg.ConnConfig.User, "postgres"; got != want {
		t.Fatalf("ConnConfig.User = %q, want %q", got, want)
	}
	if got, want := poolCfg.ConnConfig.Host, "localhost"; got != want {
		t.Fatalf("ConnConfig.Host = %q, want %q", got, want)
	}
	if got, want := poolCfg.ConnConfig.RuntimeParams["application_name"], "mcp-postgres"; got != want {
		t.Fatalf("application_name = %q, want %q", got, want)
	}
	if got, want := poolCfg.MaxConns, int32(3); got != want {
		t.Fatalf("MaxConns = %d, want %d", got, want)
	}
}

func TestPoolForDatabaseEnforcesMaxCachedPools(t *testing.T) {
	cfg := config.Config{
		BaseDSN:           "postgresql://postgres:secret@localhost:5432/postgres?sslmode=disable&application_name=mcp-postgres",
		BootstrapDatabase: "postgres",
		PoolMaxConns:      1,
		MaxCachedPools:    1,
		DefaultSampleRows: 10,
		MaxSampleRows:     100,
	}

	manager, err := NewManager(cfg, nil)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	defer manager.Close()

	ctx := context.Background()
	if _, err := manager.PoolForDatabase(ctx, "first_db"); err != nil {
		t.Fatalf("PoolForDatabase(first_db) error = %v", err)
	}

	if _, err := manager.PoolForDatabase(ctx, "second_db"); err == nil {
		t.Fatal("PoolForDatabase(second_db) error = nil, want max cached pools error")
	} else if !strings.Contains(err.Error(), "maximum cached database pools reached") {
		t.Fatalf("PoolForDatabase(second_db) error = %q, want max cached pools message", err.Error())
	}
}

func TestEvictIdlePoolsClosesNonBootstrapPools(t *testing.T) {
	cfg := config.Config{
		BaseDSN:           "postgresql://postgres:secret@localhost:5432/postgres?sslmode=disable&application_name=mcp-postgres",
		BootstrapDatabase: "postgres",
		PoolMaxConns:      1,
		MaxCachedPools:    4,
		IdlePoolTTL:       time.Second,
		DefaultSampleRows: 10,
		MaxSampleRows:     100,
	}

	manager, err := NewManager(cfg, nil)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	defer manager.Close()

	ctx := context.Background()
	if _, err := manager.PoolForDatabase(ctx, "analytics_db"); err != nil {
		t.Fatalf("PoolForDatabase(analytics_db) error = %v", err)
	}

	manager.mu.RLock()
	entry := manager.pools["analytics_db"]
	manager.mu.RUnlock()
	if entry == nil {
		t.Fatal("analytics_db pool entry is nil")
	}
	entry.touch(time.Now().UTC().Add(-2 * time.Second))

	manager.evictIdlePools(time.Now().UTC())

	manager.mu.RLock()
	_, ok := manager.pools["analytics_db"]
	manager.mu.RUnlock()
	if ok {
		t.Fatal("analytics_db pool is still cached after idle eviction")
	}
}

func TestGetServerMetricsIncludesPolicyAndPendingConfirmations(t *testing.T) {
	cfg := config.Config{
		BaseDSN:           "postgresql://postgres:secret@localhost:5432/postgres?sslmode=disable&application_name=mcp-postgres",
		BootstrapDatabase: "postgres",
		ConfirmationTTL:   2 * time.Minute,
		IdlePoolTTL:       10 * time.Minute,
		IdlePoolCleanup:   1 * time.Minute,
		DefaultSampleRows: 10,
		MaxSampleRows:     100,
	}

	manager, err := NewManager(cfg, nil)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	defer manager.Close()

	if _, err := manager.confirmations.Issue(confirm.Request{
		ToolName: "postgres.exec_admin",
		Database: "postgres",
		Mode:     "admin",
		SQLHash:  "hash",
	}); err != nil {
		t.Fatalf("Issue() error = %v", err)
	}

	first, err := manager.GetServerMetrics(context.Background())
	if err != nil {
		t.Fatalf("GetServerMetrics() first error = %v", err)
	}

	if got, want := first.Mode, "readonly"; got != want {
		t.Fatalf("Mode = %q, want %q", got, want)
	}
	if got, want := first.BootstrapDatabase, "postgres"; got != want {
		t.Fatalf("BootstrapDatabase = %q, want %q", got, want)
	}
	if got, want := first.PendingConfirmationTokens, 1; got != want {
		t.Fatalf("PendingConfirmationTokens = %d, want %d", got, want)
	}
	if got, want := first.IdlePoolTTLSeconds, int64(600); got != want {
		t.Fatalf("IdlePoolTTLSeconds = %d, want %d", got, want)
	}
	if got, want := first.IdlePoolCleanupIntervalSeconds, int64(60); got != want {
		t.Fatalf("IdlePoolCleanupIntervalSeconds = %d, want %d", got, want)
	}

	second, err := manager.GetServerMetrics(context.Background())
	if err != nil {
		t.Fatalf("GetServerMetrics() second error = %v", err)
	}
	if len(second.Operations) == 0 {
		t.Fatal("len(Operations) = 0, want at least 1")
	}
	if got, want := second.Operations[0].Tool, "postgres.get_server_metrics"; got != want {
		t.Fatalf("Operations[0].Tool = %q, want %q", got, want)
	}
	if got, want := second.Operations[0].Requests, int64(1); got != want {
		t.Fatalf("Operations[0].Requests = %d, want %d", got, want)
	}
}
