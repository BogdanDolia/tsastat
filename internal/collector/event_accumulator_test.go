package collector

import (
	"testing"
	"time"

	"github.com/BogdanDolia/tsastat/internal/model"
)

func TestEventAccumulatorBuildsCompleteSchedulerTimeline(t *testing.T) {
	base := time.Unix(0, 0)
	acc := NewEventAccumulator(time.Second, eventInitialSnapshot(base, model.StateSleeping))

	events := []model.SchedulerEvent{
		schedulerEvent(base.Add(100*time.Millisecond), model.SchedulerEventWakeup, model.SchedulerStateRunnable),
		schedulerEvent(base.Add(150*time.Millisecond), model.SchedulerEventSwitchIn, model.SchedulerStateOnCPU),
		schedulerEvent(base.Add(400*time.Millisecond), model.SchedulerEventSwitchOut, model.SchedulerStateRunnable),
		schedulerEvent(base.Add(500*time.Millisecond), model.SchedulerEventSwitchIn, model.SchedulerStateOnCPU),
		schedulerEvent(base.Add(700*time.Millisecond), model.SchedulerEventSwitchOut, model.SchedulerStateUninterruptible),
		schedulerEvent(base.Add(900*time.Millisecond), model.SchedulerEventWakeup, model.SchedulerStateRunnable),
		schedulerEvent(base.Add(950*time.Millisecond), model.SchedulerEventSwitchIn, model.SchedulerStateOnCPU),
	}
	for _, event := range events {
		acc.Observe(event)
	}

	stat := onlyThread(t, acc.Advance(base.Add(time.Second), 0))
	if stat.OnCPU != 500*time.Millisecond {
		t.Fatalf("on-CPU = %s, want 500ms", stat.OnCPU)
	}
	if stat.RunqueueWait != 200*time.Millisecond {
		t.Fatalf("runqueue wait = %s, want 200ms", stat.RunqueueWait)
	}
	if stat.Duration(model.StateSleeping) != 100*time.Millisecond {
		t.Fatalf("sleep = %s, want 100ms", stat.Duration(model.StateSleeping))
	}
	if stat.Duration(model.StateUninterruptible) != 200*time.Millisecond {
		t.Fatalf("D-state = %s, want 200ms", stat.Duration(model.StateUninterruptible))
	}
	if stat.Duration(model.StateRunning) != 700*time.Millisecond {
		t.Fatalf("combined R time = %s, want 700ms", stat.Duration(model.StateRunning))
	}
	if stat.TotalObserved != time.Second || stat.SchedulerObserved != time.Second {
		t.Fatalf("observed = %s/%s, want 1s/1s", stat.TotalObserved, stat.SchedulerObserved)
	}
	if stat.Timeslices != 3 || stat.SchedulerEventCount != len(events) {
		t.Fatalf("timeslices/events = %d/%d, want 3/%d", stat.Timeslices, stat.SchedulerEventCount, len(events))
	}
	if stat.WakeupCount != 2 || stat.WakeupLatencyTotal != 100*time.Millisecond || stat.WakeupLatencyMax != 50*time.Millisecond {
		t.Fatalf("wakeup metrics = count %d total %s max %s", stat.WakeupCount, stat.WakeupLatencyTotal, stat.WakeupLatencyMax)
	}
	if !stat.SchedulerAvailable() || stat.SchedulerSource != ebpfSamplingMethod {
		t.Fatalf("scheduler source = %q available=%t", stat.SchedulerSource, stat.SchedulerAvailable())
	}
}

func TestEventAccumulatorSplitsStateAtExactWindowBoundary(t *testing.T) {
	base := time.Unix(0, 0)
	acc := NewEventAccumulator(time.Second, eventInitialSnapshot(base, model.StateSleeping))
	acc.Observe(schedulerEvent(base.Add(900*time.Millisecond), model.SchedulerEventSwitchIn, model.SchedulerStateOnCPU))
	first := onlyThread(t, acc.Advance(base.Add(time.Second), 0))
	if first.OnCPU != 100*time.Millisecond || first.Duration(model.StateSleeping) != 900*time.Millisecond {
		t.Fatalf("first window = CPU %s sleep %s", first.OnCPU, first.Duration(model.StateSleeping))
	}

	acc.Observe(schedulerEvent(base.Add(1100*time.Millisecond), model.SchedulerEventSwitchOut, model.SchedulerStateSleeping))
	second := onlyThread(t, acc.Advance(base.Add(2*time.Second), 0))
	if second.OnCPU != 100*time.Millisecond || second.Duration(model.StateSleeping) != 900*time.Millisecond {
		t.Fatalf("second window = CPU %s sleep %s", second.OnCPU, second.Duration(model.StateSleeping))
	}
}

func TestEventAccumulatorReportsLostAndLateEvents(t *testing.T) {
	base := time.Unix(0, 0)
	acc := NewEventAccumulator(time.Second, eventInitialSnapshot(base, model.StateSleeping))
	first := acc.Advance(base.Add(time.Second), 3)
	if len(first) != 1 || first[0].Quality.SchedulerLostEventsTotal != 3 || !first[0].Quality.MissedTransitionsPossible {
		t.Fatalf("first quality = %#v", first)
	}

	acc.Observe(schedulerEvent(base.Add(900*time.Millisecond), model.SchedulerEventWakeup, model.SchedulerStateRunnable))
	lateNewThread := schedulerEvent(base.Add(800*time.Millisecond), model.SchedulerEventWakeup, model.SchedulerStateRunnable)
	lateNewThread.TID = 99
	lateNewThread.StartTimeNanoseconds = 999
	acc.Observe(lateNewThread)
	second := acc.Advance(base.Add(2*time.Second), 3)
	if len(second) != 1 || second[0].Quality.SchedulerLateEvents != 2 || !second[0].Quality.MissedTransitionsPossible {
		t.Fatalf("second quality = %#v", second)
	}
}

func TestEventAccumulatorDoesNotGuessInitialRunningState(t *testing.T) {
	base := time.Unix(0, 0)
	acc := NewEventAccumulator(time.Second, eventInitialSnapshot(base, model.StateRunning))
	acc.Observe(schedulerEvent(base.Add(100*time.Millisecond), model.SchedulerEventSwitchOut, model.SchedulerStateRunnable))
	acc.Observe(schedulerEvent(base.Add(200*time.Millisecond), model.SchedulerEventSwitchIn, model.SchedulerStateOnCPU))

	stat := onlyThread(t, acc.Advance(base.Add(time.Second), 0))
	if stat.UnknownDuration() != 100*time.Millisecond {
		t.Fatalf("initial unknown = %s, want 100ms", stat.UnknownDuration())
	}
	if stat.RunqueueWait != 100*time.Millisecond || stat.OnCPU != 800*time.Millisecond {
		t.Fatalf("runnable/on-CPU = %s/%s, want 100ms/800ms", stat.RunqueueWait, stat.OnCPU)
	}
}

func TestEventAccumulatorCarriesNewThreadProcIdentity(t *testing.T) {
	base := time.Unix(0, 0)
	acc := NewEventAccumulator(time.Second, model.ThreadSnapshot{StartedAt: base, FinishedAt: base})
	event := schedulerEvent(base.Add(100*time.Millisecond), model.SchedulerEventSwitchIn, model.SchedulerStateOnCPU)
	event.TID = 99
	event.StartTimeTicks = 789
	event.StartTimeNanoseconds = 456
	acc.Observe(event)

	stat := onlyThread(t, acc.Advance(base.Add(time.Second), 0))
	if stat.StartTimeTicks != 789 || stat.StartTimeNanoseconds != 456 {
		t.Fatalf("new thread identity = ticks %d ns %d", stat.StartTimeTicks, stat.StartTimeNanoseconds)
	}
}

func TestEventAccumulatorMarksWakeupWithoutSwitchInIncomplete(t *testing.T) {
	base := time.Unix(0, 0)
	acc := NewEventAccumulator(time.Second, eventInitialSnapshot(base, model.StateSleeping))
	acc.Observe(schedulerEvent(base.Add(100*time.Millisecond), model.SchedulerEventWakeup, model.SchedulerStateRunnable))
	acc.Observe(schedulerEvent(base.Add(300*time.Millisecond), model.SchedulerEventSwitchOut, model.SchedulerStateSleeping))

	reports := acc.Advance(base.Add(time.Second), 0)
	stat := onlyThread(t, reports)
	if stat.WakeupCount != 0 || stat.IncompleteWakeupCount != 1 {
		t.Fatalf("wakeup counts = completed %d incomplete %d, want 0/1", stat.WakeupCount, stat.IncompleteWakeupCount)
	}
	if reports[0].Quality.SchedulerIncompleteWakeups != 1 || !reports[0].Quality.MissedTransitionsPossible {
		t.Fatalf("incomplete quality = %#v", reports[0].Quality)
	}
}

func eventInitialSnapshot(at time.Time, state model.ThreadState) model.ThreadSnapshot {
	return model.ThreadSnapshot{
		Samples: []model.ThreadSample{{
			PID:            10,
			TID:            11,
			StartTimeTicks: 123,
			Comm:           "worker",
			State:          state,
			Timestamp:      at,
		}},
		StartedAt:  at,
		FinishedAt: at,
	}
}

func schedulerEvent(at time.Time, kind model.SchedulerEventKind, state model.SchedulerState) model.SchedulerEvent {
	return model.SchedulerEvent{
		Timestamp:            at,
		PID:                  10,
		TID:                  11,
		StartTimeTicks:       123,
		StartTimeNanoseconds: 456,
		Comm:                 "worker",
		CPU:                  2,
		Kind:                 kind,
		State:                state,
	}
}
