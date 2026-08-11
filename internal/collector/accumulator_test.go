package collector

import (
	"testing"
	"time"

	"github.com/BogdanDolia/tsastat/internal/model"
)

func TestAccumulatorKeepsExactWindowsWithoutDrift(t *testing.T) {
	base := time.Unix(0, 0)
	acc := NewAccumulator(time.Second)

	acc.Observe(snapshot(sample(1, 1, "worker", model.StateRunning, base), base))
	reports := acc.Observe(snapshot(sample(1, 1, "worker", model.StateRunning, base.Add(1100*time.Millisecond)), base.Add(1100*time.Millisecond)))
	if len(reports) != 1 {
		t.Fatalf("first completed reports = %d, want 1", len(reports))
	}
	assertWindow(t, reports[0], base, base.Add(time.Second))
	if got := reports[0].Threads[0].Duration(model.StateRunning); got != time.Second {
		t.Fatalf("first running duration = %s, want 1s", got)
	}

	reports = acc.Observe(snapshot(sample(1, 1, "worker", model.StateRunning, base.Add(2200*time.Millisecond)), base.Add(2200*time.Millisecond)))
	if len(reports) != 1 {
		t.Fatalf("second completed reports = %d, want 1", len(reports))
	}
	assertWindow(t, reports[0], base.Add(time.Second), base.Add(2*time.Second))
	if got := reports[0].Threads[0].Duration(model.StateRunning); got != time.Second {
		t.Fatalf("second running duration = %s, want 1s", got)
	}
}

func TestAccumulatorUsesMidpointAndReportsUncertainty(t *testing.T) {
	base := time.Unix(0, 0)
	acc := NewAccumulator(time.Second)

	acc.Observe(snapshot(sample(1, 1, "worker", model.StateRunning, base), base))
	acc.Observe(snapshot(sample(1, 1, "worker", model.StateSleeping, base.Add(100*time.Millisecond)), base.Add(100*time.Millisecond)))
	reports := acc.Observe(snapshot(sample(1, 1, "worker", model.StateSleeping, base.Add(time.Second)), base.Add(time.Second)))

	stat := onlyThread(t, reports)
	if got := stat.Duration(model.StateRunning); got != 50*time.Millisecond {
		t.Fatalf("running duration = %s, want 50ms", got)
	}
	if got := stat.Duration(model.StateSleeping); got != 950*time.Millisecond {
		t.Fatalf("sleeping duration = %s, want 950ms", got)
	}
	if stat.DetectedTransitions != 1 {
		t.Fatalf("detected transitions = %d, want 1", stat.DetectedTransitions)
	}
	if stat.DetectedTransitionUncertainty != 50*time.Millisecond {
		t.Fatalf("transition uncertainty = %s, want 50ms", stat.DetectedTransitionUncertainty)
	}
	if stat.MaxSampleGap != 900*time.Millisecond {
		t.Fatalf("max sample gap = %s, want 900ms", stat.MaxSampleGap)
	}
	if !reports[0].Quality.MissedTransitionsPossible {
		t.Fatal("MissedTransitionsPossible = false, want true")
	}
}

func TestAccumulatorCarriesTransitionUncertaintyAcrossWindowBoundary(t *testing.T) {
	base := time.Unix(0, 0)
	acc := NewAccumulator(time.Second)

	acc.Observe(snapshot(sample(1, 1, "worker", model.StateRunning, base), base))
	acc.Observe(snapshot(sample(1, 1, "worker", model.StateRunning, base.Add(900*time.Millisecond)), base.Add(900*time.Millisecond)))
	first := acc.Observe(snapshot(sample(1, 1, "worker", model.StateSleeping, base.Add(1100*time.Millisecond)), base.Add(1100*time.Millisecond)))
	firstStat := onlyThread(t, first)
	if firstStat.DetectedTransitionUncertainty != 100*time.Millisecond {
		t.Fatalf("first window uncertainty = %s, want 100ms", firstStat.DetectedTransitionUncertainty)
	}

	second := acc.Observe(snapshot(sample(1, 1, "worker", model.StateSleeping, base.Add(2*time.Second)), base.Add(2*time.Second)))
	secondStat := onlyThread(t, second)
	if secondStat.DetectedTransitionUncertainty != 100*time.Millisecond {
		t.Fatalf("second window uncertainty = %s, want 100ms", secondStat.DetectedTransitionUncertainty)
	}
}

func TestAccumulatorUsesExactSchedstatCounterDeltas(t *testing.T) {
	base := time.Unix(0, 0)
	acc := NewAccumulator(time.Second)

	acc.Observe(snapshot(schedstatSample(1, 1, "worker", model.StateRunning, base, 100*time.Millisecond, 50*time.Millisecond, 10), base))
	reports := acc.Observe(snapshot(schedstatSample(1, 1, "worker", model.StateRunning, base.Add(time.Second), 400*time.Millisecond, 250*time.Millisecond, 15), base.Add(time.Second)))
	stat := onlyThread(t, reports)

	if stat.OnCPU != 300*time.Millisecond {
		t.Fatalf("on-CPU = %s, want 300ms", stat.OnCPU)
	}
	if stat.RunqueueWait != 200*time.Millisecond {
		t.Fatalf("runqueue wait = %s, want 200ms", stat.RunqueueWait)
	}
	if stat.Timeslices != 5 {
		t.Fatalf("timeslices = %d, want 5", stat.Timeslices)
	}
	if stat.SchedstatObserved != time.Second || stat.SchedstatSamplePairs != 1 {
		t.Fatalf("schedstat coverage/pairs = %s/%d, want 1s/1", stat.SchedstatObserved, stat.SchedstatSamplePairs)
	}
	if !reports[0].Quality.SchedstatAvailable || reports[0].Quality.SchedstatThreadCount != 1 {
		t.Fatalf("schedstat interval quality = %#v", reports[0].Quality)
	}
}

func TestAccumulatorConservesSchedstatDeltaAcrossWindowBoundary(t *testing.T) {
	base := time.Unix(0, 0)
	acc := NewAccumulator(time.Second)

	acc.Observe(snapshot(schedstatSample(1, 1, "worker", model.StateRunning, base, 0, 0, 0), base))
	acc.Observe(snapshot(schedstatSample(1, 1, "worker", model.StateRunning, base.Add(900*time.Millisecond), 0, 0, 0), base.Add(900*time.Millisecond)))
	first := acc.Observe(snapshot(schedstatSample(1, 1, "worker", model.StateRunning, base.Add(1100*time.Millisecond), 200*time.Millisecond, 100*time.Millisecond, 2), base.Add(1100*time.Millisecond)))
	firstStat := onlyThread(t, first)
	if firstStat.OnCPU != 100*time.Millisecond || firstStat.RunqueueWait != 50*time.Millisecond || firstStat.Timeslices != 1 {
		t.Fatalf("first schedstat allocation = CPU %s RQ %s slices %d",
			firstStat.OnCPU, firstStat.RunqueueWait, firstStat.Timeslices)
	}

	second := acc.Observe(snapshot(schedstatSample(1, 1, "worker", model.StateRunning, base.Add(2*time.Second), 200*time.Millisecond, 100*time.Millisecond, 2), base.Add(2*time.Second)))
	secondStat := onlyThread(t, second)
	if secondStat.OnCPU != 100*time.Millisecond || secondStat.RunqueueWait != 50*time.Millisecond || secondStat.Timeslices != 1 {
		t.Fatalf("second schedstat allocation = CPU %s RQ %s slices %d",
			secondStat.OnCPU, secondStat.RunqueueWait, secondStat.Timeslices)
	}
	if firstStat.OnCPU+secondStat.OnCPU != 200*time.Millisecond || firstStat.RunqueueWait+secondStat.RunqueueWait != 100*time.Millisecond {
		t.Fatal("schedstat delta was not conserved across windows")
	}
}

func TestAccumulatorRejectsSchedstatCounterReset(t *testing.T) {
	base := time.Unix(0, 0)
	acc := NewAccumulator(time.Second)

	acc.Observe(snapshot(schedstatSample(1, 1, "worker", model.StateRunning, base, 100*time.Millisecond, 50*time.Millisecond, 10), base))
	reports := acc.Observe(snapshot(schedstatSample(1, 1, "worker", model.StateRunning, base.Add(time.Second), 50*time.Millisecond, 25*time.Millisecond, 5), base.Add(time.Second)))
	stat := onlyThread(t, reports)
	if stat.SchedstatCounterResets != 1 || stat.SchedstatAvailable() {
		t.Fatalf("schedstat reset state = resets %d available %t", stat.SchedstatCounterResets, stat.SchedstatAvailable())
	}
	if reports[0].Quality.SchedstatCounterResets != 1 {
		t.Fatalf("interval resets = %d, want 1", reports[0].Quality.SchedstatCounterResets)
	}
}

func TestAccumulatorUsesTaskstatsDelayCounterDeltas(t *testing.T) {
	base := time.Unix(0, 0)
	acc := NewAccumulator(time.Second)
	previous := delaySample(1, base, 14, true, 1, 100*time.Millisecond)
	current := delaySample(1, base.Add(time.Second), 14, true, 3, 400*time.Millisecond)

	acc.Observe(taskstatsSnapshot(previous, base))
	reports := acc.Observe(taskstatsSnapshot(current, base.Add(time.Second)))
	stat := onlyThread(t, reports)

	if !stat.DelayCountersAvailable() || stat.DelayVersion != 14 || !stat.DelayAccountingEnabled {
		t.Fatalf("delay metadata = %#v", stat)
	}
	for name, counter := range map[string]model.DelayIntervalCounter{
		"cpu":        stat.Delays.CPU,
		"block_io":   stat.Delays.BlockIO,
		"swap_in":    stat.Delays.SwapIn,
		"reclaim":    stat.Delays.Reclaim,
		"thrashing":  stat.Delays.Thrashing,
		"compaction": stat.Delays.Compaction,
		"wpcopy":     stat.Delays.WriteProtectCopy,
		"irq":        stat.Delays.IRQ,
	} {
		if !counter.Available || counter.Count != 2 || counter.Total != 300*time.Millisecond {
			t.Fatalf("%s counter = %#v, want count 2 and 300ms", name, counter)
		}
	}
	quality := reports[0].Quality
	if quality.SamplingMethod != "taskstats_counters_procfs_midpoint" || !quality.TaskstatsAvailable ||
		quality.TaskstatsThreadCount != 1 || quality.TaskstatsVersionMin != 14 || quality.TaskstatsVersionMax != 14 ||
		!quality.DelayAccountingEnabledKnown || !quality.DelayAccountingEnabled {
		t.Fatalf("taskstats quality = %#v", quality)
	}
}

func TestAccumulatorCanAlignCountersToExternalOrigin(t *testing.T) {
	base := time.Unix(0, 0)
	acc := NewAccumulatorAt(10*time.Millisecond, base)
	acc.Observe(taskstatsSnapshot(delaySample(1, base.Add(time.Millisecond), 14, true, 0, 0), base.Add(time.Millisecond)))
	reports := acc.Observe(taskstatsSnapshot(delaySample(1, base.Add(11*time.Millisecond), 14, true, 10, 100*time.Nanosecond), base.Add(11*time.Millisecond)))

	stat := onlyThread(t, reports)
	assertWindow(t, reports[0], base, base.Add(10*time.Millisecond))
	if stat.Delays.CPU.Count != 9 || stat.Delays.CPU.Total != 90*time.Nanosecond {
		t.Fatalf("aligned CPU delays = %#v", stat.Delays.CPU)
	}
}

func TestAccumulatorConservesTaskstatsDelayAcrossWindowBoundary(t *testing.T) {
	base := time.Unix(0, 0)
	acc := NewAccumulator(time.Second)
	acc.Observe(taskstatsSnapshot(delaySample(1, base, 14, true, 0, 0), base))
	acc.Observe(taskstatsSnapshot(delaySample(1, base.Add(900*time.Millisecond), 14, true, 0, 0), base.Add(900*time.Millisecond)))
	first := acc.Observe(taskstatsSnapshot(delaySample(1, base.Add(1100*time.Millisecond), 14, true, 2, 200*time.Millisecond), base.Add(1100*time.Millisecond)))
	firstStat := onlyThread(t, first)
	if firstStat.Delays.CPU.Count != 1 || firstStat.Delays.CPU.Total != 100*time.Millisecond {
		t.Fatalf("first delay allocation = %#v", firstStat.Delays.CPU)
	}

	second := acc.Observe(taskstatsSnapshot(delaySample(1, base.Add(2*time.Second), 14, true, 2, 200*time.Millisecond), base.Add(2*time.Second)))
	secondStat := onlyThread(t, second)
	if secondStat.Delays.CPU.Count != 1 || secondStat.Delays.CPU.Total != 100*time.Millisecond {
		t.Fatalf("second delay allocation = %#v", secondStat.Delays.CPU)
	}
}

func TestAccumulatorRejectsOnlyResetTaskstatsCounter(t *testing.T) {
	base := time.Unix(0, 0)
	acc := NewAccumulator(time.Second)
	previous := delaySample(1, base, 14, false, 5, 500*time.Millisecond)
	current := delaySample(1, base.Add(time.Second), 14, false, 2, 200*time.Millisecond)
	current.Delays.BlockIO = model.DelayCounter{Available: true, Count: 7, TotalNanoseconds: uint64((700 * time.Millisecond).Nanoseconds())}

	acc.Observe(taskstatsSnapshot(previous, base))
	reports := acc.Observe(taskstatsSnapshot(current, base.Add(time.Second)))
	stat := onlyThread(t, reports)
	if stat.Delays.CPU.Available || !stat.Delays.BlockIO.Available || stat.Delays.BlockIO.Count != 2 || stat.Delays.BlockIO.Total != 200*time.Millisecond {
		t.Fatalf("delay counters after reset = %#v", stat.Delays)
	}
	if stat.DelayCounterResets != 7 || reports[0].Quality.TaskstatsCounterResets != 7 {
		t.Fatalf("resets = thread %d interval %d, want 7", stat.DelayCounterResets, reports[0].Quality.TaskstatsCounterResets)
	}
}

func TestMultiplyDivideHandlesFullUint64Range(t *testing.T) {
	const maxUint64 = ^uint64(0)
	if got, want := multiplyDivide(maxUint64, 1, 2), maxUint64/2; got != want {
		t.Fatalf("multiplyDivide(max, 1, 2) = %d, want %d", got, want)
	}
}

func TestAccumulatorDoesNotMergeReusedTID(t *testing.T) {
	base := time.Unix(0, 0)
	acc := NewAccumulator(time.Second)

	acc.Observe(snapshot(sample(7, 100, "old", model.StateRunning, base), base))
	replacementAt := base.Add(time.Second)
	first := acc.Observe(model.ThreadSnapshot{
		Samples:    []model.ThreadSample{sample(7, 200, "new", model.StateSleeping, replacementAt)},
		StartedAt:  replacementAt,
		FinishedAt: replacementAt,
	})
	old := onlyThread(t, first)
	if old.StartTimeTicks != 100 {
		t.Fatalf("old start time = %d, want 100", old.StartTimeTicks)
	}
	if old.Duration(model.StateRunning) != 500*time.Millisecond || old.UnknownDuration() != 500*time.Millisecond {
		t.Fatalf("old durations = running %s unknown %s, want 500ms each",
			old.Duration(model.StateRunning), old.UnknownDuration())
	}

	second := acc.Observe(snapshot(sample(7, 200, "new", model.StateSleeping, base.Add(2*time.Second)), base.Add(2*time.Second)))
	newThread := onlyThread(t, second)
	if newThread.StartTimeTicks != 200 {
		t.Fatalf("new start time = %d, want 200", newThread.StartTimeTicks)
	}
	if newThread.Duration(model.StateSleeping) != time.Second {
		t.Fatalf("new sleeping duration = %s, want 1s", newThread.Duration(model.StateSleeping))
	}
}

func TestAccumulatorKeepsReusedTIDSeparateInsideOneWindow(t *testing.T) {
	base := time.Unix(0, 0)
	acc := NewAccumulator(2 * time.Second)

	acc.Observe(snapshot(sample(7, 100, "old", model.StateRunning, base), base))
	replacementAt := base.Add(time.Second)
	acc.Observe(model.ThreadSnapshot{
		Samples:    []model.ThreadSample{sample(7, 200, "new", model.StateSleeping, replacementAt)},
		StartedAt:  replacementAt,
		FinishedAt: replacementAt,
	})
	reports := acc.Observe(snapshot(sample(7, 200, "new", model.StateSleeping, base.Add(2*time.Second)), base.Add(2*time.Second)))

	if len(reports) != 1 {
		t.Fatalf("reports = %d, want 1", len(reports))
	}
	if len(reports[0].Threads) != 2 {
		t.Fatalf("threads = %d, want 2", len(reports[0].Threads))
	}
	if reports[0].Threads[0].StartTimeTicks != 100 || reports[0].Threads[1].StartTimeTicks != 200 {
		t.Fatalf("start times = %d,%d, want 100,200",
			reports[0].Threads[0].StartTimeTicks, reports[0].Threads[1].StartTimeTicks)
	}
}

func TestAccumulatorWaitsForEveryTrackedThreadToCrossBoundary(t *testing.T) {
	base := time.Unix(0, 0)
	acc := NewAccumulator(time.Second)
	acc.Observe(model.ThreadSnapshot{
		Samples: []model.ThreadSample{
			sample(1, 1, "one", model.StateRunning, base),
			sample(2, 2, "two", model.StateSleeping, base),
		},
		StartedAt:  base.Add(-time.Millisecond),
		FinishedAt: base,
	})

	reports := acc.Observe(model.ThreadSnapshot{
		Samples: []model.ThreadSample{
			sample(1, 1, "one", model.StateRunning, base.Add(1100*time.Millisecond)),
			sample(2, 2, "two", model.StateSleeping, base.Add(900*time.Millisecond)),
		},
		StartedAt:  base.Add(890 * time.Millisecond),
		FinishedAt: base.Add(1110 * time.Millisecond),
	})
	if len(reports) != 0 {
		t.Fatalf("reports before all threads cross boundary = %d, want 0", len(reports))
	}

	reports = acc.Observe(model.ThreadSnapshot{
		Samples: []model.ThreadSample{
			sample(1, 1, "one", model.StateRunning, base.Add(1200*time.Millisecond)),
			sample(2, 2, "two", model.StateSleeping, base.Add(1050*time.Millisecond)),
		},
		StartedAt:  base.Add(1040 * time.Millisecond),
		FinishedAt: base.Add(1210 * time.Millisecond),
	})
	if len(reports) != 1 {
		t.Fatalf("reports after all threads cross boundary = %d, want 1", len(reports))
	}
	if len(reports[0].Threads) != 2 {
		t.Fatalf("threads = %d, want 2", len(reports[0].Threads))
	}
}

func TestAccumulatorWaitsForNewThreadSecondObservation(t *testing.T) {
	base := time.Unix(0, 0)
	acc := NewAccumulator(time.Second)
	acc.Observe(snapshot(sample(1, 1, "leader", model.StateRunning, base), base))

	reports := acc.Observe(model.ThreadSnapshot{
		Samples: []model.ThreadSample{
			sample(1, 1, "leader", model.StateRunning, base.Add(1100*time.Millisecond)),
			sample(2, 2, "new", model.StateSleeping, base.Add(900*time.Millisecond)),
		},
		StartedAt:  base.Add(890 * time.Millisecond),
		FinishedAt: base.Add(1110 * time.Millisecond),
	})
	if len(reports) != 0 {
		t.Fatalf("reports before new thread has a closing sample = %d, want 0", len(reports))
	}

	reports = acc.Observe(model.ThreadSnapshot{
		Samples: []model.ThreadSample{
			sample(1, 1, "leader", model.StateRunning, base.Add(1200*time.Millisecond)),
			sample(2, 2, "new", model.StateSleeping, base.Add(1050*time.Millisecond)),
		},
		StartedAt:  base.Add(1040 * time.Millisecond),
		FinishedAt: base.Add(1210 * time.Millisecond),
	})
	if len(reports) != 1 {
		t.Fatalf("reports after new thread crosses boundary = %d, want 1", len(reports))
	}
	if len(reports[0].Threads) != 2 {
		t.Fatalf("threads = %d, want 2", len(reports[0].Threads))
	}
	newThread := reports[0].Threads[1]
	if newThread.StartTimeTicks != 2 || newThread.Duration(model.StateSleeping) != 100*time.Millisecond {
		t.Fatalf("new thread = %#v, want 100ms sleeping tracked from first observation", newThread)
	}
}

func TestAccumulatorMarksDisappearanceAsUnknown(t *testing.T) {
	base := time.Unix(0, 0)
	acc := NewAccumulator(time.Second)

	acc.Observe(snapshot(sample(1, 1, "worker", model.StateSleeping, base), base))
	reports := acc.Observe(model.ThreadSnapshot{
		StartedAt:  base.Add(time.Second),
		FinishedAt: base.Add(time.Second),
	})
	stat := onlyThread(t, reports)
	if stat.Duration(model.StateSleeping) != 500*time.Millisecond {
		t.Fatalf("sleeping duration = %s, want 500ms", stat.Duration(model.StateSleeping))
	}
	if stat.UnknownDuration() != 500*time.Millisecond {
		t.Fatalf("unknown duration = %s, want 500ms", stat.UnknownDuration())
	}
}

func TestAccumulatorTracksScanQuality(t *testing.T) {
	base := time.Unix(0, 0)
	acc := NewAccumulator(time.Second)
	first := model.ThreadSnapshot{
		Samples:    []model.ThreadSample{sample(1, 1, "worker", model.StateRunning, base)},
		StartedAt:  base.Add(-2 * time.Millisecond),
		FinishedAt: base,
	}
	second := model.ThreadSnapshot{
		Samples:    []model.ThreadSample{sample(1, 1, "worker", model.StateRunning, base.Add(time.Second))},
		StartedAt:  base.Add(time.Second - 3*time.Millisecond),
		FinishedAt: base.Add(time.Second),
	}

	acc.Observe(first)
	reports := acc.Observe(second)
	if reports[0].Quality.SnapshotCount != 2 {
		t.Fatalf("snapshot count = %d, want 2", reports[0].Quality.SnapshotCount)
	}
	if reports[0].Quality.MaxScanDuration != 3*time.Millisecond {
		t.Fatalf("max scan duration = %s, want 3ms", reports[0].Quality.MaxScanDuration)
	}
}

func onlyThread(t *testing.T, reports []model.IntervalReport) model.ThreadIntervalStats {
	t.Helper()
	if len(reports) != 1 {
		t.Fatalf("reports = %d, want 1", len(reports))
	}
	if len(reports[0].Threads) != 1 {
		t.Fatalf("threads = %d, want 1", len(reports[0].Threads))
	}
	return reports[0].Threads[0]
}

func assertWindow(t *testing.T, report model.IntervalReport, start, end time.Time) {
	t.Helper()
	if !report.IntervalStart.Equal(start) || !report.IntervalEnd.Equal(end) {
		t.Fatalf("window = [%s,%s), want [%s,%s)", report.IntervalStart, report.IntervalEnd, start, end)
	}
}

func snapshot(sample model.ThreadSample, at time.Time) model.ThreadSnapshot {
	return model.ThreadSnapshot{
		Samples:    []model.ThreadSample{sample},
		StartedAt:  at.Add(-time.Nanosecond),
		FinishedAt: at,
	}
}

func sample(tid int, startTime uint64, comm string, state model.ThreadState, ts time.Time) model.ThreadSample {
	return model.ThreadSample{
		PID:            99,
		TID:            tid,
		StartTimeTicks: startTime,
		Comm:           comm,
		State:          state,
		Timestamp:      ts,
	}
}

func schedstatSample(
	tid int,
	startTime uint64,
	comm string,
	state model.ThreadState,
	ts time.Time,
	onCPU time.Duration,
	runqueue time.Duration,
	timeslices uint64,
) model.ThreadSample {
	sample := sample(tid, startTime, comm, state, ts)
	sample.Schedstat = model.SchedstatCounters{
		Available:           true,
		OnCPUNanoseconds:    uint64(onCPU.Nanoseconds()),
		RunqueueNanoseconds: uint64(runqueue.Nanoseconds()),
		Timeslices:          timeslices,
	}
	return sample
}

func taskstatsSnapshot(sample model.ThreadSample, at time.Time) model.ThreadSnapshot {
	snapshot := snapshot(sample, at)
	snapshot.SamplingMethod = "taskstats_counters_procfs_midpoint"
	return snapshot
}

func delaySample(tid int, at time.Time, version uint16, enabled bool, count uint64, total time.Duration) model.ThreadSample {
	sample := sample(tid, 1, "worker", model.StateRunning, at)
	counter := model.DelayCounter{
		Available:        true,
		Count:            count,
		TotalNanoseconds: uint64(total.Nanoseconds()),
	}
	sample.Delays = model.DelayCounters{
		Available:              true,
		Version:                version,
		AccountingEnabled:      enabled,
		AccountingEnabledKnown: true,
		CPU:                    counter,
		BlockIO:                counter,
		SwapIn:                 counter,
		Reclaim:                counter,
		Thrashing:              counter,
		Compaction:             counter,
		WriteProtectCopy:       counter,
		IRQ:                    counter,
	}
	return sample
}
