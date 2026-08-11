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
