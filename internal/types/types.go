package types

import "time"

type EmptyInput struct{}

type DatabaseInput struct {
	Database string `json:"database" jsonschema:"target PostgreSQL database"`
}

type ListTablesInput struct {
	Database string `json:"database" jsonschema:"target PostgreSQL database"`
	Schema   string `json:"schema,omitempty" jsonschema:"optional schema filter"`
}

type DescribeTableInput struct {
	Database string `json:"database" jsonschema:"target PostgreSQL database"`
	Schema   string `json:"schema" jsonschema:"table schema"`
	Table    string `json:"table" jsonschema:"table name"`
}

type SampleTableInput struct {
	Database string `json:"database" jsonschema:"target PostgreSQL database"`
	Schema   string `json:"schema" jsonschema:"table schema"`
	Table    string `json:"table" jsonschema:"table name"`
	Limit    int    `json:"limit,omitempty" jsonschema:"optional sample size limit"`
}

type QueryInput struct {
	Database  string `json:"database" jsonschema:"target PostgreSQL database"`
	SQL       string `json:"sql" jsonschema:"single read-only SQL statement"`
	MaxRows   int    `json:"max_rows,omitempty" jsonschema:"optional row cap"`
	TimeoutMs int    `json:"timeout_ms,omitempty" jsonschema:"optional timeout in milliseconds"`
}

type ExecDMLInput struct {
	Database  string `json:"database" jsonschema:"target PostgreSQL database"`
	SQL       string `json:"sql" jsonschema:"single INSERT, UPDATE, DELETE, or MERGE statement"`
	TimeoutMs int    `json:"timeout_ms,omitempty" jsonschema:"optional timeout in milliseconds"`
}

type ExecAdminInput struct {
	Database          string `json:"database" jsonschema:"target PostgreSQL database"`
	SQL               string `json:"sql" jsonschema:"single administrative SQL statement"`
	TimeoutMs         int    `json:"timeout_ms,omitempty" jsonschema:"optional timeout in milliseconds"`
	ConfirmationToken string `json:"confirmation_token,omitempty" jsonschema:"required for destructive or privilege-changing operations after confirmation is requested"`
}

type CountRowsInput struct {
	Database string         `json:"database" jsonschema:"target PostgreSQL database"`
	Schema   string         `json:"schema" jsonschema:"table schema"`
	Table    string         `json:"table" jsonschema:"table name"`
	Where    map[string]any `json:"where,omitempty" jsonschema:"optional equality filters"`
}

type ConnectionStatusOutput struct {
	Connected         bool     `json:"connected"`
	Database          string   `json:"database"`
	User              string   `json:"user"`
	Host              string   `json:"host"`
	Version           string   `json:"version"`
	Mode              string   `json:"mode"`
	DeniedSchemas     []string `json:"denied_schemas"`
	MutationDatabases []string `json:"mutation_databases"`
}

type PoolRuntimeInfo struct {
	Database      string    `json:"database"`
	TotalConns    int32     `json:"total_conns"`
	IdleConns     int32     `json:"idle_conns"`
	AcquiredConns int32     `json:"acquired_conns"`
	MaxConns      int32     `json:"max_conns"`
	LastUsedAt    time.Time `json:"last_used_at"`
}

type ToolMetric struct {
	Tool              string `json:"tool"`
	Requests          int64  `json:"requests"`
	Successes         int64  `json:"successes"`
	Failures          int64  `json:"failures"`
	AverageDurationMs int64  `json:"average_duration_ms"`
	LastDurationMs    int64  `json:"last_duration_ms"`
}

type ServerMetricsOutput struct {
	Mode                           string            `json:"mode"`
	BootstrapDatabase              string            `json:"bootstrap_database"`
	StartedAt                      time.Time         `json:"started_at"`
	UptimeSeconds                  int64             `json:"uptime_seconds"`
	IdlePoolTTLSeconds             int64             `json:"idle_pool_ttl_seconds"`
	IdlePoolCleanupIntervalSeconds int64             `json:"idle_pool_cleanup_interval_seconds"`
	CachedPools                    int               `json:"cached_pools"`
	TotalPoolsCreated              int64             `json:"total_pools_created"`
	TotalPoolsClosed               int64             `json:"total_pools_closed"`
	IdlePoolsEvicted               int64             `json:"idle_pools_evicted"`
	IdleCleanupRuns                int64             `json:"idle_cleanup_runs"`
	LastIdleCleanupAt              *time.Time        `json:"last_idle_cleanup_at,omitempty"`
	LastIdleEvictionAt             *time.Time        `json:"last_idle_eviction_at,omitempty"`
	PendingConfirmationTokens      int               `json:"pending_confirmation_tokens"`
	Pools                          []PoolRuntimeInfo `json:"pools"`
	Operations                     []ToolMetric      `json:"operations"`
}

type DatabaseInfo struct {
	Name             string `json:"name"`
	AllowConnections bool   `json:"allow_connections"`
}

type ListDatabasesOutput struct {
	Databases []DatabaseInfo `json:"databases"`
}

type SchemaInfo struct {
	Name string `json:"name"`
}

type ListSchemasOutput struct {
	Database string       `json:"database"`
	Schemas  []SchemaInfo `json:"schemas"`
}

type TableInfo struct {
	Schema string `json:"schema"`
	Name   string `json:"name"`
	Type   string `json:"type"`
}

type ListTablesOutput struct {
	Database string      `json:"database"`
	Tables   []TableInfo `json:"tables"`
}

type ColumnInfo struct {
	Name         string  `json:"name"`
	DataType     string  `json:"data_type"`
	IsNullable   bool    `json:"is_nullable"`
	DefaultValue *string `json:"default_value,omitempty"`
	Ordinal      int     `json:"ordinal"`
}

type IndexInfo struct {
	Name       string `json:"name"`
	Definition string `json:"definition"`
}

type ConstraintInfo struct {
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	Columns []string `json:"columns"`
}

type DescribeTableOutput struct {
	Database    string           `json:"database"`
	Schema      string           `json:"schema"`
	Table       string           `json:"table"`
	Columns     []ColumnInfo     `json:"columns"`
	Indexes     []IndexInfo      `json:"indexes"`
	Constraints []ConstraintInfo `json:"constraints"`
}

type SampleTableOutput struct {
	Database  string           `json:"database"`
	Schema    string           `json:"schema"`
	Table     string           `json:"table"`
	Limit     int              `json:"limit"`
	RowCount  int              `json:"row_count"`
	Rows      []map[string]any `json:"rows"`
	Truncated bool             `json:"truncated"`
}

type QueryColumn struct {
	Name       string `json:"name"`
	SourceName string `json:"source_name,omitempty"`
	DBType     string `json:"db_type"`
}

type QueryOutput struct {
	Database   string           `json:"database"`
	Columns    []QueryColumn    `json:"columns"`
	Rows       []map[string]any `json:"rows"`
	RowCount   int              `json:"row_count"`
	Truncated  bool             `json:"truncated"`
	DurationMs int64            `json:"duration_ms"`
}

type ExecDMLOutput struct {
	Database       string `json:"database"`
	StatementClass string `json:"statement_class"`
	StatementKind  string `json:"statement_kind"`
	CommandTag     string `json:"command_tag"`
	RowsAffected   int64  `json:"rows_affected"`
	Transaction    string `json:"transaction"`
	DurationMs     int64  `json:"duration_ms"`
}

type ExecAdminOutput struct {
	Database          string `json:"database"`
	StatementClass    string `json:"statement_class"`
	StatementKind     string `json:"statement_kind"`
	Status            string `json:"status"`
	CommandTag        string `json:"command_tag,omitempty"`
	RowsAffected      int64  `json:"rows_affected,omitempty"`
	Transaction       string `json:"transaction,omitempty"`
	DurationMs        int64  `json:"duration_ms,omitempty"`
	ConfirmationToken string `json:"confirmation_token,omitempty"`
	ExpiresInSeconds  int64  `json:"expires_in_seconds,omitempty"`
}

type CountRowsOutput struct {
	Database string `json:"database"`
	Schema   string `json:"schema"`
	Table    string `json:"table"`
	Count    int64  `json:"count"`
}
