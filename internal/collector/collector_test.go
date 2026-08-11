package collector

import (
	"context"
	"testing"
	"time"

	"github.com/BogdanDolia/tsastat/internal/model"
)

func TestCollectorSamplesRepeatedlyBeforeFixedReport(t *testing.T) {
	base := time.Unix(0, 0)
	b := &sequenceBackend{snapshots: []model.ThreadSnapshot{
		snapshot(sample(1, 1, "worker", model.StateRunning, base), base),
		snapshot(sample(1, 1, "worker", model.StateSleeping, base.Add(10*time.Millisecond)), base.Add(10*time.Millisecond)),
		snapshot(sample(1, 1, "worker", model.StateSleeping, base.Add(20*time.Millisecond)), base.Add(20*time.Millisecond)),
		snapshot(sample(1, 1, "worker", model.StateSleeping, base.Add(30*time.Millisecond)), base.Add(30*time.Millisecond)),
	}}

	c := New(b, 99, 30*time.Millisecond, time.Nanosecond)
	var got model.IntervalReport
	err := c.Run(context.Background(), 1, func(report model.IntervalReport) error {
		got = report
		return nil
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if b.calls != 4 {
		t.Fatalf("Snapshot calls = %d, want 4", b.calls)
	}
	if got.IntervalEnd.Sub(got.IntervalStart) != 30*time.Millisecond {
		t.Fatalf("report duration = %s, want 30ms", got.IntervalEnd.Sub(got.IntervalStart))
	}
	if len(got.Threads) != 1 {
		t.Fatalf("reported stats = %d, want 1", len(got.Threads))
	}
	if got.Threads[0].Duration(model.StateRunning) != 5*time.Millisecond {
		t.Fatalf("running duration = %s, want midpoint estimate 5ms", got.Threads[0].Duration(model.StateRunning))
	}
	if got.Threads[0].Duration(model.StateSleeping) != 25*time.Millisecond {
		t.Fatalf("sleeping duration = %s, want 25ms", got.Threads[0].Duration(model.StateSleeping))
	}
}

type sequenceBackend struct {
	snapshots []model.ThreadSnapshot
	calls     int
}

func (b *sequenceBackend) Name() string {
	return "sequence"
}

func (b *sequenceBackend) Capabilities() model.BackendCapabilities {
	return model.BackendCapabilities{}
}

func (b *sequenceBackend) Snapshot(context.Context, int) (model.ThreadSnapshot, error) {
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
