package postgres

import (
	"encoding/base64"
	"math/big"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestBuildQueryColumnsDeduplicatesNames(t *testing.T) {
	typeMap := pgtype.NewMap()
	columns := buildQueryColumns(typeMap, []pgconn.FieldDescription{
		{Name: "id", DataTypeOID: pgtype.Int8OID},
		{Name: "id", DataTypeOID: pgtype.TextOID},
		{Name: "", DataTypeOID: pgtype.TextOID},
	})

	if got, want := columns[0].Name, "id"; got != want {
		t.Fatalf("columns[0].Name = %q, want %q", got, want)
	}
	if got, want := columns[1].Name, "id_2"; got != want {
		t.Fatalf("columns[1].Name = %q, want %q", got, want)
	}
	if got, want := columns[1].SourceName, "id"; got != want {
		t.Fatalf("columns[1].SourceName = %q, want %q", got, want)
	}
	if got, want := columns[2].Name, "column_3"; got != want {
		t.Fatalf("columns[2].Name = %q, want %q", got, want)
	}
}

func TestNormalizeQueryValueJSON(t *testing.T) {
	value, err := normalizeQueryValue([]byte(`{"active":true,"roles":["ops"]}`), pgtype.JSONBOID)
	if err != nil {
		t.Fatalf("normalizeQueryValue() error = %v", err)
	}

	decoded, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("normalizeQueryValue() type = %T, want map[string]any", value)
	}
	if got, want := decoded["active"], true; got != want {
		t.Fatalf("decoded[active] = %#v, want %#v", got, want)
	}
}

func TestNormalizeQueryValueNumericUsesString(t *testing.T) {
	value, err := normalizeQueryValue(pgtype.Numeric{
		Int:   big.NewInt(12345),
		Exp:   -2,
		Valid: true,
	}, pgtype.NumericOID)
	if err != nil {
		t.Fatalf("normalizeQueryValue() error = %v", err)
	}

	if got, want := value, "123.45"; got != want {
		t.Fatalf("normalizeQueryValue() = %#v, want %#v", got, want)
	}
}

func TestNormalizeQueryValueByteaUsesBase64(t *testing.T) {
	value, err := normalizeQueryValue([]byte{0, 1, 2, 3}, pgtype.ByteaOID)
	if err != nil {
		t.Fatalf("normalizeQueryValue() error = %v", err)
	}

	if got, want := value, base64.StdEncoding.EncodeToString([]byte{0, 1, 2, 3}); got != want {
		t.Fatalf("normalizeQueryValue() = %#v, want %#v", got, want)
	}
}

func TestNormalizeQueryValueTime(t *testing.T) {
	timestamp := time.Date(2026, time.March, 26, 15, 4, 5, 123456000, time.UTC)
	value, err := normalizeQueryValue(timestamp, pgtype.TimestamptzOID)
	if err != nil {
		t.Fatalf("normalizeQueryValue() error = %v", err)
	}

	if got, want := value, "2026-03-26T15:04:05.123456Z"; got != want {
		t.Fatalf("normalizeQueryValue() = %#v, want %#v", got, want)
	}
}
