package collector

import (
	"math/bits"
	"sort"
	"time"

	"github.com/BogdanDolia/tsastat/internal/model"
)

const procSamplingMethod = "procfs_midpoint"

type intervalWindow struct {
	stats   map[model.ThreadIdentity]*model.ThreadIntervalStats
	quality model.IntervalQuality
}

type Accumulator struct {
	interval    time.Duration
	origin      time.Time
	fixedOrigin bool
	initialized bool
	previous    map[model.ThreadIdentity]model.ThreadSample
	windows     map[int64]*intervalWindow
	nextWindow  int64
}

type schedstatDelta struct {
	onCPUNanoseconds    uint64
	runqueueNanoseconds uint64
	timeslices          uint64
}

type delayCounterDelta struct {
	available        bool
	count            uint64
	totalNanoseconds uint64
}

type delayDelta struct {
	version                uint16
	accountingEnabled      bool
	accountingEnabledKnown bool
	resets                 int
	cpu                    delayCounterDelta
	blockIO                delayCounterDelta
	swapIn                 delayCounterDelta
	reclaim                delayCounterDelta
	thrashing              delayCounterDelta
	compaction             delayCounterDelta
	writeProtectCopy       delayCounterDelta
	irq                    delayCounterDelta
}

func NewAccumulator(interval time.Duration) *Accumulator {
	return &Accumulator{
		interval: interval,
		previous: make(map[model.ThreadIdentity]model.ThreadSample),
		windows:  make(map[int64]*intervalWindow),
	}
}

// NewAccumulatorAt aligns snapshot-derived counter windows with an external
// event timeline. The first snapshot still establishes the counter baseline,
// so time before that snapshot remains unobserved for cumulative counters.
func NewAccumulatorAt(interval time.Duration, origin time.Time) *Accumulator {
	acc := NewAccumulator(interval)
	acc.origin = origin
	acc.fixedOrigin = true
	return acc
}

func (a *Accumulator) Observe(snapshot model.ThreadSnapshot) []model.IntervalReport {
	snapshot = normalizeSnapshot(snapshot)
	current := samplesByIdentity(snapshot.Samples)

	if !a.initialized {
		if !a.fixedOrigin {
			a.origin = snapshot.FinishedAt
		}
		a.initialized = true
		a.previous = current
		a.recordScan(snapshot)
		for _, sample := range snapshot.Samples {
			a.recordObservationAt(sample, 0)
		}
		return nil
	}

	a.recordScan(snapshot)
	for _, sample := range snapshot.Samples {
		a.recordObservation(sample)
	}

	watermark := snapshot.FinishedAt
	for identity, previous := range a.previous {
		if sample, exists := current[identity]; exists {
			end := sample.Timestamp
			if end.IsZero() {
				end = snapshot.FinishedAt
			}
			a.observeGap(previous, sample, end)
			watermark = earlier(watermark, end)
			continue
		}

		end := snapshot.StartedAt
		if end.IsZero() {
			end = snapshot.FinishedAt
		}
		a.observeDisappearance(previous, end)
		watermark = earlier(watermark, end)
	}
	for identity, sample := range current {
		if _, existed := a.previous[identity]; existed {
			continue
		}
		end := sample.Timestamp
		if end.IsZero() {
			end = snapshot.FinishedAt
		}
		watermark = earlier(watermark, end)
	}

	a.previous = current
	return a.finalizeThrough(watermark)
}

func (a *Accumulator) observeGap(previous, current model.ThreadSample, end time.Time) {
	start := previous.Timestamp
	if !end.After(start) {
		return
	}

	a.observeSchedstat(previous, current, start, end)
	a.observeDelays(previous, current, start, end)
	a.recordGap(previous, start, end)
	if previous.State == current.State {
		a.addSegment(previous, previous.State, start, end)
		return
	}

	transitionAt := midpointTime(start, end)
	a.addSegment(previous, previous.State, start, transitionAt)
	a.addSegment(current, current.State, transitionAt, end)
	a.recordTransition(current, start, transitionAt, end)
}

func (a *Accumulator) observeDelays(previous, current model.ThreadSample, start, end time.Time) {
	if !previous.Delays.Available || !current.Delays.Available {
		return
	}
	delta := delayDelta{
		version:                current.Delays.Version,
		accountingEnabled:      current.Delays.AccountingEnabled,
		accountingEnabledKnown: current.Delays.AccountingEnabledKnown,
	}
	delta.cpu, delta.resets = calculateDelayCounterDelta(previous.Delays.CPU, current.Delays.CPU, delta.resets)
	delta.blockIO, delta.resets = calculateDelayCounterDelta(previous.Delays.BlockIO, current.Delays.BlockIO, delta.resets)
	delta.swapIn, delta.resets = calculateDelayCounterDelta(previous.Delays.SwapIn, current.Delays.SwapIn, delta.resets)
	delta.reclaim, delta.resets = calculateDelayCounterDelta(previous.Delays.Reclaim, current.Delays.Reclaim, delta.resets)
	delta.thrashing, delta.resets = calculateDelayCounterDelta(previous.Delays.Thrashing, current.Delays.Thrashing, delta.resets)
	delta.compaction, delta.resets = calculateDelayCounterDelta(previous.Delays.Compaction, current.Delays.Compaction, delta.resets)
	delta.writeProtectCopy, delta.resets = calculateDelayCounterDelta(previous.Delays.WriteProtectCopy, current.Delays.WriteProtectCopy, delta.resets)
	delta.irq, delta.resets = calculateDelayCounterDelta(previous.Delays.IRQ, current.Delays.IRQ, delta.resets)
	if !delta.hasComparableCounter() && delta.resets == 0 {
		return
	}
	a.addDelayDelta(current, start, end, delta)
}

func calculateDelayCounterDelta(previous, current model.DelayCounter, resets int) (delayCounterDelta, int) {
	if !previous.Available || !current.Available {
		return delayCounterDelta{}, resets
	}
	if current.Count < previous.Count || current.TotalNanoseconds < previous.TotalNanoseconds {
		return delayCounterDelta{}, resets + 1
	}
	return delayCounterDelta{
		available:        true,
		count:            current.Count - previous.Count,
		totalNanoseconds: current.TotalNanoseconds - previous.TotalNanoseconds,
	}, resets
}

func (d delayDelta) hasComparableCounter() bool {
	return d.cpu.available || d.blockIO.available || d.swapIn.available || d.reclaim.available ||
		d.thrashing.available || d.compaction.available || d.writeProtectCopy.available || d.irq.available
}

func (a *Accumulator) addDelayDelta(sample model.ThreadSample, start, end time.Time, delta delayDelta) {
	total := end.Sub(start)
	if total <= 0 {
		return
	}
	totalNanoseconds := uint64(total)
	gap := total

	a.forEachWindow(start, end, func(index int64, overlapStart, overlapEnd time.Time) {
		startOffset := uint64(overlapStart.Sub(start))
		endOffset := uint64(overlapEnd.Sub(start))
		stat := a.threadStats(index, sample)
		stat.DelayVersion = delta.version
		stat.DelayAccountingEnabled = delta.accountingEnabled
		stat.DelayAccountingEnabledKnown = delta.accountingEnabledKnown
		stat.DelayCounterResets += delta.resets
		if delta.hasComparableCounter() {
			stat.DelayObserved += overlapEnd.Sub(overlapStart)
			stat.DelaySamplePairs++
			if gap > stat.DelayMaxSampleGap {
				stat.DelayMaxSampleGap = gap
			}
		}

		addDelayCounterDelta(&stat.Delays.CPU, delta.cpu, startOffset, endOffset, totalNanoseconds)
		addDelayCounterDelta(&stat.Delays.BlockIO, delta.blockIO, startOffset, endOffset, totalNanoseconds)
		addDelayCounterDelta(&stat.Delays.SwapIn, delta.swapIn, startOffset, endOffset, totalNanoseconds)
		addDelayCounterDelta(&stat.Delays.Reclaim, delta.reclaim, startOffset, endOffset, totalNanoseconds)
		addDelayCounterDelta(&stat.Delays.Thrashing, delta.thrashing, startOffset, endOffset, totalNanoseconds)
		addDelayCounterDelta(&stat.Delays.Compaction, delta.compaction, startOffset, endOffset, totalNanoseconds)
		addDelayCounterDelta(&stat.Delays.WriteProtectCopy, delta.writeProtectCopy, startOffset, endOffset, totalNanoseconds)
		addDelayCounterDelta(&stat.Delays.IRQ, delta.irq, startOffset, endOffset, totalNanoseconds)
	})
}

func addDelayCounterDelta(target *model.DelayIntervalCounter, delta delayCounterDelta, startOffset, endOffset, totalNanoseconds uint64) {
	if !delta.available {
		return
	}
	target.Available = true
	countStart := multiplyDivide(delta.count, startOffset, totalNanoseconds)
	countEnd := multiplyDivide(delta.count, endOffset, totalNanoseconds)
	totalStart := multiplyDivide(delta.totalNanoseconds, startOffset, totalNanoseconds)
	totalEnd := multiplyDivide(delta.totalNanoseconds, endOffset, totalNanoseconds)
	target.Count += countEnd - countStart
	target.Total += nanosecondsDuration(totalEnd - totalStart)
}

func (a *Accumulator) observeSchedstat(previous, current model.ThreadSample, start, end time.Time) {
	if !previous.Schedstat.Available || !current.Schedstat.Available {
		return
	}
	if current.Schedstat.OnCPUNanoseconds < previous.Schedstat.OnCPUNanoseconds ||
		current.Schedstat.RunqueueNanoseconds < previous.Schedstat.RunqueueNanoseconds ||
		current.Schedstat.Timeslices < previous.Schedstat.Timeslices {
		a.recordSchedstatReset(current, start, end)
		return
	}

	delta := schedstatDelta{
		onCPUNanoseconds:    current.Schedstat.OnCPUNanoseconds - previous.Schedstat.OnCPUNanoseconds,
		runqueueNanoseconds: current.Schedstat.RunqueueNanoseconds - previous.Schedstat.RunqueueNanoseconds,
		timeslices:          current.Schedstat.Timeslices - previous.Schedstat.Timeslices,
	}
	a.addSchedstatDelta(current, start, end, delta)
}

func (a *Accumulator) addSchedstatDelta(sample model.ThreadSample, start, end time.Time, delta schedstatDelta) {
	total := end.Sub(start)
	if total <= 0 {
		return
	}
	totalNanoseconds := uint64(total)
	gap := total

	a.forEachWindow(start, end, func(index int64, overlapStart, overlapEnd time.Time) {
		startOffset := uint64(overlapStart.Sub(start))
		endOffset := uint64(overlapEnd.Sub(start))
		onCPUStart := multiplyDivide(delta.onCPUNanoseconds, startOffset, totalNanoseconds)
		onCPUEnd := multiplyDivide(delta.onCPUNanoseconds, endOffset, totalNanoseconds)
		runqueueStart := multiplyDivide(delta.runqueueNanoseconds, startOffset, totalNanoseconds)
		runqueueEnd := multiplyDivide(delta.runqueueNanoseconds, endOffset, totalNanoseconds)
		timeslicesStart := multiplyDivide(delta.timeslices, startOffset, totalNanoseconds)
		timeslicesEnd := multiplyDivide(delta.timeslices, endOffset, totalNanoseconds)

		stat := a.threadStats(index, sample)
		stat.OnCPU += nanosecondsDuration(onCPUEnd - onCPUStart)
		stat.RunqueueWait += nanosecondsDuration(runqueueEnd - runqueueStart)
		stat.Timeslices += timeslicesEnd - timeslicesStart
		observed := overlapEnd.Sub(overlapStart)
		stat.SchedstatObserved += observed
		stat.SchedulerObserved += observed
		stat.SchedulerSource = "proc_schedstat"
		stat.SchedstatSamplePairs++
		if gap > stat.SchedstatMaxSampleGap {
			stat.SchedstatMaxSampleGap = gap
		}
	})
}

func (a *Accumulator) recordSchedstatReset(sample model.ThreadSample, start, end time.Time) {
	a.forEachWindow(start, end, func(index int64, _, _ time.Time) {
		a.threadStats(index, sample).SchedstatCounterResets++
	})
}

func (a *Accumulator) observeDisappearance(previous model.ThreadSample, end time.Time) {
	start := previous.Timestamp
	if !end.After(start) {
		return
	}

	a.recordGap(previous, start, end)
	transitionAt := midpointTime(start, end)
	a.addSegment(previous, previous.State, start, transitionAt)
	a.addSegment(previous, model.StateUnknown, transitionAt, end)
	a.recordTransition(previous, start, transitionAt, end)
}

func (a *Accumulator) addSegment(sample model.ThreadSample, state model.ThreadState, start, end time.Time) {
	a.forEachWindow(start, end, func(index int64, overlapStart, overlapEnd time.Time) {
		stat := a.threadStats(index, sample)
		delta := overlapEnd.Sub(overlapStart)
		stat.Durations[state] += delta
		stat.TotalObserved += delta
	})
}

func (a *Accumulator) recordGap(sample model.ThreadSample, start, end time.Time) {
	gap := end.Sub(start)
	a.forEachWindow(start, end, func(index int64, _, _ time.Time) {
		stat := a.threadStats(index, sample)
		if gap > stat.MaxSampleGap {
			stat.MaxSampleGap = gap
		}
		window := a.window(index)
		if gap > window.quality.MaxSampleGap {
			window.quality.MaxSampleGap = gap
		}
	})
}

func (a *Accumulator) recordTransition(sample model.ThreadSample, start, midpoint, end time.Time) {
	a.forEachWindow(start, end, func(index int64, overlapStart, overlapEnd time.Time) {
		before := overlapDuration(overlapStart, overlapEnd, start, midpoint)
		after := overlapDuration(overlapStart, overlapEnd, midpoint, end)
		uncertainty := before
		if after > uncertainty {
			uncertainty = after
		}

		stat := a.threadStats(index, sample)
		stat.DetectedTransitions++
		stat.DetectedTransitionUncertainty += uncertainty
	})
}

func (a *Accumulator) recordObservation(sample model.ThreadSample) {
	index := int64(0)
	if sample.Timestamp.After(a.origin) || sample.Timestamp.Equal(a.origin) {
		index = a.windowIndex(sample.Timestamp)
	}
	a.recordObservationAt(sample, index)
}

func (a *Accumulator) recordObservationAt(sample model.ThreadSample, index int64) {
	if index < a.nextWindow {
		return
	}
	a.threadStats(index, sample).SampleCount++
}

func (a *Accumulator) recordScan(snapshot model.ThreadSnapshot) {
	duration := snapshot.FinishedAt.Sub(snapshot.StartedAt)
	if duration < 0 {
		duration = 0
	}

	indices := a.scanWindowIndices(snapshot.StartedAt, snapshot.FinishedAt)
	for _, index := range indices {
		if index < a.nextWindow {
			continue
		}
		window := a.window(index)
		if snapshot.SamplingMethod != "" {
			window.quality.SamplingMethod = snapshot.SamplingMethod
		}
		window.quality.SnapshotCount++
		if duration > window.quality.MaxScanDuration {
			window.quality.MaxScanDuration = duration
		}
	}
}

func (a *Accumulator) scanWindowIndices(start, end time.Time) []int64 {
	if !end.After(a.origin) {
		return []int64{0}
	}
	if !end.After(start) {
		return []int64{a.windowIndex(end)}
	}

	indices := make([]int64, 0, 2)
	a.forEachWindow(start, end, func(index int64, _, _ time.Time) {
		indices = append(indices, index)
	})
	if len(indices) == 0 {
		indices = append(indices, a.windowIndex(end))
	}
	return indices
}

func (a *Accumulator) forEachWindow(start, end time.Time, fn func(int64, time.Time, time.Time)) {
	if !end.After(start) || !end.After(a.origin) {
		return
	}
	if start.Before(a.origin) {
		start = a.origin
	}

	for start.Before(end) {
		index := a.windowIndex(start)
		_, windowEnd := a.windowBounds(index)
		overlapEnd := end
		if windowEnd.Before(overlapEnd) {
			overlapEnd = windowEnd
		}
		if index >= a.nextWindow {
			fn(index, start, overlapEnd)
		}
		start = overlapEnd
	}
}

func (a *Accumulator) finalizeThrough(watermark time.Time) []model.IntervalReport {
	if !a.initialized || watermark.Before(a.origin) {
		return nil
	}

	reports := make([]model.IntervalReport, 0, 1)
	for {
		start, end := a.windowBounds(a.nextWindow)
		if end.After(watermark) {
			break
		}

		window := a.windows[a.nextWindow]
		quality := model.IntervalQuality{
			SamplingMethod:            procSamplingMethod,
			MissedTransitionsPossible: true,
		}
		var stats []model.ThreadIntervalStats
		if window != nil {
			quality = window.quality
			if quality.SamplingMethod == "" {
				quality.SamplingMethod = procSamplingMethod
			}
			quality.MissedTransitionsPossible = true
			stats = sortedStats(window.stats)
			for _, stat := range stats {
				if stat.SchedstatAvailable() {
					quality.SchedstatAvailable = true
					quality.SchedstatThreadCount++
				}
				quality.SchedstatCounterResets += stat.SchedstatCounterResets
				quality.TaskstatsCounterResets += stat.DelayCounterResets
				if stat.DelayCountersAvailable() {
					quality.TaskstatsAvailable = true
					quality.TaskstatsThreadCount++
					if quality.TaskstatsVersionMin == 0 || stat.DelayVersion < quality.TaskstatsVersionMin {
						quality.TaskstatsVersionMin = stat.DelayVersion
					}
					if stat.DelayVersion > quality.TaskstatsVersionMax {
						quality.TaskstatsVersionMax = stat.DelayVersion
					}
				}
				if stat.DelayAccountingEnabledKnown {
					if !quality.DelayAccountingEnabledKnown {
						quality.DelayAccountingEnabled = stat.DelayAccountingEnabled
					} else {
						quality.DelayAccountingEnabled = quality.DelayAccountingEnabled && stat.DelayAccountingEnabled
					}
					quality.DelayAccountingEnabledKnown = true
				}
			}
		}

		reports = append(reports, model.IntervalReport{
			IntervalStart: start,
			IntervalEnd:   end,
			Threads:       stats,
			Quality:       quality,
		})
		delete(a.windows, a.nextWindow)
		a.nextWindow++
	}
	return reports
}

func (a *Accumulator) threadStats(index int64, sample model.ThreadSample) *model.ThreadIntervalStats {
	window := a.window(index)
	identity := sample.Identity()
	stat, exists := window.stats[identity]
	if !exists {
		start, end := a.windowBounds(index)
		stat = &model.ThreadIntervalStats{
			PID:                  sample.PID,
			TID:                  sample.TID,
			StartTimeTicks:       sample.StartTimeTicks,
			StartTimeNanoseconds: sample.StartTimeNanoseconds,
			Comm:                 sample.Comm,
			IntervalStart:        start,
			IntervalEnd:          end,
			Durations:            make(map[model.ThreadState]time.Duration),
		}
		window.stats[identity] = stat
	}
	if sample.Comm != "" {
		stat.Comm = sample.Comm
	}
	return stat
}

func (a *Accumulator) window(index int64) *intervalWindow {
	window, exists := a.windows[index]
	if !exists {
		window = &intervalWindow{
			stats: make(map[model.ThreadIdentity]*model.ThreadIntervalStats),
			quality: model.IntervalQuality{
				SamplingMethod:            procSamplingMethod,
				MissedTransitionsPossible: true,
			},
		}
		a.windows[index] = window
	}
	return window
}

func (a *Accumulator) windowIndex(at time.Time) int64 {
	if !at.After(a.origin) {
		return 0
	}
	return int64(at.Sub(a.origin) / a.interval)
}

func (a *Accumulator) windowBounds(index int64) (time.Time, time.Time) {
	start := a.origin.Add(time.Duration(index) * a.interval)
	return start, start.Add(a.interval)
}

func samplesByIdentity(samples []model.ThreadSample) map[model.ThreadIdentity]model.ThreadSample {
	current := make(map[model.ThreadIdentity]model.ThreadSample, len(samples))
	for _, sample := range samples {
		current[sample.Identity()] = sample
	}
	return current
}

func sortedStats(statsByIdentity map[model.ThreadIdentity]*model.ThreadIntervalStats) []model.ThreadIntervalStats {
	identities := make([]model.ThreadIdentity, 0, len(statsByIdentity))
	for identity := range statsByIdentity {
		identities = append(identities, identity)
	}
	sort.Slice(identities, func(i, j int) bool {
		if identities[i].TID == identities[j].TID {
			if identities[i].StartTimeTicks == identities[j].StartTimeTicks {
				return identities[i].StartTimeNanoseconds < identities[j].StartTimeNanoseconds
			}
			return identities[i].StartTimeTicks < identities[j].StartTimeTicks
		}
		return identities[i].TID < identities[j].TID
	})

	stats := make([]model.ThreadIntervalStats, 0, len(identities))
	for _, identity := range identities {
		stats = append(stats, *statsByIdentity[identity])
	}
	return stats
}

func normalizeSnapshot(snapshot model.ThreadSnapshot) model.ThreadSnapshot {
	if snapshot.StartedAt.IsZero() {
		snapshot.StartedAt = firstSampleTime(snapshot.Samples)
	}
	if snapshot.FinishedAt.IsZero() {
		snapshot.FinishedAt = lastSampleTime(snapshot.Samples)
	}
	if snapshot.FinishedAt.IsZero() {
		snapshot.FinishedAt = time.Now()
	}
	if snapshot.StartedAt.IsZero() || snapshot.FinishedAt.Before(snapshot.StartedAt) {
		snapshot.StartedAt = snapshot.FinishedAt
	}
	return snapshot
}

func firstSampleTime(samples []model.ThreadSample) time.Time {
	var first time.Time
	for _, sample := range samples {
		if sample.Timestamp.IsZero() {
			continue
		}
		if first.IsZero() || sample.Timestamp.Before(first) {
			first = sample.Timestamp
		}
	}
	return first
}

func lastSampleTime(samples []model.ThreadSample) time.Time {
	var last time.Time
	for _, sample := range samples {
		if sample.Timestamp.After(last) {
			last = sample.Timestamp
		}
	}
	return last
}

func earlier(left, right time.Time) time.Time {
	if left.IsZero() || (!right.IsZero() && right.Before(left)) {
		return right
	}
	return left
}

func midpointTime(start, end time.Time) time.Time {
	return start.Add(end.Sub(start) / 2)
}

func overlapDuration(leftStart, leftEnd, rightStart, rightEnd time.Time) time.Duration {
	start := leftStart
	if rightStart.After(start) {
		start = rightStart
	}
	end := leftEnd
	if rightEnd.Before(end) {
		end = rightEnd
	}
	if !end.After(start) {
		return 0
	}
	return end.Sub(start)
}

func multiplyDivide(value, numerator, denominator uint64) uint64 {
	if value == 0 || numerator == 0 || denominator == 0 {
		return 0
	}
	high, low := bits.Mul64(value, numerator)
	quotient, _ := bits.Div64(high, low, denominator)
	return quotient
}

func nanosecondsDuration(value uint64) time.Duration {
	const maxDurationNanoseconds = uint64(1<<63 - 1)
	if value > maxDurationNanoseconds {
		return time.Duration(maxDurationNanoseconds)
	}
	return time.Duration(value)
}
