package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aparcero/mcp-postgres/internal/audit"
	"github.com/aparcero/mcp-postgres/internal/confirm"
	"github.com/aparcero/mcp-postgres/internal/sqlguard"
	"github.com/aparcero/mcp-postgres/internal/types"
)

const adminToolName = "postgres.exec_admin"

func (m *Manager) ExecAdmin(ctx context.Context, database, sql string, timeout time.Duration, confirmationToken string) (out types.ExecAdminOutput, err error) {
	startedAt := time.Now()
	defer func() {
		m.observeOperation("postgres.exec_admin", startedAt, err)
	}()

	if strings.TrimSpace(database) == "" {
		return types.ExecAdminOutput{}, fmt.Errorf("database must not be empty")
	}
	if strings.TrimSpace(sql) == "" {
		return types.ExecAdminOutput{}, fmt.Errorf("sql must not be empty")
	}
	if timeout <= 0 {
		return types.ExecAdminOutput{}, fmt.Errorf("timeout must be > 0")
	}

	statement, err := sqlguard.ParseAndClassify(sql)
	if err != nil {
		return types.ExecAdminOutput{}, err
	}
	if err := m.queryPolicy.AuthorizeAdmin(database, statement); err != nil {
		return types.ExecAdminOutput{}, err
	}

	sqlHash, err := sqlguard.NormalizedHash(sql)
	if err != nil {
		return types.ExecAdminOutput{}, err
	}
	fingerprint, err := sqlguard.Fingerprint(sql)
	if err != nil {
		return types.ExecAdminOutput{}, err
	}

	record := audit.AdminRecord{
		Database:             database,
		Mode:                 string(m.PolicyMode()),
		StatementClass:       string(statement.Class),
		StatementKind:        statement.Kind,
		StatementFingerprint: fingerprint,
	}

	request := confirm.Request{
		ToolName: adminToolName,
		Database: database,
		Mode:     string(m.PolicyMode()),
		SQLHash:  sqlHash,
	}

	if sqlguard.RequiresConfirmation(statement) {
		if confirmationToken == "" {
			issued, err := m.confirmations.Issue(request)
			if err != nil {
				return types.ExecAdminOutput{}, err
			}

			expiresIn := int64(time.Until(issued.ExpiresAt).Seconds())
			if expiresIn < 1 {
				expiresIn = m.ConfirmationTTL()
			}

			record.Status = "confirmation_required"
			record.ExpiresInSeconds = expiresIn
			audit.LogAdmin(m.logger, record)

			return types.ExecAdminOutput{
				Database:          database,
				StatementClass:    string(statement.Class),
				StatementKind:     statement.Kind,
				Status:            "confirmation_required",
				Transaction:       "pending_confirmation",
				ConfirmationToken: issued.Token,
				ExpiresInSeconds:  expiresIn,
			}, nil
		}

		if err := m.confirmations.Consume(confirmationToken, request); err != nil {
			record.Status = "confirmation_failed"
			record.Error = err.Error()
			audit.LogAdmin(m.logger, record)
			return types.ExecAdminOutput{}, err
		}
	} else if confirmationToken != "" {
		return types.ExecAdminOutput{}, fmt.Errorf("confirmation token is only valid for destructive or privilege-changing statements")
	}

	queryCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	pool, err := m.PoolForDatabase(queryCtx, database)
	if err != nil {
		return types.ExecAdminOutput{}, err
	}

	executionStartedAt := time.Now()
	commandTag, execErr := pool.Exec(queryCtx, sql)
	durationMs := time.Since(executionStartedAt).Milliseconds()

	record.DurationMs = durationMs

	if execErr != nil {
		record.Status = "failed"
		record.Error = execErr.Error()
		audit.LogAdmin(m.logger, record)
		return types.ExecAdminOutput{}, wrapQueryError("execute admin statement", execErr, queryCtx)
	}

	record.Status = "committed"
	record.CommandTag = commandTag.String()
	record.RowsAffected = commandTag.RowsAffected()
	audit.LogAdmin(m.logger, record)

	return types.ExecAdminOutput{
		Database:       database,
		StatementClass: string(statement.Class),
		StatementKind:  statement.Kind,
		Status:         "committed",
		CommandTag:     commandTag.String(),
		RowsAffected:   commandTag.RowsAffected(),
		Transaction:    "committed",
		DurationMs:     durationMs,
	}, nil
}
