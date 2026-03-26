package mcpserver

import (
	"context"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/aparcero/mcp-postgres/internal/config"
	"github.com/aparcero/mcp-postgres/internal/policy"
)

func TestRegisterToolsUsesModeCapabilities(t *testing.T) {
	testCases := []struct {
		name string
		mode policy.Mode
		want []string
	}{
		{
			name: "readonly",
			mode: policy.ModeReadOnly,
			want: []string{
				"postgres.list_databases",
				"postgres.get_connection_status",
				"postgres.get_server_metrics",
				"postgres.list_schemas",
				"postgres.list_tables",
				"postgres.describe_table",
				"postgres.sample_table",
				"postgres.query",
				"postgres.count_rows",
			},
		},
		{
			name: "operator",
			mode: policy.ModeOperator,
			want: []string{
				"postgres.list_databases",
				"postgres.get_connection_status",
				"postgres.get_server_metrics",
				"postgres.list_schemas",
				"postgres.list_tables",
				"postgres.describe_table",
				"postgres.sample_table",
				"postgres.query",
				"postgres.exec_dml",
				"postgres.count_rows",
			},
		},
		{
			name: "admin",
			mode: policy.ModeAdmin,
			want: []string{
				"postgres.list_databases",
				"postgres.get_connection_status",
				"postgres.get_server_metrics",
				"postgres.list_schemas",
				"postgres.list_tables",
				"postgres.describe_table",
				"postgres.sample_table",
				"postgres.query",
				"postgres.exec_dml",
				"postgres.exec_admin",
				"postgres.count_rows",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := registeredToolNames(t, tc.mode)
			want := append([]string(nil), tc.want...)
			sort.Strings(got)
			sort.Strings(want)

			if !reflect.DeepEqual(got, want) {
				t.Fatalf("registered tools = %#v, want %#v", got, want)
			}
		})
	}
}

func TestToolInputDefaultsAndCaps(t *testing.T) {
	cfg := config.Config{
		QueryTimeout:      5 * time.Second,
		ExecTimeout:       7 * time.Second,
		DefaultMaxRows:    25,
		MaxMaxRows:        100,
		DefaultSampleRows: 10,
		MaxSampleRows:     50,
	}

	if got, want := queryMaxRows(cfg, 0), 25; got != want {
		t.Fatalf("queryMaxRows(default) = %d, want %d", got, want)
	}
	if got, want := queryMaxRows(cfg, 250), 100; got != want {
		t.Fatalf("queryMaxRows(cap) = %d, want %d", got, want)
	}
	if got, want := queryMaxRows(cfg, 12), 12; got != want {
		t.Fatalf("queryMaxRows(requested) = %d, want %d", got, want)
	}

	if got, want := sampleLimit(cfg, 0), 10; got != want {
		t.Fatalf("sampleLimit(default) = %d, want %d", got, want)
	}
	if got, want := sampleLimit(cfg, 500), 50; got != want {
		t.Fatalf("sampleLimit(cap) = %d, want %d", got, want)
	}
	if got, want := sampleLimit(cfg, 8), 8; got != want {
		t.Fatalf("sampleLimit(requested) = %d, want %d", got, want)
	}

	if got, want := toolTimeout(0, cfg.QueryTimeout), 5*time.Second; got != want {
		t.Fatalf("toolTimeout(default) = %s, want %s", got, want)
	}
	if got, want := toolTimeout(125, cfg.ExecTimeout), 125*time.Millisecond; got != want {
		t.Fatalf("toolTimeout(requested) = %s, want %s", got, want)
	}
}

func registeredToolNames(t *testing.T, mode policy.Mode) []string {
	t.Helper()

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "mcp-postgres-test",
		Version: "test",
	}, nil)
	registerTools(server, nil, config.Config{
		Mode:              mode,
		QueryTimeout:      time.Second,
		ExecTimeout:       time.Second,
		DefaultMaxRows:    10,
		MaxMaxRows:        100,
		DefaultSampleRows: 5,
		MaxSampleRows:     10,
	})

	ctx := context.Background()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server.Connect() error = %v", err)
	}

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "mcp-client-test",
		Version: "test",
	}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		_ = serverSession.Close()
		t.Fatalf("client.Connect() error = %v", err)
	}
	defer func() {
		_ = clientSession.Close()
		_ = serverSession.Wait()
	}()

	out, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}

	names := make([]string, 0, len(out.Tools))
	for _, tool := range out.Tools {
		names = append(names, tool.Name)
	}
	return names
}
