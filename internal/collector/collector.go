package collector

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/BogdanDolia/tsastat/internal/backend"
	"github.com/BogdanDolia/tsastat/internal/model"
)

type Collector struct {
	backend        backend.Backend
	pid            int
	interval       time.Duration
	sampleInterval time.Duration
	acc            *Accumulator
}

func New(b backend.Backend, pid int, interval, sampleInterval time.Duration) *Collector {
	return &Collector{
		backend:        b,
		pid:            pid,
		interval:       interval,
		sampleInterval: sampleInterval,
		acc:            NewAccumulator(interval),
	}
}

func (c *Collector) Run(ctx context.Context, count int, emit func(model.IntervalReport) error) error {
	if hybridBackend, ok := c.backend.(backend.HybridBackend); ok {
		return c.runHybrid(ctx, hybridBackend, count, emit)
	}
	if eventBackend, ok := c.backend.(backend.SchedulerEventBackend); ok {
		return c.runSchedulerEvents(ctx, eventBackend, count, emit)
	}
	snapshotBackend, ok := c.backend.(backend.SnapshotBackend)
	if !ok {
		return fmt.Errorf("backend %q provides neither snapshots nor scheduler events", c.backend.Name())
	}
	return c.runSnapshots(ctx, snapshotBackend, count, emit)
}

func (c *Collector) runSnapshots(ctx context.Context, snapshotBackend backend.SnapshotBackend, count int, emit func(model.IntervalReport) error) error {
	snapshot, err := snapshotBackend.Snapshot(ctx, c.pid)
	if err != nil {
		return err
	}
	c.acc.Observe(snapshot)
	ticker := time.NewTicker(c.sampleInterval)
	defer ticker.Stop()

	emitted := 0
	for count <= 0 || emitted < count {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}

		snapshot, err := snapshotBackend.Snapshot(ctx, c.pid)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return nil
			}
			return err
		}

		for _, report := range c.acc.Observe(snapshot) {
			if err := emit(c.withSourceStatus(report)); err != nil {
				return err
			}
			emitted++
			if count > 0 && emitted >= count {
				return nil
			}
		}
	}

	return nil
}

func (c *Collector) runSchedulerEvents(ctx context.Context, eventBackend backend.SchedulerEventBackend, count int, emit func(model.IntervalReport) error) error {
	stream, err := eventBackend.OpenSchedulerEvents(ctx, c.pid)
	if err != nil {
		return err
	}
	defer stream.Close()

	acc := NewEventAccumulator(c.interval, stream.InitialSnapshot())
	acc.SetClockCalibrationUncertainty(stream.ClockCalibrationUncertainty())
	timer := time.NewTimer(durationUntil(acc.NextBoundary()))
	defer timer.Stop()

	emitted := 0
	for count <= 0 || emitted < count {
		select {
		case <-ctx.Done():
			return nil
		case event, ok := <-stream.Events():
			if !ok {
				return fmt.Errorf("ebpf scheduler event stream closed unexpectedly")
			}
			acc.Observe(event)
		case streamErr, ok := <-stream.Errors():
			if !ok {
				return fmt.Errorf("ebpf scheduler error stream closed unexpectedly")
			}
			if streamErr != nil {
				return streamErr
			}
		case <-timer.C:
			boundary := acc.NextBoundary()
			if err := flushSchedulerEvents(ctx, stream, acc); err != nil {
				if errors.Is(err, context.Canceled) {
					return nil
				}
				return err
			}
			for _, report := range acc.Advance(boundary, stream.LostEvents()) {
				if err := emit(c.withSourceStatus(report)); err != nil {
					return err
				}
				emitted++
				if count > 0 && emitted >= count {
					return nil
				}
			}
			timer.Reset(durationUntil(acc.NextBoundary()))
		}
	}
	return nil
}

func (c *Collector) runHybrid(ctx context.Context, hybridBackend backend.HybridBackend, count int, emit func(model.IntervalReport) error) error {
	stream, err := hybridBackend.OpenSchedulerEvents(ctx, c.pid)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil
		}
		// Event tracing is an optional source for a hybrid backend. Snapshot()
		// performs its own taskstats-to-proc fallback.
		return c.runSnapshots(ctx, hybridBackend, count, emit)
	}
	defer stream.Close()
	return c.runHybridSchedulerEvents(ctx, hybridBackend, stream, count, emit)
}

func (c *Collector) runHybridSchedulerEvents(
	ctx context.Context,
	snapshotBackend backend.SnapshotBackend,
	stream model.SchedulerEventStream,
	count int,
	emit func(model.IntervalReport) error,
) error {
	eventAcc := NewEventAccumulator(c.interval, stream.InitialSnapshot())
	eventAcc.SetClockCalibrationUncertainty(stream.ClockCalibrationUncertainty())

	auxSnapshot, err := snapshotBackend.Snapshot(ctx, c.pid)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil
		}
		return err
	}
	auxEnabled := snapshotHasDelays(auxSnapshot)
	var auxAcc *Accumulator
	auxReports := make(map[int64]model.IntervalReport)
	var sampleTicker *time.Ticker
	var sampleTick <-chan time.Time
	if auxEnabled {
		auxAcc = NewAccumulatorAt(c.interval, eventAcc.Origin())
		storeAuxReports(auxReports, auxAcc.Observe(auxSnapshot))
		sampleTicker = time.NewTicker(c.sampleInterval)
		sampleTick = sampleTicker.C
		defer sampleTicker.Stop()
	}

	timer := time.NewTimer(durationUntil(eventAcc.NextBoundary()))
	defer timer.Stop()
	emitted := 0
	for count <= 0 || emitted < count {
		select {
		case <-ctx.Done():
			return nil
		case event, ok := <-stream.Events():
			if !ok {
				return fmt.Errorf("ebpf scheduler event stream closed unexpectedly")
			}
			eventAcc.Observe(event)
		case streamErr, ok := <-stream.Errors():
			if !ok {
				return fmt.Errorf("ebpf scheduler error stream closed unexpectedly")
			}
			if streamErr != nil {
				return streamErr
			}
		case <-sampleTick:
			if err := c.observeAuxSnapshot(ctx, snapshotBackend, auxAcc, auxReports); err != nil {
				return err
			}
		case <-timer.C:
			boundary := eventAcc.NextBoundary()
			if err := flushSchedulerEvents(ctx, stream, eventAcc); err != nil {
				if errors.Is(err, context.Canceled) {
					return nil
				}
				return err
			}
			if auxEnabled {
				if err := c.observeAuxSnapshot(ctx, snapshotBackend, auxAcc, auxReports); err != nil {
					return err
				}
			}
			for _, report := range eventAcc.Advance(boundary, stream.LostEvents()) {
				if auxiliary, ok := auxReports[report.IntervalStart.UnixNano()]; ok {
					report = mergeHybridReport(report, auxiliary)
					delete(auxReports, report.IntervalStart.UnixNano())
				}
				if err := emit(c.withSourceStatus(report)); err != nil {
					return err
				}
				emitted++
				if count > 0 && emitted >= count {
					return nil
				}
			}
			timer.Reset(durationUntil(eventAcc.NextBoundary()))
		}
	}
	return nil
}

func (c *Collector) observeAuxSnapshot(
	ctx context.Context,
	snapshotBackend backend.SnapshotBackend,
	acc *Accumulator,
	reports map[int64]model.IntervalReport,
) error {
	snapshot, err := snapshotBackend.Snapshot(ctx, c.pid)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return context.Canceled
		}
		return err
	}
	storeAuxReports(reports, acc.Observe(snapshot))
	return nil
}

func snapshotHasDelays(snapshot model.ThreadSnapshot) bool {
	for _, sample := range snapshot.Samples {
		if sample.Delays.Available {
			return true
		}
	}
	return false
}

func storeAuxReports(target map[int64]model.IntervalReport, reports []model.IntervalReport) {
	for _, report := range reports {
		target[report.IntervalStart.UnixNano()] = report
	}
}

func mergeHybridReport(eventReport, auxiliary model.IntervalReport) model.IntervalReport {
	if !eventReport.IntervalStart.Equal(auxiliary.IntervalStart) || !eventReport.IntervalEnd.Equal(auxiliary.IntervalEnd) {
		return eventReport
	}

	used := make([]bool, len(auxiliary.Threads))
	for eventIndex := range eventReport.Threads {
		match := -1
		ambiguous := false
		for auxiliaryIndex := range auxiliary.Threads {
			if used[auxiliaryIndex] || !sameProvenThread(eventReport.Threads[eventIndex], auxiliary.Threads[auxiliaryIndex]) {
				continue
			}
			if match >= 0 {
				ambiguous = true
				break
			}
			match = auxiliaryIndex
		}
		if match < 0 || ambiguous {
			continue
		}
		copyDelayStats(&eventReport.Threads[eventIndex], auxiliary.Threads[match])
		used[match] = true
	}

	for index, stat := range auxiliary.Threads {
		if !used[index] && stat.DelayCountersAvailable() {
			eventReport.Quality.HybridIdentityMismatches++
		}
	}
	mergeAuxiliaryQuality(&eventReport.Quality, auxiliary.Quality)
	if auxiliary.Quality.TaskstatsAvailable {
		eventReport.Quality.SamplingMethod = ebpfSamplingMethod + "+taskstats_counters"
	}
	return eventReport
}

func sameProvenThread(left, right model.ThreadIntervalStats) bool {
	if left.TID != right.TID {
		return false
	}
	if left.StartTimeTicks != 0 && right.StartTimeTicks != 0 {
		return left.StartTimeTicks == right.StartTimeTicks
	}
	if left.StartTimeNanoseconds != 0 && right.StartTimeNanoseconds != 0 {
		return left.StartTimeNanoseconds == right.StartTimeNanoseconds
	}
	return false
}

func copyDelayStats(target *model.ThreadIntervalStats, source model.ThreadIntervalStats) {
	target.DelayVersion = source.DelayVersion
	target.DelayAccountingEnabled = source.DelayAccountingEnabled
	target.DelayAccountingEnabledKnown = source.DelayAccountingEnabledKnown
	target.DelayObserved = source.DelayObserved
	target.DelaySamplePairs = source.DelaySamplePairs
	target.DelayMaxSampleGap = source.DelayMaxSampleGap
	target.DelayCounterResets = source.DelayCounterResets
	target.Delays = source.Delays
}

func mergeAuxiliaryQuality(target *model.IntervalQuality, source model.IntervalQuality) {
	target.SnapshotCount = source.SnapshotCount
	target.MaxScanDuration = source.MaxScanDuration
	target.TaskstatsAvailable = source.TaskstatsAvailable
	target.TaskstatsThreadCount = source.TaskstatsThreadCount
	target.TaskstatsVersionMin = source.TaskstatsVersionMin
	target.TaskstatsVersionMax = source.TaskstatsVersionMax
	target.TaskstatsCounterResets = source.TaskstatsCounterResets
	target.DelayAccountingEnabled = source.DelayAccountingEnabled
	target.DelayAccountingEnabledKnown = source.DelayAccountingEnabledKnown
}

func (c *Collector) withSourceStatus(report model.IntervalReport) model.IntervalReport {
	statusBackend, ok := c.backend.(backend.SourceStatusBackend)
	if !ok {
		return report
	}
	status := statusBackend.SourceStatus()
	report.Quality.ActiveSources = status.Active
	report.Quality.UnavailableSources = status.Unavailable
	return report
}

func flushSchedulerEvents(ctx context.Context, stream model.SchedulerEventStream, acc *EventAccumulator) error {
	if err := stream.Flush(); err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event, ok := <-stream.Events():
			if !ok {
				return fmt.Errorf("ebpf scheduler event stream closed during flush")
			}
			acc.Observe(event)
		case streamErr, ok := <-stream.Errors():
			if !ok {
				return fmt.Errorf("ebpf scheduler error stream closed during flush")
			}
			if streamErr != nil {
				return streamErr
			}
		case _, ok := <-stream.Flushed():
			if !ok {
				return fmt.Errorf("ebpf scheduler flush stream closed unexpectedly")
			}
			drainSchedulerEvents(acc, stream.Events())
			return nil
		}
	}
}

func drainSchedulerEvents(acc *EventAccumulator, events <-chan model.SchedulerEvent) {
	for {
		select {
		case event, ok := <-events:
			if !ok {
				return
			}
			acc.Observe(event)
		default:
			return
		}
	}
}

func durationUntil(at time.Time) time.Duration {
	delay := time.Until(at)
	if delay < 0 {
		return 0
	}
	return delay
}
