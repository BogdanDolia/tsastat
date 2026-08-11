package collector

import (
	"context"
	"errors"
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

func TestCollectorUsesSchedulerEventStream(t *testing.T) {
	origin := time.Now().Add(-20 * time.Millisecond)
	stream := &fakeEventStream{
		initial: eventInitialSnapshot(origin, model.StateSleeping),
		events:  make(chan model.SchedulerEvent, 1),
		errors:  make(chan error),
		flushed: make(chan struct{}, 1),
		lost:    2,
		pending: []model.SchedulerEvent{
			schedulerEvent(origin.Add(5*time.Millisecond), model.SchedulerEventWakeup, model.SchedulerStateRunnable),
		},
	}
	b := &fakeEventBackend{stream: stream}
	c := New(b, 10, 10*time.Millisecond, time.Second)

	var got model.IntervalReport
	if err := c.Run(context.Background(), 1, func(report model.IntervalReport) error {
		got = report
		return nil
	}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !stream.closed {
		t.Fatal("event stream was not closed")
	}
	if !got.Quality.SchedulerEventTimeline || got.Quality.SchedulerLostEventsTotal != 2 {
		t.Fatalf("event quality = %#v", got.Quality)
	}
	if len(got.Threads) != 1 || got.Threads[0].Duration(model.StateSleeping) != 5*time.Millisecond ||
		got.Threads[0].RunqueueWait != 5*time.Millisecond {
		t.Fatalf("event threads = %#v", got.Threads)
	}
}

func TestCollectorCombinesSchedulerEventsAndTaskstats(t *testing.T) {
	origin := time.Now().Add(-20 * time.Millisecond)
	stream := &fakeEventStream{
		initial: eventInitialSnapshot(origin, model.StateSleeping),
		events:  make(chan model.SchedulerEvent, 1),
		errors:  make(chan error),
		flushed: make(chan struct{}, 1),
		pending: []model.SchedulerEvent{
			schedulerEvent(origin.Add(5*time.Millisecond), model.SchedulerEventWakeup, model.SchedulerStateRunnable),
		},
	}
	b := &fakeHybridBackend{
		stream: stream,
		snapshots: []model.ThreadSnapshot{
			hybridDelaySnapshot(origin.Add(time.Millisecond), 0, 0),
			hybridDelaySnapshot(origin.Add(11*time.Millisecond), 10, 100*time.Nanosecond),
		},
		status: model.BackendSourceStatus{Active: []string{"ebpf", "proc", "taskstats"}},
	}
	c := New(b, 10, 10*time.Millisecond, time.Hour)

	var got model.IntervalReport
	if err := c.Run(context.Background(), 1, func(report model.IntervalReport) error {
		got = report
		return nil
	}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if b.snapshotCalls != 2 {
		t.Fatalf("Snapshot calls = %d, want baseline and boundary", b.snapshotCalls)
	}
	if len(got.Threads) != 1 {
		t.Fatalf("threads = %#v", got.Threads)
	}
	stat := got.Threads[0]
	if stat.Duration(model.StateSleeping) != 5*time.Millisecond || stat.RunqueueWait != 5*time.Millisecond {
		t.Fatalf("scheduler timeline = sleep %s runnable %s", stat.Duration(model.StateSleeping), stat.RunqueueWait)
	}
	if !stat.Delays.CPU.Available || stat.Delays.CPU.Count != 9 || stat.Delays.CPU.Total != 90*time.Nanosecond {
		t.Fatalf("merged CPU delays = %#v", stat.Delays.CPU)
	}
	if !got.Quality.SchedulerEventTimeline || !got.Quality.TaskstatsAvailable ||
		got.Quality.SamplingMethod != "ebpf_sched_events+taskstats_counters" {
		t.Fatalf("hybrid quality = %#v", got.Quality)
	}
	if len(got.Quality.ActiveSources) != 3 || got.Quality.HybridIdentityMismatches != 0 {
		t.Fatalf("hybrid source quality = %#v", got.Quality)
	}
}

func TestCollectorHybridFallsBackToSnapshots(t *testing.T) {
	base := time.Unix(0, 0)
	b := &fakeHybridBackend{
		openErr: errors.New("eBPF unavailable"),
		snapshots: []model.ThreadSnapshot{
			snapshot(sample(1, 1, "worker", model.StateSleeping, base), base),
			snapshot(sample(1, 1, "worker", model.StateSleeping, base.Add(10*time.Millisecond)), base.Add(10*time.Millisecond)),
		},
		status: model.BackendSourceStatus{Active: []string{"proc"}, Unavailable: []string{"ebpf", "taskstats"}},
	}
	c := New(b, 10, 10*time.Millisecond, time.Nanosecond)

	var got model.IntervalReport
	if err := c.Run(context.Background(), 1, func(report model.IntervalReport) error {
		got = report
		return nil
	}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if got.Quality.SchedulerEventTimeline {
		t.Fatalf("fallback quality = %#v", got.Quality)
	}
	if len(got.Quality.ActiveSources) != 1 || len(got.Quality.UnavailableSources) != 2 {
		t.Fatalf("fallback sources = %#v", got.Quality)
	}
}

func TestHybridMergeRejectsUnprovenThreadIdentity(t *testing.T) {
	base := time.Unix(0, 0)
	eventReport := model.IntervalReport{
		IntervalStart: base,
		IntervalEnd:   base.Add(time.Second),
		Threads: []model.ThreadIntervalStats{{
			TID: 11, StartTimeNanoseconds: 456, SchedulerSource: ebpfSamplingMethod,
		}},
	}
	auxiliary := model.IntervalReport{
		IntervalStart: base,
		IntervalEnd:   base.Add(time.Second),
		Quality:       model.IntervalQuality{TaskstatsAvailable: true},
		Threads: []model.ThreadIntervalStats{{
			TID: 11, StartTimeTicks: 123, DelayVersion: 14, DelaySamplePairs: 1,
			Delays: model.DelayIntervalCounters{CPU: model.DelayIntervalCounter{Available: true, Count: 1}},
		}},
	}

	merged := mergeHybridReport(eventReport, auxiliary)
	if merged.Threads[0].DelayCountersAvailable() {
		t.Fatalf("unproven identity received delays: %#v", merged.Threads[0])
	}
	if merged.Quality.HybridIdentityMismatches != 1 {
		t.Fatalf("identity mismatches = %d, want 1", merged.Quality.HybridIdentityMismatches)
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

type fakeEventBackend struct {
	stream *fakeEventStream
}

type fakeHybridBackend struct {
	stream        *fakeEventStream
	openErr       error
	snapshots     []model.ThreadSnapshot
	snapshotCalls int
	status        model.BackendSourceStatus
}

func (b *fakeHybridBackend) Name() string {
	return "auto"
}

func (b *fakeHybridBackend) Capabilities() model.BackendCapabilities {
	return model.BackendCapabilities{SupportsSchedulerEvents: true, SupportsDelayCounters: true}
}

func (b *fakeHybridBackend) OpenSchedulerEvents(context.Context, int) (model.SchedulerEventStream, error) {
	return b.stream, b.openErr
}

func (b *fakeHybridBackend) Snapshot(context.Context, int) (model.ThreadSnapshot, error) {
	index := b.snapshotCalls
	if index >= len(b.snapshots) {
		index = len(b.snapshots) - 1
	}
	b.snapshotCalls++
	return b.snapshots[index], nil
}

func (b *fakeHybridBackend) SourceStatus() model.BackendSourceStatus {
	return b.status
}

func (b *fakeHybridBackend) Close() error {
	return nil
}

func (b *fakeEventBackend) Name() string {
	return "event"
}

func (b *fakeEventBackend) Capabilities() model.BackendCapabilities {
	return model.BackendCapabilities{SupportsSchedulerEvents: true}
}

func (b *fakeEventBackend) OpenSchedulerEvents(context.Context, int) (model.SchedulerEventStream, error) {
	return b.stream, nil
}

func (b *fakeEventBackend) Close() error {
	return nil
}

type fakeEventStream struct {
	initial model.ThreadSnapshot
	events  chan model.SchedulerEvent
	errors  chan error
	flushed chan struct{}
	lost    uint64
	pending []model.SchedulerEvent
	closed  bool
}

func (s *fakeEventStream) InitialSnapshot() model.ThreadSnapshot {
	return s.initial
}

func (s *fakeEventStream) Events() <-chan model.SchedulerEvent {
	return s.events
}

func (s *fakeEventStream) Errors() <-chan error {
	return s.errors
}

func (s *fakeEventStream) Flush() error {
	for _, event := range s.pending {
		s.events <- event
	}
	s.pending = nil
	s.flushed <- struct{}{}
	return nil
}

func (s *fakeEventStream) Flushed() <-chan struct{} {
	return s.flushed
}

func (s *fakeEventStream) LostEvents() uint64 {
	return s.lost
}

func (s *fakeEventStream) ClockCalibrationUncertainty() time.Duration {
	return 0
}

func (s *fakeEventStream) Close() error {
	s.closed = true
	return nil
}

func hybridDelaySnapshot(at time.Time, count uint64, total time.Duration) model.ThreadSnapshot {
	counter := model.DelayCounter{Available: true, Count: count, TotalNanoseconds: uint64(total)}
	return model.ThreadSnapshot{
		Samples: []model.ThreadSample{{
			PID: 10, TID: 11, StartTimeTicks: 123, Comm: "worker", State: model.StateSleeping,
			Timestamp: at,
			Delays: model.DelayCounters{
				Available: true, Version: 14, AccountingEnabled: true, AccountingEnabledKnown: true,
				CPU: counter,
			},
		}},
		StartedAt:      at,
		FinishedAt:     at,
		SamplingMethod: "hybrid_taskstats_procfs_midpoint",
	}
}
