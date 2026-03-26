package postgres

import (
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestQueryTxOptionsAreReadOnly(t *testing.T) {
	options := queryTxOptions()

	if got, want := options.AccessMode, pgx.ReadOnly; got != want {
		t.Fatalf("AccessMode = %q, want %q", got, want)
	}
}
