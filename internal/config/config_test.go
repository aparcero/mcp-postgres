package config

import (
	"testing"
	"time"

	"github.com/aparcero/mcp-postgres/internal/policy"
)

func TestLoadFromEnvDefaults(t *testing.T) {
	lookup := func(key string) (string, bool) {
		values := map[string]string{
			"POSTGRES_BASE_DSN": "postgresql://postgres:secret@localhost:5432/postgres?sslmode=disable",
		}
		value, ok := values[key]
		return value, ok
	}

	cfg, err := LoadFromEnv(lookup)
	if err != nil {
		t.Fatalf("LoadFromEnv() error = %v", err)
	}

	if cfg.BaseDSN == "" {
		t.Fatal("BaseDSN is empty")
	}
	if cfg.BootstrapDatabase != defaultBootstrapDatabase {
		t.Fatalf("BootstrapDatabase = %q, want %q", cfg.BootstrapDatabase, defaultBootstrapDatabase)
	}
	if cfg.Mode != policy.ModeReadOnly {
		t.Fatalf("Mode = %q, want %q", cfg.Mode, policy.ModeReadOnly)
	}
	if len(cfg.DeniedSchemas) != len(policy.DefaultDeniedSchemas) {
		t.Fatalf("len(DeniedSchemas) = %d, want %d", len(cfg.DeniedSchemas), len(policy.DefaultDeniedSchemas))
	}
	if cfg.ConfirmationTTL != defaultConfirmationTTL {
		t.Fatalf("ConfirmationTTL = %s, want %s", cfg.ConfirmationTTL, defaultConfirmationTTL)
	}
	if cfg.QueryTimeout != defaultQueryTimeout {
		t.Fatalf("QueryTimeout = %s, want %s", cfg.QueryTimeout, defaultQueryTimeout)
	}
	if cfg.ExecTimeout != defaultExecTimeout {
		t.Fatalf("ExecTimeout = %s, want %s", cfg.ExecTimeout, defaultExecTimeout)
	}
	if cfg.IdlePoolTTL != defaultIdlePoolTTL {
		t.Fatalf("IdlePoolTTL = %s, want %s", cfg.IdlePoolTTL, defaultIdlePoolTTL)
	}
	if cfg.IdlePoolCleanup != defaultIdlePoolCleanup {
		t.Fatalf("IdlePoolCleanup = %s, want %s", cfg.IdlePoolCleanup, defaultIdlePoolCleanup)
	}
	if len(cfg.MutationDatabases) != 0 {
		t.Fatalf("len(MutationDatabases) = %d, want 0", len(cfg.MutationDatabases))
	}
	if cfg.DefaultMaxRows != defaultMaxRows {
		t.Fatalf("DefaultMaxRows = %d, want %d", cfg.DefaultMaxRows, defaultMaxRows)
	}
	if cfg.MaxMaxRows != defaultHardMaxRows {
		t.Fatalf("MaxMaxRows = %d, want %d", cfg.MaxMaxRows, defaultHardMaxRows)
	}
	if cfg.DefaultSampleRows != defaultSampleRows {
		t.Fatalf("DefaultSampleRows = %d, want %d", cfg.DefaultSampleRows, defaultSampleRows)
	}
	if cfg.MaxSampleRows != defaultMaxSampleRows {
		t.Fatalf("MaxSampleRows = %d, want %d", cfg.MaxSampleRows, defaultMaxSampleRows)
	}
	if cfg.PoolMaxConns != defaultPoolMaxConns {
		t.Fatalf("PoolMaxConns = %d, want %d", cfg.PoolMaxConns, defaultPoolMaxConns)
	}
	if cfg.MaxCachedPools != defaultMaxCachedPools {
		t.Fatalf("MaxCachedPools = %d, want %d", cfg.MaxCachedPools, defaultMaxCachedPools)
	}
}

func TestLoadFromEnvRequiresBaseDSN(t *testing.T) {
	lookup := func(string) (string, bool) {
		return "", false
	}

	if _, err := LoadFromEnv(lookup); err == nil {
		t.Fatal("LoadFromEnv() error = nil, want non-nil")
	} else if got, want := err.Error(), "POSTGRES_BASE_DSN must be set"; got != want {
		t.Fatalf("LoadFromEnv() error = %q, want %q", got, want)
	}
}

func TestLoadFromEnvRejectsInvalidSampleBounds(t *testing.T) {
	lookup := func(key string) (string, bool) {
		values := map[string]string{
			"POSTGRES_BASE_DSN":                 "postgresql://postgres:secret@localhost:5432/postgres",
			"POSTGRES_BOOTSTRAP_DATABASE":       "postgres",
			"POSTGRES_MODE":                     "readonly",
			"POSTGRES_DENIED_SCHEMAS":           "pg_catalog,information_schema",
			"POSTGRES_CONFIRMATION_TTL_SECONDS": "120",
			"POSTGRES_QUERY_TIMEOUT_MS":         "15000",
			"POSTGRES_EXEC_TIMEOUT_MS":          "15000",
			"POSTGRES_DEFAULT_MAX_ROWS":         "100",
			"POSTGRES_MAX_MAX_ROWS":             "1000",
			"POSTGRES_DEFAULT_SAMPLE_ROWS":      "200",
			"POSTGRES_MAX_SAMPLE_ROWS":          "100",
		}
		value, ok := values[key]
		return value, ok
	}

	if _, err := LoadFromEnv(lookup); err == nil {
		t.Fatal("LoadFromEnv() error = nil, want non-nil")
	}
}

func TestLoadFromEnvRejectsInvalidQueryBounds(t *testing.T) {
	lookup := func(key string) (string, bool) {
		values := map[string]string{
			"POSTGRES_BASE_DSN":                 "postgresql://postgres:secret@localhost:5432/postgres",
			"POSTGRES_BOOTSTRAP_DATABASE":       "postgres",
			"POSTGRES_MODE":                     "readonly",
			"POSTGRES_DENIED_SCHEMAS":           "pg_catalog,information_schema",
			"POSTGRES_CONFIRMATION_TTL_SECONDS": "0",
			"POSTGRES_QUERY_TIMEOUT_MS":         "0",
			"POSTGRES_EXEC_TIMEOUT_MS":          "0",
			"POSTGRES_DEFAULT_MAX_ROWS":         "200",
			"POSTGRES_MAX_MAX_ROWS":             "100",
			"POSTGRES_DEFAULT_SAMPLE_ROWS":      "10",
			"POSTGRES_MAX_SAMPLE_ROWS":          "100",
		}
		value, ok := values[key]
		return value, ok
	}

	if _, err := LoadFromEnv(lookup); err == nil {
		t.Fatal("LoadFromEnv() error = nil, want non-nil")
	}
}

func TestLoadFromEnvParsesQueryTimeout(t *testing.T) {
	lookup := func(key string) (string, bool) {
		values := map[string]string{
			"POSTGRES_BASE_DSN":                      "postgresql://postgres:secret@localhost:5432/postgres",
			"POSTGRES_BOOTSTRAP_DATABASE":            "postgres",
			"POSTGRES_MODE":                          "readonly",
			"POSTGRES_DENIED_SCHEMAS":                "pg_catalog,information_schema",
			"POSTGRES_CONFIRMATION_TTL_SECONDS":      "120",
			"POSTGRES_QUERY_TIMEOUT_MS":              "2500",
			"POSTGRES_EXEC_TIMEOUT_MS":               "5000",
			"POSTGRES_IDLE_POOL_TTL_MS":              "60000",
			"POSTGRES_IDLE_POOL_CLEANUP_INTERVAL_MS": "15000",
			"POSTGRES_DEFAULT_MAX_ROWS":              "25",
			"POSTGRES_MAX_MAX_ROWS":                  "250",
			"POSTGRES_DEFAULT_SAMPLE_ROWS":           "10",
			"POSTGRES_MAX_SAMPLE_ROWS":               "100",
		}
		value, ok := values[key]
		return value, ok
	}

	cfg, err := LoadFromEnv(lookup)
	if err != nil {
		t.Fatalf("LoadFromEnv() error = %v", err)
	}

	if got, want := cfg.QueryTimeout, 2500*time.Millisecond; got != want {
		t.Fatalf("QueryTimeout = %s, want %s", got, want)
	}
	if got, want := cfg.ConfirmationTTL, 120*time.Second; got != want {
		t.Fatalf("ConfirmationTTL = %s, want %s", got, want)
	}
	if got, want := cfg.ExecTimeout, 5*time.Second; got != want {
		t.Fatalf("ExecTimeout = %s, want %s", got, want)
	}
	if got, want := cfg.IdlePoolTTL, 60*time.Second; got != want {
		t.Fatalf("IdlePoolTTL = %s, want %s", got, want)
	}
	if got, want := cfg.IdlePoolCleanup, 15*time.Second; got != want {
		t.Fatalf("IdlePoolCleanup = %s, want %s", got, want)
	}
}

func TestLoadFromEnvRejectsInvalidMode(t *testing.T) {
	lookup := func(key string) (string, bool) {
		values := map[string]string{
			"POSTGRES_BASE_DSN":                 "postgresql://postgres:secret@localhost:5432/postgres",
			"POSTGRES_BOOTSTRAP_DATABASE":       "postgres",
			"POSTGRES_MODE":                     "superuser",
			"POSTGRES_CONFIRMATION_TTL_SECONDS": "120",
			"POSTGRES_QUERY_TIMEOUT_MS":         "2500",
			"POSTGRES_EXEC_TIMEOUT_MS":          "5000",
			"POSTGRES_DEFAULT_MAX_ROWS":         "25",
			"POSTGRES_MAX_MAX_ROWS":             "250",
			"POSTGRES_DEFAULT_SAMPLE_ROWS":      "10",
			"POSTGRES_MAX_SAMPLE_ROWS":          "100",
		}
		value, ok := values[key]
		return value, ok
	}

	if _, err := LoadFromEnv(lookup); err == nil {
		t.Fatal("LoadFromEnv() error = nil, want non-nil")
	}
}

func TestLoadFromEnvRejectsInvalidNumericValue(t *testing.T) {
	lookup := func(key string) (string, bool) {
		values := map[string]string{
			"POSTGRES_BASE_DSN":         "postgresql://postgres:secret@localhost:5432/postgres",
			"POSTGRES_QUERY_TIMEOUT_MS": "not-a-number",
		}
		value, ok := values[key]
		return value, ok
	}

	if _, err := LoadFromEnv(lookup); err == nil {
		t.Fatal("LoadFromEnv() error = nil, want non-nil")
	}
}

func TestLoadFromEnvRejectsInvalidBoolValue(t *testing.T) {
	lookup := func(key string) (string, bool) {
		values := map[string]string{
			"POSTGRES_BASE_DSN": "postgresql://postgres:secret@localhost:5432/postgres",
			"POSTGRES_LOG_JSON": "sometimes",
		}
		value, ok := values[key]
		return value, ok
	}

	if _, err := LoadFromEnv(lookup); err == nil {
		t.Fatal("LoadFromEnv() error = nil, want non-nil")
	}
}

func TestLoadFromEnvParsesPoolCaps(t *testing.T) {
	lookup := func(key string) (string, bool) {
		values := map[string]string{
			"POSTGRES_BASE_DSN":         "postgresql://postgres:secret@localhost:5432/postgres",
			"POSTGRES_POOL_MAX_CONNS":   "7",
			"POSTGRES_MAX_CACHED_POOLS": "11",
		}
		value, ok := values[key]
		return value, ok
	}

	cfg, err := LoadFromEnv(lookup)
	if err != nil {
		t.Fatalf("LoadFromEnv() error = %v", err)
	}

	if got, want := cfg.PoolMaxConns, int32(7); got != want {
		t.Fatalf("PoolMaxConns = %d, want %d", got, want)
	}
	if got, want := cfg.MaxCachedPools, 11; got != want {
		t.Fatalf("MaxCachedPools = %d, want %d", got, want)
	}
}

func TestLoadFromEnvNormalizesDeniedSchemas(t *testing.T) {
	lookup := func(key string) (string, bool) {
		values := map[string]string{
			"POSTGRES_BASE_DSN":                 "postgresql://postgres:secret@localhost:5432/postgres",
			"POSTGRES_BOOTSTRAP_DATABASE":       "postgres",
			"POSTGRES_MODE":                     "operator",
			"POSTGRES_DENIED_SCHEMAS":           "PG_CATALOG, information_schema, PG_CATALOG",
			"POSTGRES_MUTATION_DATABASES":       "cancellations, analytics, cancellations",
			"POSTGRES_CONFIRMATION_TTL_SECONDS": "120",
			"POSTGRES_QUERY_TIMEOUT_MS":         "2500",
			"POSTGRES_EXEC_TIMEOUT_MS":          "5000",
			"POSTGRES_DEFAULT_MAX_ROWS":         "25",
			"POSTGRES_MAX_MAX_ROWS":             "250",
			"POSTGRES_DEFAULT_SAMPLE_ROWS":      "10",
			"POSTGRES_MAX_SAMPLE_ROWS":          "100",
		}
		value, ok := values[key]
		return value, ok
	}

	cfg, err := LoadFromEnv(lookup)
	if err != nil {
		t.Fatalf("LoadFromEnv() error = %v", err)
	}

	if got, want := cfg.Mode, policy.ModeOperator; got != want {
		t.Fatalf("Mode = %q, want %q", got, want)
	}
	if len(cfg.DeniedSchemas) != 2 {
		t.Fatalf("len(DeniedSchemas) = %d, want 2", len(cfg.DeniedSchemas))
	}
	if got, want := cfg.DeniedSchemas[0], "pg_catalog"; got != want {
		t.Fatalf("DeniedSchemas[0] = %q, want %q", got, want)
	}
	if got, want := cfg.DeniedSchemas[1], "information_schema"; got != want {
		t.Fatalf("DeniedSchemas[1] = %q, want %q", got, want)
	}
	if len(cfg.MutationDatabases) != 2 {
		t.Fatalf("len(MutationDatabases) = %d, want 2", len(cfg.MutationDatabases))
	}
	if got, want := cfg.MutationDatabases[0], "cancellations"; got != want {
		t.Fatalf("MutationDatabases[0] = %q, want %q", got, want)
	}
	if got, want := cfg.MutationDatabases[1], "analytics"; got != want {
		t.Fatalf("MutationDatabases[1] = %q, want %q", got, want)
	}
}
