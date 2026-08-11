package hybrid

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/BogdanDolia/tsastat/internal/model"
)

func TestSnapshotCombinesTaskstatsAndProcByStableIdentity(t *testing.T) {
	base := time.Unix(100, 0)
	delays := model.DelayCounters{
		Available: true,
		Version:   14,
		CPU:       model.DelayCounter{Available: true, Count: 2, TotalNanoseconds: 50},
	}
	stats := &fakeSnapshotSource{snapshot: model.ThreadSnapshot{
		Samples: []model.ThreadSample{{
			PID: 7, TID: 8, StartTimeTicks: 9, Comm: "worker", State: model.StateSleeping,
			Timestamp: base.Add(time.Millisecond), Delays: delays,
		}},
		StartedAt:  base,
		FinishedAt: base.Add(2 * time.Millisecond),
	}}
	procSource := &fakeSnapshotSource{snapshot: model.ThreadSnapshot{
		Samples: []model.ThreadSample{{
			PID: 7, TID: 8, StartTimeTicks: 9, Comm: "worker", State: model.StateRunning,
			Timestamp: base.Add(5 * time.Millisecond),
			Schedstat: model.SchedstatCounters{Available: true, OnCPUNanoseconds: 100},
		}},
		StartedAt:  base.Add(3 * time.Millisecond),
		FinishedAt: base.Add(6 * time.Millisecond),
	}}
	b := newWithSources("auto", &fakeSchedulerSource{}, stats, procSource)

	snapshot, err := b.Snapshot(context.Background(), 7)
	if err != nil {
		t.Fatalf("Snapshot returned error: %v", err)
	}
	if snapshot.SamplingMethod != hybridSamplingMethod || len(snapshot.Samples) != 1 {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	sample := snapshot.Samples[0]
	if !sample.Delays.Available || !sample.Schedstat.Available || sample.State != model.StateRunning {
		t.Fatalf("merged sample = %#v", sample)
	}
	if want := base.Add(3 * time.Millisecond); !sample.Timestamp.Equal(want) {
		t.Fatalf("sample timestamp = %s, want %s", sample.Timestamp, want)
	}
	if !snapshot.StartedAt.Equal(base) || !snapshot.FinishedAt.Equal(base.Add(6*time.Millisecond)) {
		t.Fatalf("scan bounds = [%s,%s]", snapshot.StartedAt, snapshot.FinishedAt)
	}
	status := b.SourceStatus()
	if !reflect.DeepEqual(status.Active, []string{"proc", "taskstats"}) || len(status.Unavailable) != 0 {
		t.Fatalf("source status = %#v", status)
	}
}

func TestSnapshotDoesNotMergeReusedTID(t *testing.T) {
	base := time.Unix(100, 0)
	stats := &fakeSnapshotSource{snapshot: model.ThreadSnapshot{Samples: []model.ThreadSample{{
		PID: 7, TID: 8, StartTimeTicks: 9, Timestamp: base,
		Delays: model.DelayCounters{Available: true},
	}}}}
	procSource := &fakeSnapshotSource{snapshot: model.ThreadSnapshot{Samples: []model.ThreadSample{{
		PID: 7, TID: 8, StartTimeTicks: 10, Timestamp: base,
		Schedstat: model.SchedstatCounters{Available: true},
	}}}}
	b := newWithSources("auto", &fakeSchedulerSource{}, stats, procSource)

	snapshot, err := b.Snapshot(context.Background(), 7)
	if err != nil {
		t.Fatalf("Snapshot returned error: %v", err)
	}
	if len(snapshot.Samples) != 2 {
		t.Fatalf("samples = %#v, want separate identities", snapshot.Samples)
	}
}

func TestBackendFallsBackAndReportsUnavailableSources(t *testing.T) {
	stats := &fakeSnapshotSource{err: errors.New("taskstats permission denied")}
	procSource := &fakeSnapshotSource{snapshot: model.ThreadSnapshot{Samples: []model.ThreadSample{{TID: 8, StartTimeTicks: 9}}}}
	b := newWithSources("auto", &fakeSchedulerSource{err: errors.New("bpf permission denied")}, stats, procSource)

	if _, err := b.OpenSchedulerEvents(context.Background(), 7); err == nil {
		t.Fatal("OpenSchedulerEvents returned nil error")
	}
	snapshot, err := b.Snapshot(context.Background(), 7)
	if err != nil {
		t.Fatalf("Snapshot returned error: %v", err)
	}
	if snapshot.SamplingMethod != procSamplingMethod {
		t.Fatalf("sampling method = %q, want proc fallback", snapshot.SamplingMethod)
	}
	status := b.SourceStatus()
	if !reflect.DeepEqual(status.Active, []string{"proc"}) || !reflect.DeepEqual(status.Unavailable, []string{"ebpf", "taskstats"}) {
		t.Fatalf("source status = %#v", status)
	}
	if stats.calls != 1 || procSource.calls != 1 {
		t.Fatalf("source calls = taskstats %d proc %d", stats.calls, procSource.calls)
	}
	if _, err := b.Snapshot(context.Background(), 7); err != nil {
		t.Fatalf("second Snapshot returned error: %v", err)
	}
	if stats.calls != 1 {
		t.Fatalf("disabled taskstats calls = %d, want 1", stats.calls)
	}
}

type fakeSchedulerSource struct {
	err error
}

func (s *fakeSchedulerSource) OpenSchedulerEvents(context.Context, int) (model.SchedulerEventStream, error) {
	return nil, s.err
}

func (s *fakeSchedulerSource) Close() error {
	return nil
}

type fakeSnapshotSource struct {
	snapshot model.ThreadSnapshot
	err      error
	calls    int
}

func (s *fakeSnapshotSource) Snapshot(context.Context, int) (model.ThreadSnapshot, error) {
	s.calls++
	return s.snapshot, s.err
}

func (s *fakeSnapshotSource) Close() error {
	return nil
}
