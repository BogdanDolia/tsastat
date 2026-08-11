package model

import "time"

type ThreadIntervalStats struct {
	PID                  int
	TID                  int
	StartTimeTicks       uint64
	StartTimeNanoseconds uint64
	Comm                 string

	IntervalStart time.Time
	IntervalEnd   time.Time

	Durations           map[ThreadState]time.Duration
	TotalObserved       time.Duration
	SampleCount         int
	MaxSampleGap        time.Duration
	DetectedTransitions int
	// DetectedTransitionUncertainty sums the per-window timing ambiguity of
	// endpoint state changes. It does not cover missed transitions whose
	// endpoint states were equal.
	DetectedTransitionUncertainty time.Duration

	OnCPU                  time.Duration
	RunqueueWait           time.Duration
	Timeslices             uint64
	SchedstatObserved      time.Duration
	SchedstatSamplePairs   int
	SchedstatMaxSampleGap  time.Duration
	SchedstatCounterResets int

	SchedulerSource       string
	SchedulerObserved     time.Duration
	SchedulerEventCount   int
	WakeupCount           int
	WakeupLatencyTotal    time.Duration
	WakeupLatencyMax      time.Duration
	IncompleteWakeupCount int

	DelayVersion                uint16
	DelayAccountingEnabled      bool
	DelayAccountingEnabledKnown bool
	DelayObserved               time.Duration
	DelaySamplePairs            int
	DelayMaxSampleGap           time.Duration
	DelayCounterResets          int
	Delays                      DelayIntervalCounters
}

type DelayIntervalCounter struct {
	Available bool
	Count     uint64
	Total     time.Duration
}

func (c DelayIntervalCounter) Average() time.Duration {
	if c.Count == 0 || c.Total <= 0 {
		return 0
	}
	return time.Duration(uint64(c.Total) / c.Count)
}

type DelayIntervalCounters struct {
	CPU              DelayIntervalCounter
	BlockIO          DelayIntervalCounter
	SwapIn           DelayIntervalCounter
	Reclaim          DelayIntervalCounter
	Thrashing        DelayIntervalCounter
	Compaction       DelayIntervalCounter
	WriteProtectCopy DelayIntervalCounter
	IRQ              DelayIntervalCounter
}

func (s ThreadIntervalStats) Duration(state ThreadState) time.Duration {
	if s.Durations == nil {
		return 0
	}
	return s.Durations[state]
}

func (s ThreadIntervalStats) Percent(state ThreadState) float64 {
	if s.TotalObserved <= 0 {
		return 0
	}
	return float64(s.Duration(state)) * 100 / float64(s.TotalObserved)
}

func (s ThreadIntervalStats) StopDuration() time.Duration {
	return s.Duration(StateStopped) + s.Duration(StateTracingStop)
}

func (s ThreadIntervalStats) UnknownDuration() time.Duration {
	return s.Duration(StateUnknown)
}

func (s ThreadIntervalStats) SchedstatAvailable() bool {
	return s.SchedstatSamplePairs > 0
}

func (s ThreadIntervalStats) SchedulerAvailable() bool {
	return s.SchedulerSource != ""
}

func (s ThreadIntervalStats) schedulerObserved() time.Duration {
	if s.SchedulerObserved > 0 {
		return s.SchedulerObserved
	}
	return s.SchedstatObserved
}

func (s ThreadIntervalStats) OnCPUPercent() float64 {
	observed := s.schedulerObserved()
	if observed <= 0 {
		return 0
	}
	return float64(s.OnCPU) * 100 / float64(observed)
}

func (s ThreadIntervalStats) RunqueueWaitPercent() float64 {
	observed := s.schedulerObserved()
	if observed <= 0 {
		return 0
	}
	return float64(s.RunqueueWait) * 100 / float64(observed)
}

func (s ThreadIntervalStats) AverageWakeupLatency() time.Duration {
	if s.WakeupCount == 0 {
		return 0
	}
	return s.WakeupLatencyTotal / time.Duration(s.WakeupCount)
}

func (s ThreadIntervalStats) DelayCountersAvailable() bool {
	return s.DelaySamplePairs > 0
}

type IntervalQuality struct {
	ActiveSources               []string
	UnavailableSources          []string
	HybridIdentityMismatches    int
	SamplingMethod              string
	SnapshotCount               int
	MaxScanDuration             time.Duration
	MaxSampleGap                time.Duration
	MissedTransitionsPossible   bool
	SchedstatAvailable          bool
	SchedstatThreadCount        int
	SchedstatCounterResets      int
	SchedulerEventTimeline      bool
	SchedulerEventCount         int
	SchedulerLostEventsTotal    uint64
	SchedulerLateEvents         int
	SchedulerIncompleteWakeups  int
	InitializationRaces         int
	ClockCalibrationUncertainty time.Duration
	TaskstatsAvailable          bool
	TaskstatsThreadCount        int
	TaskstatsVersionMin         uint16
	TaskstatsVersionMax         uint16
	TaskstatsCounterResets      int
	DelayAccountingEnabled      bool
	DelayAccountingEnabledKnown bool
}

type IntervalReport struct {
	IntervalStart time.Time
	IntervalEnd   time.Time
	Threads       []ThreadIntervalStats
	Quality       IntervalQuality
}
