package postgres

import (
	"context"
	"slices"
	"time"

	"github.com/aparcero/mcp-postgres/internal/types"
)

func (m *Manager) GetServerMetrics(_ context.Context) (out types.ServerMetricsOutput, err error) {
	startedAt := time.Now()
	defer func() {
		m.observeOperation("postgres.get_server_metrics", startedAt, err)
	}()

	now := time.Now().UTC()
	snapshot := m.telemetry.Snapshot(now)
	pools := m.poolRuntimeSnapshot()
	operations := make([]types.ToolMetric, 0, len(snapshot.Operations))
	for _, item := range snapshot.Operations {
		operations = append(operations, types.ToolMetric{
			Tool:              item.Name,
			Requests:          item.Requests,
			Successes:         item.Successes,
			Failures:          item.Failures,
			AverageDurationMs: item.AverageDurationMs,
			LastDurationMs:    item.LastDurationMs,
		})
	}

	out = types.ServerMetricsOutput{
		Mode:                           string(m.PolicyMode()),
		BootstrapDatabase:              m.BootstrapDatabase(),
		StartedAt:                      snapshot.StartedAt,
		UptimeSeconds:                  snapshot.UptimeSeconds,
		IdlePoolTTLSeconds:             m.IdlePoolTTL(),
		IdlePoolCleanupIntervalSeconds: m.IdlePoolCleanupInterval(),
		CachedPools:                    len(pools),
		TotalPoolsCreated:              snapshot.TotalPoolsCreated,
		TotalPoolsClosed:               snapshot.TotalPoolsClosed,
		IdlePoolsEvicted:               snapshot.IdlePoolsEvicted,
		IdleCleanupRuns:                snapshot.IdleCleanupRuns,
		LastIdleCleanupAt:              snapshot.LastIdleCleanupAt,
		LastIdleEvictionAt:             snapshot.LastIdleEvictionAt,
		PendingConfirmationTokens:      m.confirmations.PendingCount(),
		Pools:                          pools,
		Operations:                     operations,
	}
	return out, nil
}

func (m *Manager) observeOperation(name string, startedAt time.Time, err error) {
	if m == nil || m.telemetry == nil {
		return
	}

	m.telemetry.RecordOperation(name, time.Since(startedAt), err)
}

func (m *Manager) poolRuntimeSnapshot() []types.PoolRuntimeInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]types.PoolRuntimeInfo, 0, len(m.pools))
	for database, entry := range m.pools {
		if entry == nil || entry.pool == nil {
			continue
		}

		stats := entry.pool.Stat()
		out = append(out, types.PoolRuntimeInfo{
			Database:      database,
			TotalConns:    stats.TotalConns(),
			IdleConns:     stats.IdleConns(),
			AcquiredConns: stats.AcquiredConns(),
			MaxConns:      int32(stats.MaxConns()),
			LastUsedAt:    entry.lastUsedAt(),
		})
	}

	slices.SortFunc(out, func(left, right types.PoolRuntimeInfo) int {
		switch {
		case left.Database < right.Database:
			return -1
		case left.Database > right.Database:
			return 1
		default:
			return 0
		}
	})

	return out
}
