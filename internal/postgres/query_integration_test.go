package postgres

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/aparcero/mcp-postgres/internal/policy"
)

func TestReadonlyIntegration(t *testing.T) {
	manager, database := integrationManager(t, policy.ModeReadOnly, nil)

	ctx := context.Background()
	if err := manager.PingBootstrap(ctx); err != nil {
		t.Fatalf("PingBootstrap() error = %v", err)
	}

	pool := integrationPool(t, ctx, manager, database)
	schema := createIntegrationSchema(t, ctx, pool, "readonly")
	table := "widgets"
	view := "active_widgets"

	if _, err := pool.Exec(ctx, fmt.Sprintf(`create table %s (
		id integer primary key,
		status text not null,
		payload jsonb not null,
		created_at timestamptz not null,
		note text,
		deleted_at timestamptz,
		constraint widgets_status_check check (status <> '')
	)`, integrationIdentifier(schema, table))); err != nil {
		t.Fatalf("create test table error = %v", err)
	}
	if _, err := pool.Exec(ctx, fmt.Sprintf(`create index widgets_status_idx on %s (status)`, integrationIdentifier(schema, table))); err != nil {
		t.Fatalf("create test index error = %v", err)
	}
	if _, err := pool.Exec(ctx, fmt.Sprintf(`insert into %s (id, status, payload, created_at, note, deleted_at) values
		(1, 'pending', '{"source":"seed"}', '2026-03-26T00:00:00Z', null, null),
		(2, 'done', '{"source":"api"}', '2026-03-26T01:00:00Z', 'ready', '2026-03-27T00:00:00Z'),
		(3, 'pending', '{"source":"api"}', '2026-03-26T02:00:00Z', 'queued', null)`, integrationIdentifier(schema, table))); err != nil {
		t.Fatalf("insert fixture rows error = %v", err)
	}
	if _, err := pool.Exec(ctx, fmt.Sprintf(`create view %s as select id, status from %s where deleted_at is null`, integrationIdentifier(schema, view), integrationIdentifier(schema, table))); err != nil {
		t.Fatalf("create test view error = %v", err)
	}

	statusOut, err := manager.GetConnectionStatus(ctx)
	if err != nil {
		t.Fatalf("GetConnectionStatus() error = %v", err)
	}
	if !statusOut.Connected {
		t.Fatal("GetConnectionStatus().Connected = false, want true")
	}
	if got, want := statusOut.Database, database; got != want {
		t.Fatalf("GetConnectionStatus().Database = %q, want %q", got, want)
	}
	if got, want := statusOut.Mode, string(policy.ModeReadOnly); got != want {
		t.Fatalf("GetConnectionStatus().Mode = %q, want %q", got, want)
	}

	databasesOut, err := manager.ListDatabases(ctx)
	if err != nil {
		t.Fatalf("ListDatabases() error = %v", err)
	}
	if !databaseListed(databasesOut.Databases, database) {
		t.Fatalf("ListDatabases() did not include %q", database)
	}

	schemasOut, err := manager.ListSchemas(ctx, database)
	if err != nil {
		t.Fatalf("ListSchemas() error = %v", err)
	}
	if !schemaListed(schemasOut.Schemas, schema) {
		t.Fatalf("ListSchemas() did not include %q", schema)
	}

	tablesOut, err := manager.ListTables(ctx, database, schema)
	if err != nil {
		t.Fatalf("ListTables() error = %v", err)
	}
	if !tableListed(tablesOut.Tables, schema, table) {
		t.Fatalf("ListTables() did not include table %s.%s", schema, table)
	}
	if !tableListed(tablesOut.Tables, schema, view) {
		t.Fatalf("ListTables() did not include view %s.%s", schema, view)
	}

	describeOut, err := manager.DescribeTable(ctx, database, schema, table)
	if err != nil {
		t.Fatalf("DescribeTable() error = %v", err)
	}
	if !columnListed(describeOut.Columns, "payload") {
		t.Fatalf("DescribeTable().Columns did not include payload")
	}
	if len(describeOut.Indexes) == 0 {
		t.Fatal("len(DescribeTable().Indexes) = 0, want at least 1")
	}
	if len(describeOut.Constraints) == 0 {
		t.Fatal("len(DescribeTable().Constraints) = 0, want at least 1")
	}

	sampleOut, err := manager.SampleTable(ctx, database, schema, table, 2)
	if err != nil {
		t.Fatalf("SampleTable() error = %v", err)
	}
	if got, want := sampleOut.RowCount, 2; got != want {
		t.Fatalf("SampleTable().RowCount = %d, want %d", got, want)
	}
	if !sampleOut.Truncated {
		t.Fatal("SampleTable().Truncated = false, want true")
	}

	countOut, err := manager.CountRows(ctx, database, schema, table, map[string]any{
		"status":     "pending",
		"deleted_at": nil,
	})
	if err != nil {
		t.Fatalf("CountRows() error = %v", err)
	}
	if got, want := countOut.Count, int64(2); got != want {
		t.Fatalf("CountRows().Count = %d, want %d", got, want)
	}

	querySQL := fmt.Sprintf(`select id, payload, created_at, note from %s order by id`, integrationIdentifier(schema, table))
	queryOut, err := manager.Query(ctx, database, querySQL, 1, 5*time.Second)
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if got, want := queryOut.RowCount, 1; got != want {
		t.Fatalf("Query().RowCount = %d, want %d", got, want)
	}
	if !queryOut.Truncated {
		t.Fatal("Query().Truncated = false, want true")
	}
	payload, ok := queryOut.Rows[0]["payload"].(map[string]any)
	if !ok {
		t.Fatalf("payload type = %T, want map[string]any", queryOut.Rows[0]["payload"])
	}
	if got, want := payload["source"], "seed"; got != want {
		t.Fatalf("payload[source] = %v, want %q", got, want)
	}
	if got := queryOut.Rows[0]["created_at"]; got == nil || got == "" {
		t.Fatalf("created_at = %v, want non-empty string", got)
	}
	if got := queryOut.Rows[0]["note"]; got != nil {
		t.Fatalf("note = %v, want nil", got)
	}

	readOnlyOut, err := manager.Query(ctx, database, "select current_setting('transaction_read_only') as transaction_read_only", 1, 5*time.Second)
	if err != nil {
		t.Fatalf("Query(transaction_read_only) error = %v", err)
	}
	if got, want := readOnlyOut.Rows[0]["transaction_read_only"], "on"; got != want {
		t.Fatalf("transaction_read_only = %v, want %q", got, want)
	}

	if _, err := manager.Query(ctx, database, fmt.Sprintf(`insert into %s (id, status, payload, created_at) values (4, 'blocked', '{}', now())`, integrationIdentifier(schema, table)), 1, 5*time.Second); err == nil {
		t.Fatal("Query(mutating SQL) error = nil, want non-nil")
	} else if !strings.Contains(err.Error(), "read-only") {
		t.Fatalf("Query(mutating SQL) error = %q, want read-only message", err.Error())
	}

	countAfterRejected, err := manager.CountRows(ctx, database, schema, table, nil)
	if err != nil {
		t.Fatalf("CountRows() after rejected query error = %v", err)
	}
	if got, want := countAfterRejected.Count, int64(3); got != want {
		t.Fatalf("count after rejected query = %d, want %d", got, want)
	}

	if _, err := manager.Query(ctx, database, "select pg_sleep(1)", 1, 10*time.Millisecond); err == nil {
		t.Fatal("Query(pg_sleep) error = nil, want timeout")
	} else if !strings.Contains(err.Error(), "timed out") && !strings.Contains(err.Error(), "canceled") {
		t.Fatalf("Query(pg_sleep) error = %q, want timeout/cancellation message", err.Error())
	}
}
