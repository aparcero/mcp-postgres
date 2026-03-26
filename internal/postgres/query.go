package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/aparcero/mcp-postgres/internal/sqlguard"
	"github.com/aparcero/mcp-postgres/internal/types"
)

func (m *Manager) Query(ctx context.Context, database, sql string, maxRows int, timeout time.Duration) (out types.QueryOutput, err error) {
	startedAt := time.Now()
	defer func() {
		m.observeOperation("postgres.query", startedAt, err)
	}()

	if strings.TrimSpace(database) == "" {
		return types.QueryOutput{}, fmt.Errorf("database must not be empty")
	}
	if strings.TrimSpace(sql) == "" {
		return types.QueryOutput{}, fmt.Errorf("sql must not be empty")
	}
	if maxRows < 1 {
		return types.QueryOutput{}, fmt.Errorf("maxRows must be >= 1")
	}
	if timeout <= 0 {
		return types.QueryOutput{}, fmt.Errorf("timeout must be > 0")
	}

	statement, err := sqlguard.ParseAndClassify(sql)
	if err != nil {
		return types.QueryOutput{}, err
	}
	if err := m.queryPolicy.AuthorizeQuery(statement); err != nil {
		return types.QueryOutput{}, err
	}

	pool, err := m.PoolForDatabase(ctx, database)
	if err != nil {
		return types.QueryOutput{}, err
	}

	queryCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	conn, err := pool.Acquire(queryCtx)
	if err != nil {
		return types.QueryOutput{}, wrapQueryError("acquire connection", err, queryCtx)
	}
	defer conn.Release()

	executionStartedAt := time.Now()
	tx, err := conn.BeginTx(queryCtx, queryTxOptions())
	if err != nil {
		return types.QueryOutput{}, wrapQueryError("begin read-only transaction", err, queryCtx)
	}
	defer func() {
		_ = tx.Rollback(context.Background())
	}()

	rows, err := tx.Query(queryCtx, sql)
	if err != nil {
		return types.QueryOutput{}, wrapQueryError("execute query", err, queryCtx)
	}
	defer rows.Close()

	fields := rows.FieldDescriptions()
	columns := buildQueryColumns(conn.Conn().TypeMap(), fields)
	resultRows, truncated, err := collectQueryRows(rows, columns, fields, maxRows)
	if err != nil {
		return types.QueryOutput{}, wrapQueryError("read rows", err, queryCtx)
	}
	rows.Close()
	if err := tx.Commit(queryCtx); err != nil {
		return types.QueryOutput{}, wrapQueryError("commit read-only transaction", err, queryCtx)
	}

	return types.QueryOutput{
		Database:   database,
		Columns:    columns,
		Rows:       resultRows,
		RowCount:   len(resultRows),
		Truncated:  truncated,
		DurationMs: time.Since(executionStartedAt).Milliseconds(),
	}, nil
}

func queryTxOptions() pgx.TxOptions {
	return pgx.TxOptions{AccessMode: pgx.ReadOnly}
}

func collectQueryRows(rows pgx.Rows, columns []types.QueryColumn, fields []pgconn.FieldDescription, maxRows int) ([]map[string]any, bool, error) {
	resultRows := make([]map[string]any, 0, maxRows)
	truncated := false

	for rows.Next() {
		if len(resultRows) == maxRows {
			truncated = true
			rows.Close()
			break
		}

		values, err := rows.Values()
		if err != nil {
			return nil, false, fmt.Errorf("read row values: %w", err)
		}

		row, err := buildQueryRow(columns, fields, values)
		if err != nil {
			return nil, false, err
		}

		resultRows = append(resultRows, row)
	}

	if err := rows.Err(); err != nil {
		return nil, false, err
	}

	return resultRows, truncated, nil
}

func wrapQueryError(action string, err error, ctx context.Context) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return fmt.Errorf("%s: query timed out", action)
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return fmt.Errorf("%s: query canceled", action)
	}
	return fmt.Errorf("%s: %w", action, err)
}
