package sqlguard

import (
	"strings"
	"testing"
)

func TestParseAndClassify(t *testing.T) {
	testCases := []struct {
		name                string
		sql                 string
		wantClass           StatementClass
		wantKind            string
		wantSchemas         []string
		wantUnqualified     bool
		wantBroadMutation   bool
		wantReasonSubstring string
		wantErrSubstring    string
	}{
		{
			name:      "select",
			sql:       "select 1",
			wantClass: StatementClassReadOnly,
			wantKind:  "select",
		},
		{
			name:      "show",
			sql:       "show search_path",
			wantClass: StatementClassReadOnly,
			wantKind:  "show",
		},
		{
			name:      "commented select with semicolon",
			sql:       "/* lead */ select 1;",
			wantClass: StatementClassReadOnly,
			wantKind:  "select",
		},
		{
			name:      "with mutating cte",
			sql:       "with moved as (delete from public.widgets returning *) select * from moved",
			wantClass: StatementClassMutating,
			wantKind:  "delete",
			wantSchemas: []string{
				"public",
			},
			wantBroadMutation: true,
		},
		{
			name:                "explain analyze",
			sql:                 "explain analyze select 1",
			wantClass:           StatementClassUnsupported,
			wantKind:            "explain",
			wantReasonSubstring: "EXPLAIN ANALYZE",
		},
		{
			name:      "explain analyze false",
			sql:       "explain (analyze false) select 1",
			wantClass: StatementClassReadOnly,
			wantKind:  "explain",
		},
		{
			name:             "multi statement rejected",
			sql:              "select 1; select 2",
			wantErrSubstring: "exactly one SQL statement is allowed",
		},
		{
			name:      "insert qualified schema",
			sql:       "insert into public.widgets(id) values (1)",
			wantClass: StatementClassMutating,
			wantKind:  "insert",
			wantSchemas: []string{
				"public",
			},
		},
		{
			name:            "delete unqualified schema",
			sql:             "delete from widgets where id = 1",
			wantClass:       StatementClassMutating,
			wantKind:        "delete",
			wantUnqualified: true,
		},
		{
			name:              "update without where is broad mutation",
			sql:               "update public.widgets set active = false",
			wantClass:         StatementClassMutating,
			wantKind:          "update",
			wantSchemas:       []string{"public"},
			wantBroadMutation: true,
		},
		{
			name:      "update with where is scoped mutation",
			sql:       "update public.widgets set active = false where id = 1",
			wantClass: StatementClassMutating,
			wantKind:  "update",
			wantSchemas: []string{
				"public",
			},
		},
		{
			name:      "merge qualified schema",
			sql:       "merge into public.widgets as target using (values (1, true)) as source(id, active) on target.id = source.id when matched then update set active = source.active",
			wantClass: StatementClassMutating,
			wantKind:  "merge",
			wantSchemas: []string{
				"public",
			},
		},
		{
			name:              "delete without where inside cte is broad mutation",
			sql:               "with moved as (delete from public.widgets returning *) insert into public.archive select * from moved",
			wantClass:         StatementClassMutating,
			wantKind:          "insert",
			wantSchemas:       []string{"public"},
			wantBroadMutation: true,
		},
		{
			name:      "drop table",
			sql:       "drop table public.widgets",
			wantClass: StatementClassDestructive,
			wantKind:  "drop",
			wantSchemas: []string{
				"public",
			},
		},
		{
			name:      "grant table",
			sql:       "grant select on table public.widgets to app_user",
			wantClass: StatementClassPrivilegeChanging,
			wantKind:  "grant",
			wantSchemas: []string{
				"public",
			},
		},
		{
			name:      "revoke table",
			sql:       "revoke select on table public.widgets from app_user",
			wantClass: StatementClassPrivilegeChanging,
			wantKind:  "revoke",
			wantSchemas: []string{
				"public",
			},
		},
		{
			name:                "select for update",
			sql:                 "select * from public.widgets for update",
			wantClass:           StatementClassSessionChanging,
			wantKind:            "select",
			wantReasonSubstring: "locking clauses",
		},
		{
			name:      "alter table add column administrative",
			sql:       "alter table public.widgets add column note text",
			wantClass: StatementClassAdministrative,
			wantKind:  "alter_table",
			wantSchemas: []string{
				"public",
			},
		},
		{
			name:      "alter table drop column destructive",
			sql:       "alter table public.widgets drop column note",
			wantClass: StatementClassDestructive,
			wantKind:  "alter_table",
			wantSchemas: []string{
				"public",
			},
		},
		{
			name:                "set session",
			sql:                 "set search_path = public",
			wantClass:           StatementClassSessionChanging,
			wantKind:            "set",
			wantReasonSubstring: "session settings",
		},
		{
			name:      "create table administrative",
			sql:       "create table public.widgets (id integer primary key)",
			wantClass: StatementClassAdministrative,
			wantKind:  "create",
			wantSchemas: []string{
				"public",
			},
		},
		{
			name:      "create schema administrative",
			sql:       "create schema ops",
			wantClass: StatementClassAdministrative,
			wantKind:  "create_schema",
			wantSchemas: []string{
				"ops",
			},
		},
		{
			name:      "create index administrative",
			sql:       "create index widgets_note_idx on public.widgets (id)",
			wantClass: StatementClassAdministrative,
			wantKind:  "create_index",
			wantSchemas: []string{
				"public",
			},
		},
		{
			name:                "do block unsupported",
			sql:                 "do $$ begin perform 1; end $$",
			wantClass:           StatementClassUnsupported,
			wantKind:            "unsupported",
			wantReasonSubstring: "statement type is not supported",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			statement, err := ParseAndClassify(tc.sql)
			if tc.wantErrSubstring != "" {
				if err == nil {
					t.Fatalf("ParseAndClassify() error = nil, want substring %q", tc.wantErrSubstring)
				}
				if !strings.Contains(err.Error(), tc.wantErrSubstring) {
					t.Fatalf("ParseAndClassify() error = %q, want substring %q", err.Error(), tc.wantErrSubstring)
				}
				return
			}

			if err != nil {
				t.Fatalf("ParseAndClassify() error = %v", err)
			}
			if got, want := statement.Class, tc.wantClass; got != want {
				t.Fatalf("statement.Class = %q, want %q", got, want)
			}
			if got, want := statement.Kind, tc.wantKind; got != want {
				t.Fatalf("statement.Kind = %q, want %q", got, want)
			}
			if got, want := statement.HasUnqualifiedTargets, tc.wantUnqualified; got != want {
				t.Fatalf("statement.HasUnqualifiedTargets = %v, want %v", got, want)
			}
			if got, want := statement.HasBroadMutation, tc.wantBroadMutation; got != want {
				t.Fatalf("statement.HasBroadMutation = %v, want %v", got, want)
			}
			if len(statement.TargetSchemas) != len(tc.wantSchemas) {
				t.Fatalf("len(statement.TargetSchemas) = %d, want %d", len(statement.TargetSchemas), len(tc.wantSchemas))
			}
			for index, want := range tc.wantSchemas {
				if got := statement.TargetSchemas[index]; got != want {
					t.Fatalf("statement.TargetSchemas[%d] = %q, want %q", index, got, want)
				}
			}
			if tc.wantReasonSubstring != "" && !strings.Contains(statement.Reason, tc.wantReasonSubstring) {
				t.Fatalf("statement.Reason = %q, want substring %q", statement.Reason, tc.wantReasonSubstring)
			}
		})
	}
}

func TestRequiresConfirmation(t *testing.T) {
	testCases := []struct {
		name      string
		statement Statement
		want      bool
	}{
		{
			name:      "destructive",
			statement: Statement{Class: StatementClassDestructive},
			want:      true,
		},
		{
			name:      "privilege changing",
			statement: Statement{Class: StatementClassPrivilegeChanging},
			want:      true,
		},
		{
			name:      "administrative",
			statement: Statement{Class: StatementClassAdministrative},
		},
		{
			name:      "mutating",
			statement: Statement{Class: StatementClassMutating},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := RequiresConfirmation(tc.statement); got != tc.want {
				t.Fatalf("RequiresConfirmation() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestNormalizedHash(t *testing.T) {
	first, err := NormalizedHash("select 1")
	if err != nil {
		t.Fatalf("NormalizedHash() error = %v", err)
	}

	second, err := NormalizedHash("  SELECT   1 ; ")
	if err != nil {
		t.Fatalf("NormalizedHash() second error = %v", err)
	}

	if first != second {
		t.Fatalf("NormalizedHash() = %q and %q, want equal hashes", first, second)
	}
}
