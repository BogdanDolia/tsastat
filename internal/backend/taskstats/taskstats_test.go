package taskstats

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/BogdanDolia/tsastat/internal/model"
)

type fakeStatsClient struct {
	stats   map[int]model.DelayCounters
	seen    []int
	onStats func(int)
}

func (c *fakeStatsClient) Stats(_ context.Context, tid int, _, _ bool) (model.DelayCounters, error) {
	c.seen = append(c.seen, tid)
	if c.onStats != nil {
		c.onStats(tid)
	}
	stats, ok := c.stats[tid]
	if !ok {
		return model.DelayCounters{}, os.ErrNotExist
	}
	return stats, nil
}

func TestSnapshotRetriesWhenTIDIdentityChangesAroundTaskstatsQuery(t *testing.T) {
	root := t.TempDir()
	statDir := filepath.Join(root, "123", "task", "124")
	if err := os.MkdirAll(statDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	statPath := filepath.Join(statDir, "stat")
	writeStat := func(start uint64, comm string) {
		t.Helper()
		line := fmt.Sprintf("124 (%s) S 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 %d", comm, start)
		if err := os.WriteFile(statPath, []byte(line), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}
	writeStat(100, "old")
	client := &fakeStatsClient{
		stats: map[int]model.DelayCounters{124: {Available: true, Version: 14}},
	}
	client.onStats = func(int) {
		if len(client.seen) == 1 {
			writeStat(200, "new")
		}
	}

	snapshot, err := newWithClient(root, client, true, true).Snapshot(context.Background(), 123)
	if err != nil {
		t.Fatalf("Snapshot returned error: %v", err)
	}
	if len(snapshot.Samples) != 1 || snapshot.Samples[0].StartTimeTicks != 200 || snapshot.Samples[0].Comm != "new" {
		t.Fatalf("samples = %#v, want retried identity", snapshot.Samples)
	}
	if len(client.seen) != 2 {
		t.Fatalf("taskstats queries = %v, want two attempts", client.seen)
	}
}

func (c *fakeStatsClient) Close() error {
	return nil
}

func TestSnapshotCombinesProcIdentityWithPerTIDTaskstats(t *testing.T) {
	root := t.TempDir()
	statDir := filepath.Join(root, "123", "task", "124")
	if err := os.MkdirAll(statDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	line := fmt.Sprintf("124 (worker thread) S 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 %d", 987654)
	if err := os.WriteFile(filepath.Join(statDir, "stat"), []byte(line), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	delays := model.DelayCounters{
		Available: true,
		Version:   14,
		CPU:       model.DelayCounter{Available: true, Count: 2, TotalNanoseconds: 30},
	}
	client := &fakeStatsClient{stats: map[int]model.DelayCounters{124: delays}}
	backend := newWithClient(root, client, true, true)

	snapshot, err := backend.Snapshot(context.Background(), 123)
	if err != nil {
		t.Fatalf("Snapshot returned error: %v", err)
	}
	if snapshot.SamplingMethod != samplingMethod || len(snapshot.Samples) != 1 {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	sample := snapshot.Samples[0]
	if sample.TID != 124 || sample.StartTimeTicks != 987654 || sample.Comm != "worker thread" || sample.State != model.StateSleeping {
		t.Fatalf("proc identity = %#v", sample)
	}
	if sample.Delays.Version != 14 || sample.Delays.CPU.Count != 2 || len(client.seen) != 1 || client.seen[0] != 124 {
		t.Fatalf("taskstats sample/client = %#v/%v", sample.Delays, client.seen)
	}
	if sample.Timestamp.Before(snapshot.StartedAt) || sample.Timestamp.After(snapshot.FinishedAt) {
		t.Fatalf("sample timestamp %s outside scan [%s,%s]", sample.Timestamp, snapshot.StartedAt, snapshot.FinishedAt)
	}
}

func TestSnapshotSkipsThreadWhichExitsBeforeTaskstatsQuery(t *testing.T) {
	root := t.TempDir()
	statDir := filepath.Join(root, "123", "task", "124")
	if err := os.MkdirAll(statDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	line := "124 (worker) S 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 987654"
	if err := os.WriteFile(filepath.Join(statDir, "stat"), []byte(line), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	backend := newWithClient(root, &fakeStatsClient{stats: map[int]model.DelayCounters{}}, false, true)

	snapshot, err := backend.Snapshot(context.Background(), 123)
	if err != nil {
		t.Fatalf("Snapshot returned error: %v", err)
	}
	if len(snapshot.Samples) != 0 {
		t.Fatalf("samples = %#v, want exited thread skipped", snapshot.Samples)
	}
}

func TestCapabilitiesDescribeDelayCounterSemantics(t *testing.T) {
	capabilities := Capabilities()
	if !capabilities.SupportsThreadStates || !capabilities.SupportsDelayCounters || capabilities.SupportsSchedulerEvents {
		t.Fatalf("capabilities = %#v", capabilities)
	}
}
