package collector

import (
	"sort"
	"time"

	"tsastat/internal/model"
)

type Accumulator struct {
	previous map[int]model.ThreadSample
}

func NewAccumulator() *Accumulator {
	return &Accumulator{previous: make(map[int]model.ThreadSample)}
}

func (a *Accumulator) Observe(samples []model.ThreadSample) []model.ThreadIntervalStats {
	current := make(map[int]model.ThreadSample, len(samples))
	for _, sample := range samples {
		current[sample.TID] = sample
	}

	if len(a.previous) == 0 {
		a.previous = current
		return nil
	}

	now, ok := snapshotTime(samples)
	if !ok {
		a.previous = current
		return nil
	}

	tids := make([]int, 0, len(a.previous))
	for tid := range a.previous {
		tids = append(tids, tid)
	}
	sort.Ints(tids)

	stats := make([]model.ThreadIntervalStats, 0, len(tids))
	for _, tid := range tids {
		prev := a.previous[tid]
		end := now
		if sample, exists := current[tid]; exists {
			end = sample.Timestamp
		}
		delta := end.Sub(prev.Timestamp)
		if delta < 0 {
			delta = 0
		}

		comm := prev.Comm
		if sample, exists := current[tid]; exists && sample.Comm != "" {
			comm = sample.Comm
		}

		durations := map[model.ThreadState]time.Duration{
			prev.State: delta,
		}
		stats = append(stats, model.ThreadIntervalStats{
			PID:           prev.PID,
			TID:           tid,
			Comm:          comm,
			IntervalStart: prev.Timestamp,
			IntervalEnd:   end,
			Durations:     durations,
			TotalObserved: delta,
		})
	}

	a.previous = current
	return stats
}

func snapshotTime(samples []model.ThreadSample) (time.Time, bool) {
	for _, sample := range samples {
		if !sample.Timestamp.IsZero() {
			return sample.Timestamp, true
		}
	}
	return time.Time{}, false
}
