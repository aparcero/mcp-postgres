package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aparcero/mcp-postgres/internal/audit"
	"github.com/aparcero/mcp-postgres/internal/sqlguard"
	"github.com/aparcero/mcp-postgres/internal/types"
)

func (m *Manager) ExecDML(ctx context.Context, database, sql string, timeout time.Duration) (out types.ExecDMLOutput, err error) {
	startedAt := time.Now()
	defer func() {
		m.observeOperation("postgres.exec_dml", startedAt, err)
	}()

	if strings.TrimSpace(database) == "" {
		return types.ExecDMLOutput{}, fmt.Errorf("database must not be empty")
	}
	if strings.TrimSpace(sql) == "" {
		return types.ExecDMLOutput{}, fmt.Errorf("sql must not be empty")
	}
	if timeout <= 0 {
		return types.ExecDMLOutput{}, fmt.Errorf("timeout must be > 0")
	}

	statement, err := sqlguard.ParseAndClassify(sql)
	if err != nil {
		return types.ExecDMLOutput{}, err
	}
	if err := m.queryPolicy.AuthorizeDML(database, statement); err != nil {
		return types.ExecDMLOutput{}, err
	}

	fingerprint, err := sqlguard.Fingerprint(sql)
	if err != nil {
		return types.ExecDMLOutput{}, err
	}

	queryCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	pool, err := m.PoolForDatabase(queryCtx, database)
	if err != nil {
		return types.ExecDMLOutput{}, err
	}

	executionStartedAt := time.Now()
	commandTag, execErr := pool.Exec(queryCtx, sql)
	durationMs := time.Since(executionStartedAt).Milliseconds()

	record := audit.DMLRecord{
		Database:             database,
		Mode:                 string(m.PolicyMode()),
		StatementClass:       string(statement.Class),
		StatementKind:        statement.Kind,
		StatementFingerprint: fingerprint,
		DurationMs:           durationMs,
	}

	if execErr != nil {
		record.Status = "failed"
		record.Error = execErr.Error()
		audit.LogDML(m.logger, record)
		return types.ExecDMLOutput{}, wrapQueryError("execute DML", execErr, queryCtx)
	}

	record.Status = "committed"
	record.CommandTag = commandTag.String()
	record.RowsAffected = commandTag.RowsAffected()
	audit.LogDML(m.logger, record)

	return types.ExecDMLOutput{
		Database:       database,
		StatementClass: string(statement.Class),
		StatementKind:  statement.Kind,
		CommandTag:     commandTag.String(),
		RowsAffected:   commandTag.RowsAffected(),
		Transaction:    "committed",
		DurationMs:     durationMs,
	}, nil
}
