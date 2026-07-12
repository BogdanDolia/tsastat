package collector

import (
	"sort"
	"time"

	"github.com/BogdanDolia/tsastat/internal/model"
)

type Accumulator struct {
	previous map[int]model.ThreadSample
	window   map[int]*model.ThreadIntervalStats
}

func NewAccumulator() *Accumulator {
	return &Accumulator{
		previous: make(map[int]model.ThreadSample),
		window:   make(map[int]*model.ThreadIntervalStats),
	}
}

func (a *Accumulator) Observe(samples []model.ThreadSample) {
	current := make(map[int]model.ThreadSample, len(samples))
	for _, sample := range samples {
		current[sample.TID] = sample
	}

	if len(a.previous) == 0 {
		a.previous = current
		return
	}

	now, ok := snapshotTime(samples)
	if !ok {
		a.previous = current
		return
	}

	tids := make([]int, 0, len(a.previous))
	for tid := range a.previous {
		tids = append(tids, tid)
	}
	sort.Ints(tids)

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

		stat, exists := a.window[tid]
		if !exists {
			stat = &model.ThreadIntervalStats{
				PID:           prev.PID,
				TID:           tid,
				Comm:          comm,
				IntervalStart: prev.Timestamp,
				Durations:     make(map[model.ThreadState]time.Duration),
			}
			a.window[tid] = stat
		}
		stat.Comm = comm
		stat.IntervalEnd = end
		stat.Durations[prev.State] += delta
		stat.TotalObserved += delta
	}

	a.previous = current
}

func (a *Accumulator) Flush() []model.ThreadIntervalStats {
	tids := make([]int, 0, len(a.window))
	for tid := range a.window {
		tids = append(tids, tid)
	}
	sort.Ints(tids)

	stats := make([]model.ThreadIntervalStats, 0, len(tids))
	for _, tid := range tids {
		stats = append(stats, *a.window[tid])
	}
	a.window = make(map[int]*model.ThreadIntervalStats)
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
