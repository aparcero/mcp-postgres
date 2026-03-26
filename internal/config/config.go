package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/aparcero/mcp-postgres/internal/policy"
)

const (
	defaultBootstrapDatabase = "postgres"
	defaultLogLevel          = "info"
	defaultConfirmationTTL   = 120 * time.Second
	defaultQueryTimeout      = 15 * time.Second
	defaultExecTimeout       = 15 * time.Second
	defaultIdlePoolTTL       = 10 * time.Minute
	defaultIdlePoolCleanup   = 1 * time.Minute
	defaultMaxRows           = 100
	defaultHardMaxRows       = 1000
	defaultSampleRows        = 10
	defaultMaxSampleRows     = 100
	defaultPoolMaxConns      = 4
	defaultMaxCachedPools    = 16
)

type Config struct {
	BaseDSN           string
	BootstrapDatabase string
	Mode              policy.Mode
	DeniedSchemas     []string
	MutationDatabases []string
	ConfirmationTTL   time.Duration
	LogLevel          string
	LogJSON           bool
	QueryTimeout      time.Duration
	ExecTimeout       time.Duration
	IdlePoolTTL       time.Duration
	IdlePoolCleanup   time.Duration
	DefaultMaxRows    int
	MaxMaxRows        int
	DefaultSampleRows int
	MaxSampleRows     int
	PoolMaxConns      int32
	MaxCachedPools    int
}

func Load() (Config, error) {
	return LoadFromEnv(os.LookupEnv)
}

func LoadFromEnv(lookup func(string) (string, bool)) (Config, error) {
	baseDSN, ok := lookup("POSTGRES_BASE_DSN")
	if !ok || strings.TrimSpace(baseDSN) == "" {
		return Config{}, fmt.Errorf("POSTGRES_BASE_DSN must be set")
	}

	confirmationTTLSeconds, err := envInt(lookup, "POSTGRES_CONFIRMATION_TTL_SECONDS", int(defaultConfirmationTTL/time.Second))
	if err != nil {
		return Config{}, err
	}
	logJSON, err := envBool(lookup, "POSTGRES_LOG_JSON", false)
	if err != nil {
		return Config{}, err
	}
	queryTimeoutMS, err := envInt(lookup, "POSTGRES_QUERY_TIMEOUT_MS", int(defaultQueryTimeout/time.Millisecond))
	if err != nil {
		return Config{}, err
	}
	execTimeoutMS, err := envInt(lookup, "POSTGRES_EXEC_TIMEOUT_MS", int(defaultExecTimeout/time.Millisecond))
	if err != nil {
		return Config{}, err
	}
	idlePoolTTLMS, err := envInt(lookup, "POSTGRES_IDLE_POOL_TTL_MS", int(defaultIdlePoolTTL/time.Millisecond))
	if err != nil {
		return Config{}, err
	}
	idlePoolCleanupMS, err := envInt(lookup, "POSTGRES_IDLE_POOL_CLEANUP_INTERVAL_MS", int(defaultIdlePoolCleanup/time.Millisecond))
	if err != nil {
		return Config{}, err
	}
	parsedDefaultMaxRows, err := envInt(lookup, "POSTGRES_DEFAULT_MAX_ROWS", defaultMaxRows)
	if err != nil {
		return Config{}, err
	}
	maxMaxRows, err := envInt(lookup, "POSTGRES_MAX_MAX_ROWS", defaultHardMaxRows)
	if err != nil {
		return Config{}, err
	}
	parsedDefaultSampleRows, err := envInt(lookup, "POSTGRES_DEFAULT_SAMPLE_ROWS", defaultSampleRows)
	if err != nil {
		return Config{}, err
	}
	maxSampleRows, err := envInt(lookup, "POSTGRES_MAX_SAMPLE_ROWS", defaultMaxSampleRows)
	if err != nil {
		return Config{}, err
	}
	poolMaxConns, err := envInt(lookup, "POSTGRES_POOL_MAX_CONNS", defaultPoolMaxConns)
	if err != nil {
		return Config{}, err
	}
	maxCachedPools, err := envInt(lookup, "POSTGRES_MAX_CACHED_POOLS", defaultMaxCachedPools)
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		BaseDSN:           strings.TrimSpace(baseDSN),
		BootstrapDatabase: envString(lookup, "POSTGRES_BOOTSTRAP_DATABASE", defaultBootstrapDatabase),
		DeniedSchemas:     envList(lookup, "POSTGRES_DENIED_SCHEMAS", policy.DefaultDeniedSchemas),
		MutationDatabases: envList(lookup, "POSTGRES_MUTATION_DATABASES", nil),
		ConfirmationTTL:   time.Duration(confirmationTTLSeconds) * time.Second,
		LogLevel:          strings.ToLower(envString(lookup, "POSTGRES_LOG_LEVEL", defaultLogLevel)),
		LogJSON:           logJSON,
		QueryTimeout:      time.Duration(queryTimeoutMS) * time.Millisecond,
		ExecTimeout:       time.Duration(execTimeoutMS) * time.Millisecond,
		IdlePoolTTL:       time.Duration(idlePoolTTLMS) * time.Millisecond,
		IdlePoolCleanup:   time.Duration(idlePoolCleanupMS) * time.Millisecond,
		DefaultMaxRows:    parsedDefaultMaxRows,
		MaxMaxRows:        maxMaxRows,
		DefaultSampleRows: parsedDefaultSampleRows,
		MaxSampleRows:     maxSampleRows,
		PoolMaxConns:      int32(poolMaxConns),
		MaxCachedPools:    maxCachedPools,
	}

	mode, err := policy.ParseMode(envString(lookup, "POSTGRES_MODE", string(policy.ModeReadOnly)))
	if err != nil {
		return Config{}, err
	}
	cfg.Mode = mode

	if cfg.QueryTimeout <= 0 {
		return Config{}, fmt.Errorf("POSTGRES_QUERY_TIMEOUT_MS must be >= 1")
	}
	if cfg.ConfirmationTTL <= 0 {
		return Config{}, fmt.Errorf("POSTGRES_CONFIRMATION_TTL_SECONDS must be >= 1")
	}
	if cfg.ExecTimeout <= 0 {
		return Config{}, fmt.Errorf("POSTGRES_EXEC_TIMEOUT_MS must be >= 1")
	}
	if cfg.IdlePoolTTL < 0 {
		return Config{}, fmt.Errorf("POSTGRES_IDLE_POOL_TTL_MS must be >= 0")
	}
	if cfg.IdlePoolCleanup < 0 {
		return Config{}, fmt.Errorf("POSTGRES_IDLE_POOL_CLEANUP_INTERVAL_MS must be >= 0")
	}
	if cfg.DefaultMaxRows < 1 {
		return Config{}, fmt.Errorf("POSTGRES_DEFAULT_MAX_ROWS must be >= 1")
	}
	if cfg.MaxMaxRows < 1 {
		return Config{}, fmt.Errorf("POSTGRES_MAX_MAX_ROWS must be >= 1")
	}
	if cfg.DefaultMaxRows > cfg.MaxMaxRows {
		return Config{}, fmt.Errorf("POSTGRES_DEFAULT_MAX_ROWS must be <= POSTGRES_MAX_MAX_ROWS")
	}
	if cfg.DefaultSampleRows < 1 {
		return Config{}, fmt.Errorf("POSTGRES_DEFAULT_SAMPLE_ROWS must be >= 1")
	}
	if cfg.MaxSampleRows < 1 {
		return Config{}, fmt.Errorf("POSTGRES_MAX_SAMPLE_ROWS must be >= 1")
	}
	if cfg.DefaultSampleRows > cfg.MaxSampleRows {
		return Config{}, fmt.Errorf("POSTGRES_DEFAULT_SAMPLE_ROWS must be <= POSTGRES_MAX_SAMPLE_ROWS")
	}
	if cfg.PoolMaxConns < 1 {
		return Config{}, fmt.Errorf("POSTGRES_POOL_MAX_CONNS must be >= 1")
	}
	if cfg.MaxCachedPools < 1 {
		return Config{}, fmt.Errorf("POSTGRES_MAX_CACHED_POOLS must be >= 1")
	}
	if cfg.BootstrapDatabase == "" {
		return Config{}, fmt.Errorf("POSTGRES_BOOTSTRAP_DATABASE must not be empty")
	}

	if _, err := pgxpool.ParseConfig(cfg.BaseDSN); err != nil {
		return Config{}, fmt.Errorf("parse base DSN: %w", err)
	}

	return cfg, nil
}

func envList(lookup func(string) (string, bool), key string, fallback []string) []string {
	value, ok := lookup(key)
	if !ok || strings.TrimSpace(value) == "" {
		return cloneStrings(fallback)
	}

	parts := strings.Split(value, ",")
	return normalizeList(parts)
}

func envString(lookup func(string) (string, bool), key, fallback string) string {
	if value, ok := lookup(key); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return fallback
}

func envBool(lookup func(string) (string, bool), key string, fallback bool) (bool, error) {
	value, ok := lookup(key)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback, nil
	}

	parsed, err := strconv.ParseBool(strings.TrimSpace(value))
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean", key)
	}
	return parsed, nil
}

func envInt(lookup func(string) (string, bool), key string, fallback int) (int, error) {
	value, ok := lookup(key)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback, nil
	}

	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer", key)
	}
	return parsed, nil
}

func normalizeList(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		normalized := strings.ToLower(strings.TrimSpace(value))
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	return out
}

func cloneStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	out := make([]string, len(values))
	copy(out, values)
	return out
}
