package policy

import (
	"strings"
	"testing"

	"github.com/aparcero/mcp-postgres/internal/sqlguard"
)

func TestAuthorizeQuery(t *testing.T) {
	p := New(ModeOperator, nil, []string{"app_db"})

	if err := p.AuthorizeQuery(sqlguard.Statement{Class: sqlguard.StatementClassReadOnly, Kind: "select"}); err != nil {
		t.Fatalf("AuthorizeQuery() error = %v, want nil", err)
	}

	err := p.AuthorizeQuery(sqlguard.Statement{Class: sqlguard.StatementClassMutating, Kind: "insert"})
	if err == nil {
		t.Fatal("AuthorizeQuery() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "read-only") {
		t.Fatalf("AuthorizeQuery() error = %q, want read-only message", err.Error())
	}
}

func TestAuthorizeExec(t *testing.T) {
	testCases := []struct {
		name        string
		policy      Policy
		database    string
		statement   sqlguard.Statement
		wantErrText string
	}{
		{
			name:     "readonly denies mutating",
			policy:   New(ModeReadOnly, nil, []string{"app_db"}),
			database: "app_db",
			statement: sqlguard.Statement{
				Class:                 sqlguard.StatementClassMutating,
				Kind:                  "insert",
				TargetSchemas:         []string{"public"},
				HasUnqualifiedTargets: false,
			},
			wantErrText: "mutating statements are not allowed",
		},
		{
			name:     "operator allows qualified mutating",
			policy:   New(ModeOperator, nil, []string{"app_db"}),
			database: "app_db",
			statement: sqlguard.Statement{
				Class:                 sqlguard.StatementClassMutating,
				Kind:                  "insert",
				TargetSchemas:         []string{"public"},
				HasUnqualifiedTargets: false,
			},
		},
		{
			name:     "operator rejects broad update",
			policy:   New(ModeOperator, nil, []string{"app_db"}),
			database: "app_db",
			statement: sqlguard.Statement{
				Class:            sqlguard.StatementClassMutating,
				Kind:             "update",
				TargetSchemas:    []string{"public"},
				HasBroadMutation: true,
			},
			wantErrText: "UPDATE and DELETE statements must include a WHERE clause",
		},
		{
			name:     "operator rejects destructive",
			policy:   New(ModeOperator, nil, []string{"app_db"}),
			database: "app_db",
			statement: sqlguard.Statement{
				Class:                 sqlguard.StatementClassDestructive,
				Kind:                  "drop",
				TargetSchemas:         []string{"public"},
				HasUnqualifiedTargets: false,
			},
			wantErrText: "only allowed in admin mode",
		},
		{
			name:     "admin allows destructive",
			policy:   New(ModeAdmin, nil, []string{"app_db"}),
			database: "app_db",
			statement: sqlguard.Statement{
				Class:                 sqlguard.StatementClassDestructive,
				Kind:                  "drop",
				TargetSchemas:         []string{"public"},
				HasUnqualifiedTargets: false,
			},
		},
		{
			name:     "denied schema blocked",
			policy:   New(ModeAdmin, []string{"pg_catalog"}, []string{"app_db"}),
			database: "app_db",
			statement: sqlguard.Statement{
				Class:                 sqlguard.StatementClassMutating,
				Kind:                  "update",
				TargetSchemas:         []string{"pg_catalog"},
				HasUnqualifiedTargets: false,
			},
			wantErrText: `schema "pg_catalog" is denied`,
		},
		{
			name:     "unqualified target blocked",
			policy:   New(ModeAdmin, nil, []string{"app_db"}),
			database: "app_db",
			statement: sqlguard.Statement{
				Class:                 sqlguard.StatementClassMutating,
				Kind:                  "delete",
				HasUnqualifiedTargets: true,
			},
			wantErrText: "schema-qualified targets",
		},
		{
			name:     "session-changing always blocked",
			policy:   New(ModeAdmin, nil, []string{"app_db"}),
			database: "app_db",
			statement: sqlguard.Statement{
				Class:  sqlguard.StatementClassSessionChanging,
				Kind:   "set",
				Reason: "session settings are not allowed",
			},
			wantErrText: "session-changing statements are not allowed",
		},
		{
			name:     "mutation database allowlist enforced",
			policy:   New(ModeOperator, nil, []string{"app_db"}),
			database: "other_db",
			statement: sqlguard.Statement{
				Class:                 sqlguard.StatementClassMutating,
				Kind:                  "update",
				TargetSchemas:         []string{"public"},
				HasUnqualifiedTargets: false,
			},
			wantErrText: `database "other_db" is not allowed for mutation`,
		},
		{
			name:     "no mutation databases configured",
			policy:   New(ModeOperator, nil, nil),
			database: "app_db",
			statement: sqlguard.Statement{
				Class:                 sqlguard.StatementClassMutating,
				Kind:                  "update",
				TargetSchemas:         []string{"public"},
				HasUnqualifiedTargets: false,
			},
			wantErrText: "no mutation databases are configured",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.policy.AuthorizeExec(tc.database, tc.statement)
			if tc.wantErrText == "" {
				if err != nil {
					t.Fatalf("AuthorizeExec() error = %v, want nil", err)
				}
				return
			}

			if err == nil {
				t.Fatalf("AuthorizeExec() error = nil, want substring %q", tc.wantErrText)
			}
			if !strings.Contains(err.Error(), tc.wantErrText) {
				t.Fatalf("AuthorizeExec() error = %q, want substring %q", err.Error(), tc.wantErrText)
			}
		})
	}
}

func TestAuthorizeDMLRejectsNonMutatingStatements(t *testing.T) {
	p := New(ModeAdmin, nil, []string{"app_db"})

	err := p.AuthorizeDML("app_db", sqlguard.Statement{
		Class: sqlguard.StatementClassReadOnly,
		Kind:  "select",
	})
	if err == nil {
		t.Fatal("AuthorizeDML() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "DML tool only accepts") {
		t.Fatalf("AuthorizeDML() error = %q, want DML-specific message", err.Error())
	}
}

func TestAuthorizeAdmin(t *testing.T) {
	testCases := []struct {
		name        string
		policy      Policy
		database    string
		statement   sqlguard.Statement
		wantErrText string
	}{
		{
			name:     "admin allows administrative",
			policy:   New(ModeAdmin, nil, []string{"app_db"}),
			database: "app_db",
			statement: sqlguard.Statement{
				Class:                 sqlguard.StatementClassAdministrative,
				Kind:                  "create",
				TargetSchemas:         []string{"public"},
				HasUnqualifiedTargets: false,
			},
		},
		{
			name:        "operator rejects administrative",
			policy:      New(ModeOperator, nil, []string{"app_db"}),
			database:    "app_db",
			statement:   sqlguard.Statement{Class: sqlguard.StatementClassAdministrative, Kind: "create", TargetSchemas: []string{"public"}},
			wantErrText: "only allowed in admin mode",
		},
		{
			name:     "admin allows privilege changing",
			policy:   New(ModeAdmin, nil, []string{"app_db"}),
			database: "app_db",
			statement: sqlguard.Statement{
				Class:                 sqlguard.StatementClassPrivilegeChanging,
				Kind:                  "grant",
				TargetSchemas:         []string{"public"},
				HasUnqualifiedTargets: false,
			},
		},
		{
			name:        "admin rejects dml through admin tool",
			policy:      New(ModeAdmin, nil, []string{"app_db"}),
			database:    "app_db",
			statement:   sqlguard.Statement{Class: sqlguard.StatementClassMutating, Kind: "update", TargetSchemas: []string{"public"}},
			wantErrText: "does not accept DML",
		},
		{
			name:        "admin rejects query through admin tool",
			policy:      New(ModeAdmin, nil, []string{"app_db"}),
			database:    "app_db",
			statement:   sqlguard.Statement{Class: sqlguard.StatementClassReadOnly, Kind: "select"},
			wantErrText: "does not accept read-only",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.policy.AuthorizeAdmin(tc.database, tc.statement)
			if tc.wantErrText == "" {
				if err != nil {
					t.Fatalf("AuthorizeAdmin() error = %v, want nil", err)
				}
				return
			}

			if err == nil {
				t.Fatalf("AuthorizeAdmin() error = nil, want substring %q", tc.wantErrText)
			}
			if !strings.Contains(err.Error(), tc.wantErrText) {
				t.Fatalf("AuthorizeAdmin() error = %q, want substring %q", err.Error(), tc.wantErrText)
			}
		})
	}
}
