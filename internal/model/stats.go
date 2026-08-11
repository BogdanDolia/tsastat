package model

import "time"

type ThreadIntervalStats struct {
	PID            int
	TID            int
	StartTimeTicks uint64
	Comm           string

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

func (s ThreadIntervalStats) OnCPUPercent() float64 {
	if s.SchedstatObserved <= 0 {
		return 0
	}
	return float64(s.OnCPU) * 100 / float64(s.SchedstatObserved)
}

func (s ThreadIntervalStats) RunqueueWaitPercent() float64 {
	if s.SchedstatObserved <= 0 {
		return 0
	}
	return float64(s.RunqueueWait) * 100 / float64(s.SchedstatObserved)
}

type IntervalQuality struct {
	SamplingMethod            string
	SnapshotCount             int
	MaxScanDuration           time.Duration
	MaxSampleGap              time.Duration
	MissedTransitionsPossible bool
	SchedstatAvailable        bool
	SchedstatThreadCount      int
	SchedstatCounterResets    int
}

type IntervalReport struct {
	IntervalStart time.Time
	IntervalEnd   time.Time
	Threads       []ThreadIntervalStats
	Quality       IntervalQuality
}
