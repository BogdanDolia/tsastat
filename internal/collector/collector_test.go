package collector

import (
	"context"
	"testing"
	"time"

	"tsastat/internal/model"
)

func TestCollectorSamplesRepeatedlyBeforeReport(t *testing.T) {
	base := time.Unix(0, 0)
	b := &sequenceBackend{snapshots: [][]model.ThreadSample{
		{sample(1, "worker", model.StateRunning, base)},
		{sample(1, "worker", model.StateSleeping, base.Add(10*time.Millisecond))},
		{sample(1, "worker", model.StateSleeping, base.Add(20*time.Millisecond))},
		{sample(1, "worker", model.StateSleeping, base.Add(30*time.Millisecond))},
	}}

	c := New(b, 99, 30*time.Millisecond, time.Nanosecond)
	var got []model.ThreadIntervalStats
	err := c.Run(context.Background(), 1, func(stats []model.ThreadIntervalStats) error {
		got = stats
		return nil
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if b.calls != 4 {
		t.Fatalf("Snapshot calls = %d, want 4", b.calls)
	}
	if len(got) != 1 {
		t.Fatalf("reported stats = %d, want 1", len(got))
	}
	if got[0].Duration(model.StateRunning) != 10*time.Millisecond {
		t.Fatalf("running duration = %s, want 10ms", got[0].Duration(model.StateRunning))
	}
	if got[0].Duration(model.StateSleeping) != 20*time.Millisecond {
		t.Fatalf("sleeping duration = %s, want 20ms", got[0].Duration(model.StateSleeping))
	}
}

type sequenceBackend struct {
	snapshots [][]model.ThreadSample
	calls     int
}

func (b *sequenceBackend) Name() string {
	return "sequence"
}

func (b *sequenceBackend) Capabilities() model.BackendCapabilities {
	return model.BackendCapabilities{}
}

func (b *sequenceBackend) Snapshot(context.Context, int) ([]model.ThreadSample, error) {
	index := b.calls
	if index >= len(b.snapshots) {
		index = len(b.snapshots) - 1
	}
	b.calls++
	return b.snapshots[index], nil
}

func (b *sequenceBackend) Close() error {
	return nil
}
