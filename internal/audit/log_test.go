package audit

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestLogDMLUsesFingerprintWithoutRawSQL(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	LogDML(logger, DMLRecord{
		Database:             "app_db",
		Mode:                 "operator",
		StatementClass:       "mutating",
		StatementKind:        "update",
		StatementFingerprint: "fingerprint-123",
		Status:               "committed",
		CommandTag:           "UPDATE 1",
		RowsAffected:         1,
		DurationMs:           2,
		Error:                "constraint violation on widgets",
	})

	got := buf.String()
	if !strings.Contains(got, `"statement_fingerprint":"fingerprint-123"`) {
		t.Fatalf("audit log = %s, want statement fingerprint", got)
	}
	if strings.Contains(got, "update public.widgets") {
		t.Fatalf("audit log contains raw SQL: %s", got)
	}
}

func TestLogAdminUsesFingerprintWithoutRawSQL(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	LogAdmin(logger, AdminRecord{
		Database:             "app_db",
		Mode:                 "admin",
		StatementClass:       "destructive",
		StatementKind:        "drop",
		StatementFingerprint: "fingerprint-456",
		Status:               "confirmation_required",
		ExpiresInSeconds:     120,
		Error:                "relation old_orders is locked",
	})

	got := buf.String()
	if !strings.Contains(got, `"statement_fingerprint":"fingerprint-456"`) {
		t.Fatalf("audit log = %s, want statement fingerprint", got)
	}
	if strings.Contains(got, "drop table public.old_orders") {
		t.Fatalf("audit log contains raw SQL: %s", got)
	}
}
