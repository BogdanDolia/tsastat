package collector

import (
	"sort"
	"time"

	"github.com/BogdanDolia/tsastat/internal/model"
)

const ebpfSamplingMethod = "ebpf_sched_events"

type schedulerTrack struct {
	PID                  int
	TID                  int
	StartTimeTicks       uint64
	StartTimeNanoseconds uint64
	Comm                 string
	state                model.SchedulerState
	since                time.Time
	seedAt               time.Time
	lastEvent            time.Time
	wakeupAt             time.Time
	alive                bool
}

func (t *schedulerTrack) identity() model.ThreadIdentity {
	return model.ThreadIdentity{
		TID:                  t.TID,
		StartTimeTicks:       t.StartTimeTicks,
		StartTimeNanoseconds: t.StartTimeNanoseconds,
	}
}

type schedulerEventWindow struct {
	stats   map[model.ThreadIdentity]*model.ThreadIntervalStats
	quality model.IntervalQuality
}

type EventAccumulator struct {
	interval                    time.Duration
	origin                      time.Time
	tracks                      map[int]*schedulerTrack
	windows                     map[int64]*schedulerEventWindow
	nextWindow                  int64
	clockCalibrationUncertainty time.Duration
	watermark                   time.Time
	advanced                    bool
}

func NewEventAccumulator(interval time.Duration, initial model.ThreadSnapshot) *EventAccumulator {
	initial = normalizeSnapshot(initial)
	origin := initial.FinishedAt
	acc := &EventAccumulator{
		interval:  interval,
		origin:    origin,
		tracks:    make(map[int]*schedulerTrack, len(initial.Samples)),
		windows:   make(map[int64]*schedulerEventWindow),
		watermark: origin,
	}
	for _, sample := range initial.Samples {
		seedAt := sample.Timestamp
		if seedAt.IsZero() {
			seedAt = origin
		}
		acc.tracks[sample.TID] = &schedulerTrack{
			PID:                  sample.PID,
			TID:                  sample.TID,
			StartTimeTicks:       sample.StartTimeTicks,
			StartTimeNanoseconds: sample.StartTimeNanoseconds,
			Comm:                 sample.Comm,
			state:                schedulerStateFromInitialSample(sample.State),
			since:                origin,
			seedAt:               seedAt,
			lastEvent:            seedAt,
			alive:                true,
		}
	}
	return acc
}

func (a *EventAccumulator) Origin() time.Time {
	return a.origin
}

func (a *EventAccumulator) SetClockCalibrationUncertainty(uncertainty time.Duration) {
	if uncertainty > 0 {
		a.clockCalibrationUncertainty = uncertainty
	}
}

func (a *EventAccumulator) NextBoundary() time.Time {
	return a.origin.Add(time.Duration(a.nextWindow+1) * a.interval)
}

func (a *EventAccumulator) Observe(event model.SchedulerEvent) {
	if event.Timestamp.IsZero() || event.TID <= 0 {
		return
	}
	if a.advanced && event.Timestamp.Before(a.watermark) {
		a.recordLateEvent()
		return
	}

	track := a.tracks[event.TID]
	if track != nil && schedulerIdentityChanged(track, event) {
		a.recordInitializationRace(event.Timestamp)
		track.alive = false
		track = nil
	}
	if track == nil {
		track = &schedulerTrack{
			PID:                  event.PID,
			TID:                  event.TID,
			StartTimeTicks:       event.StartTimeTicks,
			StartTimeNanoseconds: event.StartTimeNanoseconds,
			Comm:                 event.Comm,
			state:                model.SchedulerStateUnknown,
			since:                later(a.origin, event.Timestamp),
			seedAt:               event.Timestamp,
			lastEvent:            event.Timestamp,
			alive:                true,
		}
		a.tracks[event.TID] = track
	} else {
		if event.StartTimeTicks != 0 {
			track.StartTimeTicks = event.StartTimeTicks
		}
		if event.StartTimeNanoseconds != 0 {
			track.StartTimeNanoseconds = event.StartTimeNanoseconds
		}
		if event.PID > 0 {
			track.PID = event.PID
		}
		if event.Comm != "" {
			track.Comm = event.Comm
		}
	}

	if event.Timestamp.Before(track.seedAt) {
		return
	}
	if event.Timestamp.Before(a.origin) {
		if event.Timestamp.Before(track.lastEvent) {
			a.recordInitializationRace(a.origin)
			return
		}
		a.applyTransition(track, event, false)
		track.lastEvent = event.Timestamp
		track.since = a.origin
		return
	}
	if event.Timestamp.Before(track.since) || event.Timestamp.Before(track.lastEvent) {
		a.recordLateEvent()
		return
	}

	a.addTrackSegment(track, track.since, event.Timestamp)
	a.recordEvent(track, event)
	a.applyTransition(track, event, true)
	track.since = event.Timestamp
	track.lastEvent = event.Timestamp
}

func schedulerIdentityChanged(track *schedulerTrack, event model.SchedulerEvent) bool {
	return (event.StartTimeTicks != 0 && track.StartTimeTicks != 0 &&
		event.StartTimeTicks != track.StartTimeTicks) ||
		(event.StartTimeNanoseconds != 0 && track.StartTimeNanoseconds != 0 &&
			event.StartTimeNanoseconds != track.StartTimeNanoseconds)
}

func (a *EventAccumulator) Advance(watermark time.Time, lostEventsTotal uint64) []model.IntervalReport {
	if watermark.Before(a.origin) {
		return nil
	}
	if watermark.After(a.watermark) {
		a.watermark = watermark
	}
	a.advanced = true
	for _, track := range a.tracks {
		if !track.alive || !watermark.After(track.since) {
			continue
		}
		a.addTrackSegment(track, track.since, watermark)
		track.since = watermark
	}

	reports := make([]model.IntervalReport, 0, 1)
	for !a.NextBoundary().After(watermark) {
		start := a.origin.Add(time.Duration(a.nextWindow) * a.interval)
		end := start.Add(a.interval)
		window := a.windows[a.nextWindow]
		quality := model.IntervalQuality{
			SamplingMethod:           ebpfSamplingMethod,
			SchedulerEventTimeline:   true,
			SchedulerLostEventsTotal: lostEventsTotal,
		}
		var stats []model.ThreadIntervalStats
		if window != nil {
			quality = window.quality
			quality.SamplingMethod = ebpfSamplingMethod
			quality.SchedulerEventTimeline = true
			quality.SchedulerLostEventsTotal = lostEventsTotal
			stats = sortedEventStats(window.stats)
		}
		quality.MissedTransitionsPossible = lostEventsTotal > 0 ||
			quality.SchedulerLateEvents > 0 || quality.SchedulerIncompleteWakeups > 0 ||
			quality.InitializationRaces > 0
		quality.ClockCalibrationUncertainty = a.clockCalibrationUncertainty

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

func (a *EventAccumulator) applyTransition(track *schedulerTrack, event model.SchedulerEvent, measureWakeup bool) {
	switch event.Kind {
	case model.SchedulerEventWakeup:
		if track.state != model.SchedulerStateOnCPU {
			track.state = model.SchedulerStateRunnable
			if measureWakeup && !event.Timestamp.Before(a.origin) {
				track.wakeupAt = event.Timestamp
			}
		}
	case model.SchedulerEventSwitchIn:
		track.state = model.SchedulerStateOnCPU
		track.alive = true
		track.wakeupAt = time.Time{}
	case model.SchedulerEventSwitchOut:
		track.state = event.State
		track.wakeupAt = time.Time{}
		if event.State == model.SchedulerStateDead {
			track.alive = false
		}
	}
}

func (a *EventAccumulator) recordEvent(track *schedulerTrack, event model.SchedulerEvent) {
	index := a.windowIndex(event.Timestamp)
	stat := a.threadStats(index, track)
	stat.SchedulerEventCount++
	window := a.window(index)
	window.quality.SchedulerEventCount++

	if event.Kind == model.SchedulerEventSwitchOut && !track.wakeupAt.IsZero() {
		stat.IncompleteWakeupCount++
		window.quality.SchedulerIncompleteWakeups++
	}
	if event.Kind != model.SchedulerEventSwitchIn {
		return
	}
	stat.Timeslices++
	if track.wakeupAt.IsZero() || event.Timestamp.Before(track.wakeupAt) {
		return
	}
	latency := event.Timestamp.Sub(track.wakeupAt)
	stat.WakeupCount++
	stat.WakeupLatencyTotal += latency
	if latency > stat.WakeupLatencyMax {
		stat.WakeupLatencyMax = latency
	}
}

func (a *EventAccumulator) addTrackSegment(track *schedulerTrack, start, end time.Time) {
	if !end.After(start) {
		return
	}
	a.forEachWindow(start, end, func(index int64, overlapStart, overlapEnd time.Time) {
		delta := overlapEnd.Sub(overlapStart)
		stat := a.threadStats(index, track)
		stat.TotalObserved += delta
		stat.SchedulerObserved += delta
		switch track.state {
		case model.SchedulerStateOnCPU:
			stat.OnCPU += delta
			stat.Durations[model.StateRunning] += delta
		case model.SchedulerStateRunnable:
			stat.RunqueueWait += delta
			stat.Durations[model.StateRunning] += delta
		case model.SchedulerStateSleeping:
			stat.Durations[model.StateSleeping] += delta
		case model.SchedulerStateUninterruptible:
			stat.Durations[model.StateUninterruptible] += delta
		case model.SchedulerStateStopped:
			stat.Durations[model.StateStopped] += delta
		case model.SchedulerStateTracingStop:
			stat.Durations[model.StateTracingStop] += delta
		case model.SchedulerStateIdle:
			stat.Durations[model.StateIdle] += delta
		case model.SchedulerStateDead:
			stat.Durations[model.StateDead] += delta
		default:
			stat.Durations[model.StateUnknown] += delta
		}
	})
}

func (a *EventAccumulator) recordLateEvent() {
	window := a.window(a.nextWindow)
	window.quality.SchedulerLateEvents++
}

func (a *EventAccumulator) recordInitializationRace(at time.Time) {
	index := a.nextWindow
	if !at.Before(a.origin) {
		index = a.windowIndex(at)
		if index < a.nextWindow {
			index = a.nextWindow
		}
	}
	a.window(index).quality.InitializationRaces++
}

func (a *EventAccumulator) threadStats(index int64, track *schedulerTrack) *model.ThreadIntervalStats {
	window := a.window(index)
	identity := track.identity()
	stat := window.stats[identity]
	if stat == nil {
		start := a.origin.Add(time.Duration(index) * a.interval)
		stat = &model.ThreadIntervalStats{
			PID:                  track.PID,
			TID:                  track.TID,
			StartTimeTicks:       track.StartTimeTicks,
			StartTimeNanoseconds: track.StartTimeNanoseconds,
			Comm:                 track.Comm,
			IntervalStart:        start,
			IntervalEnd:          start.Add(a.interval),
			Durations:            make(map[model.ThreadState]time.Duration),
			SchedulerSource:      ebpfSamplingMethod,
		}
		window.stats[identity] = stat
	}
	if track.Comm != "" {
		stat.Comm = track.Comm
	}
	return stat
}

func (a *EventAccumulator) window(index int64) *schedulerEventWindow {
	window := a.windows[index]
	if window == nil {
		window = &schedulerEventWindow{
			stats: make(map[model.ThreadIdentity]*model.ThreadIntervalStats),
			quality: model.IntervalQuality{
				SamplingMethod:         ebpfSamplingMethod,
				SchedulerEventTimeline: true,
			},
		}
		a.windows[index] = window
	}
	return window
}

func (a *EventAccumulator) forEachWindow(start, end time.Time, fn func(int64, time.Time, time.Time)) {
	if start.Before(a.origin) {
		start = a.origin
	}
	for start.Before(end) {
		index := a.windowIndex(start)
		windowEnd := a.origin.Add(time.Duration(index+1) * a.interval)
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

func (a *EventAccumulator) windowIndex(at time.Time) int64 {
	if !at.After(a.origin) {
		return 0
	}
	return int64(at.Sub(a.origin) / a.interval)
}

func sortedEventStats(statsByIdentity map[model.ThreadIdentity]*model.ThreadIntervalStats) []model.ThreadIntervalStats {
	stats := make([]model.ThreadIntervalStats, 0, len(statsByIdentity))
	for _, stat := range statsByIdentity {
		stats = append(stats, *stat)
	}
	sort.Slice(stats, func(i, j int) bool {
		if stats[i].TID != stats[j].TID {
			return stats[i].TID < stats[j].TID
		}
		if stats[i].StartTimeNanoseconds != stats[j].StartTimeNanoseconds {
			return stats[i].StartTimeNanoseconds < stats[j].StartTimeNanoseconds
		}
		return stats[i].StartTimeTicks < stats[j].StartTimeTicks
	})
	return stats
}

func schedulerStateFromInitialSample(state model.ThreadState) model.SchedulerState {
	switch state {
	case model.StateSleeping:
		return model.SchedulerStateSleeping
	case model.StateUninterruptible:
		return model.SchedulerStateUninterruptible
	case model.StateStopped:
		return model.SchedulerStateStopped
	case model.StateTracingStop:
		return model.SchedulerStateTracingStop
	case model.StateIdle:
		return model.SchedulerStateIdle
	case model.StateDead, model.StateZombie:
		return model.SchedulerStateDead
	default:
		// Proc reports both on-CPU and runnable tasks as R. Do not guess which
		// scheduler state applies before the first eBPF event.
		return model.SchedulerStateUnknown
	}
}

func later(left, right time.Time) time.Time {
	if right.After(left) {
		return right
	}
	return left
}
