package audit

import "log/slog"

type DMLRecord struct {
	Database             string
	Mode                 string
	StatementClass       string
	StatementKind        string
	StatementFingerprint string
	Status               string
	CommandTag           string
	RowsAffected         int64
	DurationMs           int64
	Error                string
}

type AdminRecord struct {
	Database             string
	Mode                 string
	StatementClass       string
	StatementKind        string
	StatementFingerprint string
	Status               string
	CommandTag           string
	RowsAffected         int64
	DurationMs           int64
	ExpiresInSeconds     int64
	Error                string
}

func LogDML(logger *slog.Logger, record DMLRecord) {
	if logger == nil {
		return
	}

	logger.Info("audit dml",
		"database", record.Database,
		"mode", record.Mode,
		"statement_class", record.StatementClass,
		"statement_kind", record.StatementKind,
		"statement_fingerprint", record.StatementFingerprint,
		"status", record.Status,
		"command_tag", record.CommandTag,
		"rows_affected", record.RowsAffected,
		"duration_ms", record.DurationMs,
		"error", record.Error,
	)
}

func LogAdmin(logger *slog.Logger, record AdminRecord) {
	if logger == nil {
		return
	}

	logger.Info("audit admin",
		"database", record.Database,
		"mode", record.Mode,
		"statement_class", record.StatementClass,
		"statement_kind", record.StatementKind,
		"statement_fingerprint", record.StatementFingerprint,
		"status", record.Status,
		"command_tag", record.CommandTag,
		"rows_affected", record.RowsAffected,
		"duration_ms", record.DurationMs,
		"expires_in_seconds", record.ExpiresInSeconds,
		"error", record.Error,
	)
}
