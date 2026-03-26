package postgres

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/aparcero/mcp-postgres/internal/policy"
)

func TestExecDMLIntegration(t *testing.T) {
	manager, database := integrationManager(t, policy.ModeOperator, []string{requireIntegrationDSN(t).database})

	ctx := context.Background()
	if err := manager.PingBootstrap(ctx); err != nil {
		t.Fatalf("PingBootstrap() error = %v", err)
	}

	pool := integrationPool(t, ctx, manager, database)
	schema := createIntegrationSchema(t, ctx, pool, "dml")
	table := "widgets"
	if _, err := pool.Exec(ctx, fmt.Sprintf(`create table %s (
		id integer primary key,
		note text,
		active boolean default false
	)`, integrationIdentifier(schema, table))); err != nil {
		t.Fatalf("create test table error = %v", err)
	}

	insertOut, err := manager.ExecDML(ctx, database, fmt.Sprintf(`insert into %s (id, note, active) values (1, 'first', true)`, integrationIdentifier(schema, table)), 5*time.Second)
	if err != nil {
		t.Fatalf("ExecDML(insert) error = %v", err)
	}
	if got, want := insertOut.RowsAffected, int64(1); got != want {
		t.Fatalf("insert RowsAffected = %d, want %d", got, want)
	}

	countOut, err := manager.CountRows(ctx, database, schema, table, map[string]any{"id": 1})
	if err != nil {
		t.Fatalf("CountRows() after insert error = %v", err)
	}
	if got, want := countOut.Count, int64(1); got != want {
		t.Fatalf("count after insert = %d, want %d", got, want)
	}

	updateOut, err := manager.ExecDML(ctx, database, fmt.Sprintf(`update %s set note = 'second' where id = 1`, integrationIdentifier(schema, table)), 5*time.Second)
	if err != nil {
		t.Fatalf("ExecDML(update) error = %v", err)
	}
	if got, want := updateOut.RowsAffected, int64(1); got != want {
		t.Fatalf("update RowsAffected = %d, want %d", got, want)
	}

	var note string
	if err := pool.QueryRow(ctx, fmt.Sprintf(`select note from %s where id = 1`, integrationIdentifier(schema, table))).Scan(&note); err != nil {
		t.Fatalf("verify update error = %v", err)
	}
	if got, want := note, "second"; got != want {
		t.Fatalf("updated note = %q, want %q", got, want)
	}

	mergeOut, err := manager.ExecDML(ctx, database, fmt.Sprintf(`merge into %s as target
		using (values (2, 'merged', true)) as source(id, note, active)
		on target.id = source.id
		when matched then update set note = source.note, active = source.active
		when not matched then insert (id, note, active) values (source.id, source.note, source.active)`, integrationIdentifier(schema, table)), 5*time.Second)
	if err != nil {
		t.Fatalf("ExecDML(merge) error = %v", err)
	}
	if got, want := mergeOut.RowsAffected, int64(1); got != want {
		t.Fatalf("merge RowsAffected = %d, want %d", got, want)
	}

	deleteOut, err := manager.ExecDML(ctx, database, fmt.Sprintf(`delete from %s where id = 1`, integrationIdentifier(schema, table)), 5*time.Second)
	if err != nil {
		t.Fatalf("ExecDML(delete) error = %v", err)
	}
	if got, want := deleteOut.RowsAffected, int64(1); got != want {
		t.Fatalf("delete RowsAffected = %d, want %d", got, want)
	}

	countOut, err = manager.CountRows(ctx, database, schema, table, nil)
	if err != nil {
		t.Fatalf("CountRows() after delete error = %v", err)
	}
	if got, want := countOut.Count, int64(1); got != want {
		t.Fatalf("count after delete = %d, want %d", got, want)
	}
}

func TestExecDMLPolicyIntegration(t *testing.T) {
	manager, database := integrationManager(t, policy.ModeOperator, []string{requireIntegrationDSN(t).database})

	ctx := context.Background()
	pool := integrationPool(t, ctx, manager, database)
	schema := createIntegrationSchema(t, ctx, pool, "dml_policy")
	table := "widgets"
	if _, err := pool.Exec(ctx, fmt.Sprintf(`create table %s (
		id integer primary key,
		note text
	)`, integrationIdentifier(schema, table))); err != nil {
		t.Fatalf("create test table error = %v", err)
	}
	if _, err := pool.Exec(ctx, fmt.Sprintf(`insert into %s (id, note) values (1, 'stable')`, integrationIdentifier(schema, table))); err != nil {
		t.Fatalf("insert fixture row error = %v", err)
	}

	testCases := []struct {
		name        string
		sql         string
		wantErrText string
	}{
		{
			name:        "unqualified target",
			sql:         "insert into widgets (id, note) values (2, 'blocked')",
			wantErrText: "schema-qualified targets",
		},
		{
			name:        "broad update",
			sql:         fmt.Sprintf(`update %s set note = 'blocked'`, integrationIdentifier(schema, table)),
			wantErrText: "WHERE clause",
		},
		{
			name:        "broad delete",
			sql:         fmt.Sprintf(`delete from %s`, integrationIdentifier(schema, table)),
			wantErrText: "WHERE clause",
		},
		{
			name:        "denied schema",
			sql:         "insert into pg_catalog.pg_type (oid) values (1)",
			wantErrText: `schema "pg_catalog" is denied`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := manager.ExecDML(ctx, database, tc.sql, 5*time.Second); err == nil {
				t.Fatalf("ExecDML(%s) error = nil, want non-nil", tc.name)
			} else if !strings.Contains(err.Error(), tc.wantErrText) {
				t.Fatalf("ExecDML(%s) error = %q, want substring %q", tc.name, err.Error(), tc.wantErrText)
			}
		})
	}

	countOut, err := manager.CountRows(ctx, database, schema, table, nil)
	if err != nil {
		t.Fatalf("CountRows() after rejected DML error = %v", err)
	}
	if got, want := countOut.Count, int64(1); got != want {
		t.Fatalf("count after rejected DML = %d, want %d", got, want)
	}

	noAllowlistManager, _ := integrationManager(t, policy.ModeOperator, nil)
	if _, err := noAllowlistManager.ExecDML(ctx, database, fmt.Sprintf(`insert into %s (id, note) values (3, 'blocked')`, integrationIdentifier(schema, table)), 5*time.Second); err == nil {
		t.Fatal("ExecDML(no allowlist) error = nil, want non-nil")
	} else if !strings.Contains(err.Error(), "no mutation databases are configured") {
		t.Fatalf("ExecDML(no allowlist) error = %q, want no mutation databases message", err.Error())
	}

	wrongAllowlistManager, _ := integrationManager(t, policy.ModeOperator, []string{"other_database"})
	if _, err := wrongAllowlistManager.ExecDML(ctx, database, fmt.Sprintf(`insert into %s (id, note) values (4, 'blocked')`, integrationIdentifier(schema, table)), 5*time.Second); err == nil {
		t.Fatal("ExecDML(wrong allowlist) error = nil, want non-nil")
	} else if !strings.Contains(err.Error(), "is not allowed for mutation") {
		t.Fatalf("ExecDML(wrong allowlist) error = %q, want mutation allowlist message", err.Error())
	}
}
