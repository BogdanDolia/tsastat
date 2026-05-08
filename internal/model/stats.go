package model

import "time"

type ThreadIntervalStats struct {
	PID  int
	TID  int
	Comm string

	IntervalStart time.Time
	IntervalEnd   time.Time

	Durations     map[ThreadState]time.Duration
	TotalObserved time.Duration
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
