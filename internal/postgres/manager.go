package postgres

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/aparcero/mcp-postgres/internal/config"
	"github.com/aparcero/mcp-postgres/internal/confirm"
	"github.com/aparcero/mcp-postgres/internal/metrics"
	"github.com/aparcero/mcp-postgres/internal/policy"
)

type Manager struct {
	logger            *slog.Logger
	basePoolConfig    *pgxpool.Config
	bootstrapDatabase string
	queryPolicy       policy.Policy
	confirmations     *confirm.Manager
	telemetry         *metrics.Collector
	idlePoolTTL       time.Duration
	idlePoolCleanup   time.Duration
	poolMaxConns      int32
	maxCachedPools    int

	mu    sync.RWMutex
	pools map[string]*poolEntry

	closeOnce   sync.Once
	cleanupStop chan struct{}
	cleanupDone chan struct{}
}

type poolEntry struct {
	pool     *pgxpool.Pool
	lastUsed atomic.Int64
}

func NewManager(cfg config.Config, logger *slog.Logger) (*Manager, error) {
	basePoolConfig, err := pgxpool.ParseConfig(cfg.BaseDSN)
	if err != nil {
		return nil, fmt.Errorf("parse base DSN: %w", err)
	}
	if cfg.PoolMaxConns > 0 {
		basePoolConfig.MaxConns = cfg.PoolMaxConns
	}

	manager := &Manager{
		logger:            logger,
		basePoolConfig:    basePoolConfig,
		bootstrapDatabase: cfg.BootstrapDatabase,
		queryPolicy:       policy.New(cfg.Mode, cfg.DeniedSchemas, cfg.MutationDatabases),
		confirmations:     confirm.NewManager(cfg.ConfirmationTTL),
		telemetry:         metrics.NewCollector(time.Now().UTC()),
		idlePoolTTL:       cfg.IdlePoolTTL,
		idlePoolCleanup:   cfg.IdlePoolCleanup,
		poolMaxConns:      cfg.PoolMaxConns,
		maxCachedPools:    cfg.MaxCachedPools,
		pools:             make(map[string]*poolEntry),
		cleanupStop:       make(chan struct{}),
		cleanupDone:       make(chan struct{}),
	}

	if manager.idlePoolTTL > 0 && manager.idlePoolCleanup > 0 {
		go manager.cleanupLoop()
	} else {
		close(manager.cleanupDone)
	}

	return manager, nil
}

func (m *Manager) PingBootstrap(ctx context.Context) error {
	pool, err := m.BootstrapPool(ctx)
	if err != nil {
		return err
	}
	return pool.Ping(ctx)
}

func (m *Manager) BootstrapPool(ctx context.Context) (*pgxpool.Pool, error) {
	return m.PoolForDatabase(ctx, m.bootstrapDatabase)
}

func (m *Manager) PoolForDatabase(ctx context.Context, database string) (*pgxpool.Pool, error) {
	database = normalizeDatabaseName(database)
	if database == "" {
		return nil, fmt.Errorf("database must not be empty")
	}

	m.mu.RLock()
	if entry, ok := m.pools[database]; ok {
		entry.touch(time.Now().UTC())
		m.mu.RUnlock()
		return entry.pool, nil
	}
	m.mu.RUnlock()

	cfg, err := m.poolConfigForDatabase(database)
	if err != nil {
		return nil, err
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create pool for database %q: %w", database, err)
	}

	now := time.Now().UTC()
	entry := newPoolEntry(pool, now)

	m.mu.Lock()
	if existing, ok := m.pools[database]; ok {
		existing.touch(now)
		m.mu.Unlock()
		pool.Close()
		return existing.pool, nil
	}
	if m.maxCachedPools > 0 && len(m.pools) >= m.maxCachedPools {
		m.mu.Unlock()
		pool.Close()
		return nil, fmt.Errorf("maximum cached database pools reached (%d)", m.maxCachedPools)
	}

	m.pools[database] = entry
	m.mu.Unlock()

	m.telemetry.RecordPoolCreated()
	if m.logger != nil {
		m.logger.Debug("created database pool", "database", database)
	}
	return pool, nil
}

func (m *Manager) Close() {
	m.closeOnce.Do(func() {
		close(m.cleanupStop)
		<-m.cleanupDone

		m.mu.Lock()
		defer m.mu.Unlock()

		for database, entry := range m.pools {
			if entry.pool != nil {
				entry.pool.Close()
				m.telemetry.RecordPoolClosed()
			}
			delete(m.pools, database)
		}
	})
}

func (m *Manager) BootstrapDatabase() string {
	return m.bootstrapDatabase
}

func (m *Manager) BaseUser() string {
	return m.basePoolConfig.ConnConfig.User
}

func (m *Manager) BaseHostPort() string {
	return fmt.Sprintf("%s:%d", m.basePoolConfig.ConnConfig.Host, m.basePoolConfig.ConnConfig.Port)
}

func (m *Manager) PolicyMode() policy.Mode {
	return m.queryPolicy.Mode()
}

func (m *Manager) DeniedSchemas() []string {
	return m.queryPolicy.DeniedSchemas()
}

func (m *Manager) MutationDatabases() []string {
	return m.queryPolicy.MutationDatabases()
}

func (m *Manager) ConfirmationTTL() int64 {
	if m.confirmations == nil {
		return 0
	}
	return int64(m.confirmations.TTL().Seconds())
}

func (m *Manager) IdlePoolTTL() int64 {
	if m.idlePoolTTL <= 0 {
		return 0
	}
	return int64(m.idlePoolTTL.Seconds())
}

func (m *Manager) IdlePoolCleanupInterval() int64 {
	if m.idlePoolCleanup <= 0 {
		return 0
	}
	return int64(m.idlePoolCleanup.Seconds())
}

func (m *Manager) poolConfigForDatabase(database string) (*pgxpool.Config, error) {
	database = normalizeDatabaseName(database)
	if database == "" {
		return nil, fmt.Errorf("database must not be empty")
	}

	cfg := m.basePoolConfig.Copy()
	cfg.ConnConfig.Database = database
	if m.poolMaxConns > 0 {
		cfg.MaxConns = m.poolMaxConns
	}
	return cfg, nil
}

func normalizeDatabaseName(database string) string {
	return strings.TrimSpace(database)
}

func newPoolEntry(pool *pgxpool.Pool, now time.Time) *poolEntry {
	entry := &poolEntry{pool: pool}
	entry.touch(now)
	return entry
}

func (p *poolEntry) touch(now time.Time) {
	if p == nil {
		return
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	p.lastUsed.Store(now.UnixNano())
}

func (p *poolEntry) lastUsedAt() time.Time {
	if p == nil {
		return time.Time{}
	}

	value := p.lastUsed.Load()
	if value == 0 {
		return time.Time{}
	}
	return time.Unix(0, value).UTC()
}

func (m *Manager) cleanupLoop() {
	ticker := time.NewTicker(m.idlePoolCleanup)
	defer ticker.Stop()
	defer close(m.cleanupDone)

	for {
		select {
		case <-ticker.C:
			m.evictIdlePools(time.Now().UTC())
		case <-m.cleanupStop:
			return
		}
	}
}

func (m *Manager) evictIdlePools(now time.Time) {
	if m.idlePoolTTL <= 0 {
		return
	}

	type idlePool struct {
		database string
		pool     *pgxpool.Pool
	}

	evicted := make([]idlePool, 0)

	m.mu.Lock()
	for database, entry := range m.pools {
		if database == m.bootstrapDatabase {
			continue
		}
		if entry == nil || entry.pool == nil {
			delete(m.pools, database)
			continue
		}
		if now.Sub(entry.lastUsedAt()) < m.idlePoolTTL {
			continue
		}
		if entry.pool.Stat().AcquiredConns() > 0 {
			continue
		}

		delete(m.pools, database)
		evicted = append(evicted, idlePool{
			database: database,
			pool:     entry.pool,
		})
	}
	m.mu.Unlock()

	for _, item := range evicted {
		item.pool.Close()
		m.telemetry.RecordPoolClosed()
		if m.logger != nil {
			m.logger.Debug("evicted idle database pool", "database", item.database)
		}
	}

	m.telemetry.RecordIdleCleanup(now, len(evicted))
}
