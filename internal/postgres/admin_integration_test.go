package postgres

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/aparcero/mcp-postgres/internal/policy"
)

func TestExecAdminIntegration(t *testing.T) {
	manager, database := integrationManager(t, policy.ModeAdmin, []string{requireIntegrationDSN(t).database})

	ctx := context.Background()
	if err := manager.PingBootstrap(ctx); err != nil {
		t.Fatalf("PingBootstrap() error = %v", err)
	}

	pool := integrationPool(t, ctx, manager, database)
	schema := uniqueIntegrationName("admin")
	table := "widgets"
	dropTable := "old_widgets"
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), fmt.Sprintf(`drop schema if exists %s cascade`, integrationIdentifier(schema)))
	})

	createSchemaOut, err := manager.ExecAdmin(ctx, database, fmt.Sprintf(`create schema %s`, integrationIdentifier(schema)), 5*time.Second, "")
	if err != nil {
		t.Fatalf("ExecAdmin(create schema) error = %v", err)
	}
	if got, want := createSchemaOut.Status, "committed"; got != want {
		t.Fatalf("create schema status = %q, want %q", got, want)
	}

	createTableSQL := fmt.Sprintf(`create table %s (
		id integer primary key,
		note text,
		constraint widgets_note_check check (note is null or note <> '')
	)`, integrationIdentifier(schema, table))
	createTableOut, err := manager.ExecAdmin(ctx, database, createTableSQL, 5*time.Second, "")
	if err != nil {
		t.Fatalf("ExecAdmin(create table) error = %v", err)
	}
	if got, want := createTableOut.Status, "committed"; got != want {
		t.Fatalf("create table status = %q, want %q", got, want)
	}

	describeOut, err := manager.DescribeTable(ctx, database, schema, table)
	if err != nil {
		t.Fatalf("DescribeTable() error = %v", err)
	}
	if len(describeOut.Columns) != 2 {
		t.Fatalf("len(DescribeTable().Columns) = %d, want 2", len(describeOut.Columns))
	}
	if len(describeOut.Constraints) == 0 {
		t.Fatal("len(DescribeTable().Constraints) = 0, want at least 1")
	}

	grantSQL := fmt.Sprintf(`grant select on table %s to public`, integrationIdentifier(schema, table))
	grantPendingOut, err := manager.ExecAdmin(ctx, database, grantSQL, 5*time.Second, "")
	if err != nil {
		t.Fatalf("ExecAdmin(grant pending) error = %v", err)
	}
	if got, want := grantPendingOut.Status, "confirmation_required"; got != want {
		t.Fatalf("grant pending status = %q, want %q", got, want)
	}
	if grantPendingOut.ConfirmationToken == "" {
		t.Fatal("grant pending confirmation token is empty")
	}
	grantOut, err := manager.ExecAdmin(ctx, database, grantSQL, 5*time.Second, grantPendingOut.ConfirmationToken)
	if err != nil {
		t.Fatalf("ExecAdmin(grant confirmed) error = %v", err)
	}
	if got, want := grantOut.Status, "committed"; got != want {
		t.Fatalf("grant status = %q, want %q", got, want)
	}

	createDropTableSQL := fmt.Sprintf(`create table %s (id integer primary key)`, integrationIdentifier(schema, dropTable))
	if _, err := manager.ExecAdmin(ctx, database, createDropTableSQL, 5*time.Second, ""); err != nil {
		t.Fatalf("ExecAdmin(create old table) error = %v", err)
	}

	dropTableSQL := fmt.Sprintf(`drop table %s`, integrationIdentifier(schema, dropTable))
	pendingOut, err := manager.ExecAdmin(ctx, database, dropTableSQL, 5*time.Second, "")
	if err != nil {
		t.Fatalf("ExecAdmin(drop table pending) error = %v", err)
	}
	if got, want := pendingOut.Status, "confirmation_required"; got != want {
		t.Fatalf("pending status = %q, want %q", got, want)
	}
	if pendingOut.ConfirmationToken == "" {
		t.Fatal("pending confirmation token is empty")
	}
	if !relationExists(t, ctx, pool, schema, dropTable) {
		t.Fatal("drop target does not exist after pending confirmation")
	}

	if _, err := manager.ExecAdmin(ctx, database, dropTableSQL, 5*time.Second, "bad-token"); err == nil {
		t.Fatal("ExecAdmin(drop table) invalid token error = nil, want non-nil")
	}
	if !relationExists(t, ctx, pool, schema, dropTable) {
		t.Fatal("drop target does not exist after invalid token")
	}

	dropOut, err := manager.ExecAdmin(ctx, database, dropTableSQL, 5*time.Second, pendingOut.ConfirmationToken)
	if err != nil {
		t.Fatalf("ExecAdmin(drop table confirmed) error = %v", err)
	}
	if got, want := dropOut.Status, "committed"; got != want {
		t.Fatalf("drop status = %q, want %q", got, want)
	}
	if relationExists(t, ctx, pool, schema, dropTable) {
		t.Fatal("drop target still exists after valid token")
	}

	if _, err := manager.ExecAdmin(ctx, database, dropTableSQL, 5*time.Second, pendingOut.ConfirmationToken); err == nil {
		t.Fatal("ExecAdmin(drop table) reused token error = nil, want non-nil")
	}
}

func TestExecAdminDeniedSchemaIntegration(t *testing.T) {
	manager, database := integrationManager(t, policy.ModeAdmin, []string{requireIntegrationDSN(t).database})

	ctx := context.Background()
	if _, err := manager.ExecAdmin(ctx, database, "create table pg_catalog.codex_blocked (id integer)", 5*time.Second, ""); err == nil {
		t.Fatal("ExecAdmin(denied schema) error = nil, want non-nil")
	} else if !strings.Contains(err.Error(), `schema "pg_catalog" is denied`) {
		t.Fatalf("ExecAdmin(denied schema) error = %q, want denied schema message", err.Error())
	}
}
