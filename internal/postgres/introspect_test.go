package postgres

import "testing"

func TestCountRowsQuerySortsFiltersAndHandlesNulls(t *testing.T) {
	query, args, err := countRowsQuery("public", "rules", map[string]any{
		"deleted_at": nil,
		"priority":   2000,
		"result": map[string]any{
			"cancellation_cost": []string{"free"},
		},
	})
	if err != nil {
		t.Fatalf("countRowsQuery() error = %v", err)
	}

	wantQuery := `select count(*) from "public"."rules" where "deleted_at" is null and to_jsonb("priority") = $1::jsonb and to_jsonb("result") = $2::jsonb`
	if query != wantQuery {
		t.Fatalf("countRowsQuery() query = %q, want %q", query, wantQuery)
	}

	if got, want := len(args), 2; got != want {
		t.Fatalf("len(args) = %d, want %d", got, want)
	}
	if got, want := args[0], "2000"; got != want {
		t.Fatalf("args[0] = %#v, want %#v", got, want)
	}
	if got, want := args[1], `{"cancellation_cost":["free"]}`; got != want {
		t.Fatalf("args[1] = %#v, want %#v", got, want)
	}
}

func TestNormalizeRelationInputRejectsEmptyNames(t *testing.T) {
	if _, _, err := normalizeRelationInput("", "rules"); err == nil {
		t.Fatal("normalizeRelationInput() error = nil, want non-nil for empty schema")
	}

	if _, _, err := normalizeRelationInput("public", ""); err == nil {
		t.Fatal("normalizeRelationInput() error = nil, want non-nil for empty table")
	}
}
