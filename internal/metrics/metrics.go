package metrics

import (
	"sort"
	"sync"
	"time"
)

type OperationSnapshot struct {
	Name              string
	Requests          int64
	Successes         int64
	Failures          int64
	AverageDurationMs int64
	LastDurationMs    int64
}

type Snapshot struct {
	StartedAt          time.Time
	UptimeSeconds      int64
	TotalPoolsCreated  int64
	TotalPoolsClosed   int64
	IdlePoolsEvicted   int64
	IdleCleanupRuns    int64
	LastIdleCleanupAt  *time.Time
	LastIdleEvictionAt *time.Time
	Operations         []OperationSnapshot
}

type Collector struct {
	startedAt time.Time

	mu sync.Mutex

	totalPoolsCreated  int64
	totalPoolsClosed   int64
	idlePoolsEvicted   int64
	idleCleanupRuns    int64
	lastIdleCleanupAt  time.Time
	lastIdleEvictionAt time.Time
	operations         map[string]*operationTotals
}

type operationTotals struct {
	requests        int64
	successes       int64
	failures        int64
	totalDurationMs int64
	lastDurationMs  int64
}

func NewCollector(now time.Time) *Collector {
	if now.IsZero() {
		now = time.Now().UTC()
	}

	return &Collector{
		startedAt:  now,
		operations: make(map[string]*operationTotals),
	}
}

func (c *Collector) RecordOperation(name string, duration time.Duration, err error) {
	if c == nil || name == "" {
		return
	}

	durationMs := duration.Milliseconds()
	if durationMs < 0 {
		durationMs = 0
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	item, ok := c.operations[name]
	if !ok {
		item = &operationTotals{}
		c.operations[name] = item
	}

	item.requests++
	item.totalDurationMs += durationMs
	item.lastDurationMs = durationMs
	if err != nil {
		item.failures++
		return
	}

	item.successes++
}

func (c *Collector) RecordPoolCreated() {
	if c == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.totalPoolsCreated++
}

func (c *Collector) RecordPoolClosed() {
	if c == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.totalPoolsClosed++
}

func (c *Collector) RecordIdleCleanup(now time.Time, evicted int) {
	if c == nil {
		return
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if evicted < 0 {
		evicted = 0
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.idleCleanupRuns++
	c.lastIdleCleanupAt = now
	if evicted > 0 {
		c.idlePoolsEvicted += int64(evicted)
		c.lastIdleEvictionAt = now
	}
}

func (c *Collector) Snapshot(now time.Time) Snapshot {
	if c == nil {
		return Snapshot{}
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	operations := make([]OperationSnapshot, 0, len(c.operations))
	for name, totals := range c.operations {
		average := int64(0)
		if totals.requests > 0 {
			average = totals.totalDurationMs / totals.requests
		}
		operations = append(operations, OperationSnapshot{
			Name:              name,
			Requests:          totals.requests,
			Successes:         totals.successes,
			Failures:          totals.failures,
			AverageDurationMs: average,
			LastDurationMs:    totals.lastDurationMs,
		})
	}
	sort.Slice(operations, func(i, j int) bool {
		return operations[i].Name < operations[j].Name
	})

	return Snapshot{
		StartedAt:          c.startedAt,
		UptimeSeconds:      int64(now.Sub(c.startedAt).Seconds()),
		TotalPoolsCreated:  c.totalPoolsCreated,
		TotalPoolsClosed:   c.totalPoolsClosed,
		IdlePoolsEvicted:   c.idlePoolsEvicted,
		IdleCleanupRuns:    c.idleCleanupRuns,
		LastIdleCleanupAt:  timePtr(c.lastIdleCleanupAt),
		LastIdleEvictionAt: timePtr(c.lastIdleEvictionAt),
		Operations:         operations,
	}
}

func timePtr(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}

	copyValue := value
	return &copyValue
}
