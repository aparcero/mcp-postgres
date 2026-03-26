package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/aparcero/mcp-postgres/internal/types"
)

func (m *Manager) GetConnectionStatus(ctx context.Context) (out types.ConnectionStatusOutput, err error) {
	startedAt := time.Now()
	defer func() {
		m.observeOperation("postgres.get_connection_status", startedAt, err)
	}()

	pool, err := m.BootstrapPool(ctx)
	if err != nil {
		return types.ConnectionStatusOutput{}, err
	}

	err = pool.QueryRow(ctx, "select current_database(), current_user, version()").
		Scan(&out.Database, &out.User, &out.Version)
	if err != nil {
		return types.ConnectionStatusOutput{}, fmt.Errorf("query connection status: %w", err)
	}

	out.Connected = true
	out.Host = m.BaseHostPort()
	out.Mode = string(m.PolicyMode())
	out.DeniedSchemas = m.DeniedSchemas()
	out.MutationDatabases = m.MutationDatabases()
	return out, nil
}

func (m *Manager) ListDatabases(ctx context.Context) (out types.ListDatabasesOutput, err error) {
	startedAt := time.Now()
	defer func() {
		m.observeOperation("postgres.list_databases", startedAt, err)
	}()

	pool, err := m.BootstrapPool(ctx)
	if err != nil {
		return types.ListDatabasesOutput{}, err
	}

	rows, err := pool.Query(ctx, `
		select datname, datallowconn
		from pg_database
		where datistemplate = false
		order by datname`)
	if err != nil {
		return types.ListDatabasesOutput{}, fmt.Errorf("list databases: %w", err)
	}
	defer rows.Close()

	var databases []types.DatabaseInfo
	for rows.Next() {
		var item types.DatabaseInfo
		if err := rows.Scan(&item.Name, &item.AllowConnections); err != nil {
			return types.ListDatabasesOutput{}, fmt.Errorf("scan database row: %w", err)
		}
		databases = append(databases, item)
	}
	if err := rows.Err(); err != nil {
		return types.ListDatabasesOutput{}, fmt.Errorf("iterate database rows: %w", err)
	}

	return types.ListDatabasesOutput{Databases: databases}, nil
}

func (m *Manager) ListSchemas(ctx context.Context, database string) (out types.ListSchemasOutput, err error) {
	startedAt := time.Now()
	defer func() {
		m.observeOperation("postgres.list_schemas", startedAt, err)
	}()

	pool, err := m.PoolForDatabase(ctx, database)
	if err != nil {
		return types.ListSchemasOutput{}, err
	}

	rows, err := pool.Query(ctx, `
		select schema_name
		from information_schema.schemata
		order by schema_name`)
	if err != nil {
		return types.ListSchemasOutput{}, fmt.Errorf("list schemas: %w", err)
	}
	defer rows.Close()

	var schemas []types.SchemaInfo
	for rows.Next() {
		var item types.SchemaInfo
		if err := rows.Scan(&item.Name); err != nil {
			return types.ListSchemasOutput{}, fmt.Errorf("scan schema row: %w", err)
		}
		schemas = append(schemas, item)
	}
	if err := rows.Err(); err != nil {
		return types.ListSchemasOutput{}, fmt.Errorf("iterate schema rows: %w", err)
	}

	return types.ListSchemasOutput{
		Database: database,
		Schemas:  schemas,
	}, nil
}

func (m *Manager) ListTables(ctx context.Context, database, schema string) (out types.ListTablesOutput, err error) {
	startedAt := time.Now()
	defer func() {
		m.observeOperation("postgres.list_tables", startedAt, err)
	}()

	pool, err := m.PoolForDatabase(ctx, database)
	if err != nil {
		return types.ListTablesOutput{}, err
	}

	query := `
		select table_schema, table_name, table_type
		from information_schema.tables
		where (
			($1 <> '' and table_schema = $1)
			or (
				$1 = ''
				and table_schema not in ('pg_catalog', 'information_schema')
				and table_schema not like 'pg_toast%%'
				and table_schema not like 'pg_temp_%%'
			)
		)
		order by table_schema, table_name`

	rows, err := pool.Query(ctx, query, schema)
	if err != nil {
		return types.ListTablesOutput{}, fmt.Errorf("list tables: %w", err)
	}
	defer rows.Close()

	var tables []types.TableInfo
	for rows.Next() {
		var item types.TableInfo
		if err := rows.Scan(&item.Schema, &item.Name, &item.Type); err != nil {
			return types.ListTablesOutput{}, fmt.Errorf("scan table row: %w", err)
		}
		tables = append(tables, item)
	}
	if err := rows.Err(); err != nil {
		return types.ListTablesOutput{}, fmt.Errorf("iterate table rows: %w", err)
	}

	return types.ListTablesOutput{
		Database: database,
		Tables:   tables,
	}, nil
}

func (m *Manager) DescribeTable(ctx context.Context, database, schema, table string) (out types.DescribeTableOutput, err error) {
	startedAt := time.Now()
	defer func() {
		m.observeOperation("postgres.describe_table", startedAt, err)
	}()

	schema, table, err = normalizeRelationInput(schema, table)
	if err != nil {
		return types.DescribeTableOutput{}, err
	}

	pool, err := m.PoolForDatabase(ctx, database)
	if err != nil {
		return types.DescribeTableOutput{}, err
	}

	columnRows, err := pool.Query(ctx, `
		select
			column_name,
			data_type,
			is_nullable = 'YES' as is_nullable,
			column_default,
			ordinal_position
		from information_schema.columns
		where table_schema = $1 and table_name = $2
		order by ordinal_position`, schema, table)
	if err != nil {
		return types.DescribeTableOutput{}, fmt.Errorf("describe columns: %w", err)
	}
	defer columnRows.Close()

	var columns []types.ColumnInfo
	for columnRows.Next() {
		var item types.ColumnInfo
		if err := columnRows.Scan(&item.Name, &item.DataType, &item.IsNullable, &item.DefaultValue, &item.Ordinal); err != nil {
			return types.DescribeTableOutput{}, fmt.Errorf("scan column row: %w", err)
		}
		columns = append(columns, item)
	}
	if err := columnRows.Err(); err != nil {
		return types.DescribeTableOutput{}, fmt.Errorf("iterate column rows: %w", err)
	}

	indexRows, err := pool.Query(ctx, `
		select indexname, indexdef
		from pg_indexes
		where schemaname = $1 and tablename = $2
		order by indexname`, schema, table)
	if err != nil {
		return types.DescribeTableOutput{}, fmt.Errorf("describe indexes: %w", err)
	}
	defer indexRows.Close()

	var indexes []types.IndexInfo
	for indexRows.Next() {
		var item types.IndexInfo
		if err := indexRows.Scan(&item.Name, &item.Definition); err != nil {
			return types.DescribeTableOutput{}, fmt.Errorf("scan index row: %w", err)
		}
		indexes = append(indexes, item)
	}
	if err := indexRows.Err(); err != nil {
		return types.DescribeTableOutput{}, fmt.Errorf("iterate index rows: %w", err)
	}

	constraintRows, err := pool.Query(ctx, `
		select
			tc.constraint_name,
			tc.constraint_type,
			coalesce(
				array_agg(kcu.column_name order by kcu.ordinal_position)
				filter (where kcu.column_name is not null),
				array[]::text[]
			) as columns
		from information_schema.table_constraints tc
		left join information_schema.key_column_usage kcu
			on tc.constraint_name = kcu.constraint_name
			and tc.table_schema = kcu.table_schema
			and tc.table_name = kcu.table_name
		where tc.table_schema = $1 and tc.table_name = $2
		group by tc.constraint_name, tc.constraint_type
		order by tc.constraint_type, tc.constraint_name`, schema, table)
	if err != nil {
		return types.DescribeTableOutput{}, fmt.Errorf("describe constraints: %w", err)
	}
	defer constraintRows.Close()

	var constraints []types.ConstraintInfo
	for constraintRows.Next() {
		var item types.ConstraintInfo
		if err := constraintRows.Scan(&item.Name, &item.Type, &item.Columns); err != nil {
			return types.DescribeTableOutput{}, fmt.Errorf("scan constraint row: %w", err)
		}
		constraints = append(constraints, item)
	}
	if err := constraintRows.Err(); err != nil {
		return types.DescribeTableOutput{}, fmt.Errorf("iterate constraint rows: %w", err)
	}

	return types.DescribeTableOutput{
		Database:    database,
		Schema:      schema,
		Table:       table,
		Columns:     columns,
		Indexes:     indexes,
		Constraints: constraints,
	}, nil
}

func (m *Manager) SampleTable(ctx context.Context, database, schema, table string, limit int) (out types.SampleTableOutput, err error) {
	startedAt := time.Now()
	defer func() {
		m.observeOperation("postgres.sample_table", startedAt, err)
	}()

	schema, table, err = normalizeRelationInput(schema, table)
	if err != nil {
		return types.SampleTableOutput{}, err
	}

	pool, err := m.PoolForDatabase(ctx, database)
	if err != nil {
		return types.SampleTableOutput{}, err
	}

	identifier := qualifiedIdentifier(schema, table)
	query := fmt.Sprintf(`
		select coalesce(json_agg(row_to_json(t)), '[]'::json)
		from (
			select *
			from %s
			limit $1
		) as t`, identifier)

	var raw []byte
	if err := pool.QueryRow(ctx, query, limit+1).Scan(&raw); err != nil {
		return types.SampleTableOutput{}, fmt.Errorf("sample table: %w", err)
	}

	var rows []map[string]any
	if err := json.Unmarshal(raw, &rows); err != nil {
		return types.SampleTableOutput{}, fmt.Errorf("decode sampled rows: %w", err)
	}

	truncated := false
	if len(rows) > limit {
		rows = rows[:limit]
		truncated = true
	}

	return types.SampleTableOutput{
		Database:  database,
		Schema:    schema,
		Table:     table,
		Limit:     limit,
		RowCount:  len(rows),
		Rows:      rows,
		Truncated: truncated,
	}, nil
}

func (m *Manager) CountRows(ctx context.Context, database, schema, table string, where map[string]any) (out types.CountRowsOutput, err error) {
	startedAt := time.Now()
	defer func() {
		m.observeOperation("postgres.count_rows", startedAt, err)
	}()

	schema, table, err = normalizeRelationInput(schema, table)
	if err != nil {
		return types.CountRowsOutput{}, err
	}

	pool, err := m.PoolForDatabase(ctx, database)
	if err != nil {
		return types.CountRowsOutput{}, err
	}

	query, args, err := countRowsQuery(schema, table, where)
	if err != nil {
		return types.CountRowsOutput{}, err
	}

	var count int64
	if err := pool.QueryRow(ctx, query, args...).Scan(&count); err != nil {
		return types.CountRowsOutput{}, fmt.Errorf("count rows: %w", err)
	}

	return types.CountRowsOutput{
		Database: database,
		Schema:   schema,
		Table:    table,
		Count:    count,
	}, nil
}

func qualifiedIdentifier(schema, table string) string {
	return pgx.Identifier{schema, table}.Sanitize()
}

func countRowsQuery(schema, table string, where map[string]any) (string, []any, error) {
	base := fmt.Sprintf("select count(*) from %s", qualifiedIdentifier(schema, table))
	if len(where) == 0 {
		return base, nil, nil
	}

	clauses := make([]string, 0, len(where))
	args := make([]any, 0, len(where))
	argIndex := 1
	columns := make([]string, 0, len(where))
	for column := range where {
		columns = append(columns, column)
	}
	slices.Sort(columns)

	for _, column := range columns {
		value := where[column]
		columnIdent := pgx.Identifier{column}.Sanitize()
		if value == nil {
			clauses = append(clauses, fmt.Sprintf("%s is null", columnIdent))
			continue
		}

		raw, err := json.Marshal(value)
		if err != nil {
			return "", nil, fmt.Errorf("marshal where value for %q: %w", column, err)
		}

		clauses = append(clauses, fmt.Sprintf("to_jsonb(%s) = $%d::jsonb", columnIdent, argIndex))
		args = append(args, string(raw))
		argIndex++
	}

	query := base + " where " + joinClauses(clauses)
	return query, args, nil
}

func normalizeRelationInput(schema, table string) (string, string, error) {
	schema = strings.TrimSpace(schema)
	table = strings.TrimSpace(table)

	if schema == "" {
		return "", "", fmt.Errorf("schema must not be empty")
	}
	if table == "" {
		return "", "", fmt.Errorf("table must not be empty")
	}

	return schema, table, nil
}

func joinClauses(clauses []string) string {
	switch len(clauses) {
	case 0:
		return ""
	case 1:
		return clauses[0]
	default:
		out := clauses[0]
		for _, clause := range clauses[1:] {
			out += " and " + clause
		}
		return out
	}
}
