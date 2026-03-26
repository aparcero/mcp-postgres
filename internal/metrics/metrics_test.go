package metrics

import (
	"testing"
	"time"
)

func TestCollectorSnapshot(t *testing.T) {
	startedAt := time.Date(2026, time.March, 26, 20, 0, 0, 0, time.UTC)
	collector := NewCollector(startedAt)

	collector.RecordPoolCreated()
	collector.RecordPoolCreated()
	collector.RecordPoolClosed()
	collector.RecordIdleCleanup(startedAt.Add(30*time.Second), 0)
	collector.RecordIdleCleanup(startedAt.Add(1*time.Minute), 1)
	collector.RecordOperation("postgres.query", 12*time.Millisecond, nil)
	collector.RecordOperation("postgres.query", 30*time.Millisecond, assertError{})
	collector.RecordOperation("postgres.get_connection_status", 5*time.Millisecond, nil)

	snapshot := collector.Snapshot(startedAt.Add(2 * time.Minute))
	if got, want := snapshot.UptimeSeconds, int64(120); got != want {
		t.Fatalf("UptimeSeconds = %d, want %d", got, want)
	}
	if got, want := snapshot.TotalPoolsCreated, int64(2); got != want {
		t.Fatalf("TotalPoolsCreated = %d, want %d", got, want)
	}
	if got, want := snapshot.TotalPoolsClosed, int64(1); got != want {
		t.Fatalf("TotalPoolsClosed = %d, want %d", got, want)
	}
	if got, want := snapshot.IdleCleanupRuns, int64(2); got != want {
		t.Fatalf("IdleCleanupRuns = %d, want %d", got, want)
	}
	if got, want := snapshot.IdlePoolsEvicted, int64(1); got != want {
		t.Fatalf("IdlePoolsEvicted = %d, want %d", got, want)
	}
	if snapshot.LastIdleCleanupAt == nil {
		t.Fatal("LastIdleCleanupAt = nil, want non-nil")
	}
	if snapshot.LastIdleEvictionAt == nil {
		t.Fatal("LastIdleEvictionAt = nil, want non-nil")
	}
	if len(snapshot.Operations) != 2 {
		t.Fatalf("len(Operations) = %d, want 2", len(snapshot.Operations))
	}
	if got, want := snapshot.Operations[0].Name, "postgres.get_connection_status"; got != want {
		t.Fatalf("Operations[0].Name = %q, want %q", got, want)
	}
	if got, want := snapshot.Operations[1].Name, "postgres.query"; got != want {
		t.Fatalf("Operations[1].Name = %q, want %q", got, want)
	}
	if got, want := snapshot.Operations[1].Requests, int64(2); got != want {
		t.Fatalf("query Requests = %d, want %d", got, want)
	}
	if got, want := snapshot.Operations[1].Successes, int64(1); got != want {
		t.Fatalf("query Successes = %d, want %d", got, want)
	}
	if got, want := snapshot.Operations[1].Failures, int64(1); got != want {
		t.Fatalf("query Failures = %d, want %d", got, want)
	}
	if got, want := snapshot.Operations[1].AverageDurationMs, int64(21); got != want {
		t.Fatalf("query AverageDurationMs = %d, want %d", got, want)
	}
	if got, want := snapshot.Operations[1].LastDurationMs, int64(30); got != want {
		t.Fatalf("query LastDurationMs = %d, want %d", got, want)
	}
}

type assertError struct{}

func (assertError) Error() string {
	return "boom"
}
