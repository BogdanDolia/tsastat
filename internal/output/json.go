package output

import (
	"encoding/json"
	"io"
	"time"

	"github.com/BogdanDolia/tsastat/internal/model"
)

type JSONRenderer struct {
	encoder        *json.Encoder
	pid            int
	backend        string
	interval       time.Duration
	sampleInterval time.Duration
}

func NewJSONRenderer(w io.Writer, pid int, backendName string, interval, sampleInterval time.Duration) *JSONRenderer {
	enc := json.NewEncoder(w)
	return &JSONRenderer{
		encoder:        enc,
		pid:            pid,
		backend:        backendName,
		interval:       interval,
		sampleInterval: sampleInterval,
	}
}

func (r *JSONRenderer) Render(report model.IntervalReport) error {
	event := jsonInterval{
		Timestamp:        report.IntervalEnd.UTC().Format(time.RFC3339Nano),
		IntervalStart:    report.IntervalStart.UTC().Format(time.RFC3339Nano),
		IntervalEnd:      report.IntervalEnd.UTC().Format(time.RFC3339Nano),
		PID:              r.pid,
		Backend:          r.backend,
		IntervalMS:       intervalDuration(report, r.interval).Milliseconds(),
		SampleIntervalMS: r.sampleInterval.Milliseconds(),
		Quality:          newJSONIntervalQuality(report.Quality),
		Threads:          make([]jsonThread, 0, len(report.Threads)),
	}

	for _, stat := range report.Threads {
		event.Threads = append(event.Threads, jsonThread{
			TID:            stat.TID,
			StartTimeTicks: stat.StartTimeTicks,
			Comm:           stat.Comm,
			DurationsMS:    durationsMS(stat),
			Percent:        percents(stat),
			Quality:        newJSONThreadQuality(stat),
			Scheduler:      newJSONSchedulerMetrics(stat),
		})
	}

	return r.encoder.Encode(event)
}

type jsonInterval struct {
	Timestamp        string              `json:"timestamp"`
	IntervalStart    string              `json:"interval_start"`
	IntervalEnd      string              `json:"interval_end"`
	PID              int                 `json:"pid"`
	Backend          string              `json:"backend"`
	IntervalMS       int64               `json:"interval_ms"`
	SampleIntervalMS int64               `json:"sample_interval_ms"`
	Quality          jsonIntervalQuality `json:"quality"`
	Threads          []jsonThread        `json:"threads"`
}

type jsonThread struct {
	TID            int                  `json:"tid"`
	StartTimeTicks uint64               `json:"start_time_ticks"`
	Comm           string               `json:"comm"`
	DurationsMS    map[string]int64     `json:"durations_ms"`
	Percent        map[string]float64   `json:"percent"`
	Quality        jsonThreadQuality    `json:"quality"`
	Scheduler      jsonSchedulerMetrics `json:"scheduler"`
}

type jsonIntervalQuality struct {
	SamplingMethod            string `json:"sampling_method"`
	SnapshotCount             int    `json:"snapshot_count"`
	MaxScanDurationMS         int64  `json:"max_scan_duration_ms"`
	MaxSampleGapMS            int64  `json:"max_sample_gap_ms"`
	MissedTransitionsPossible bool   `json:"missed_transitions_possible"`
	SchedstatAvailable        bool   `json:"schedstat_available"`
	SchedstatThreadCount      int    `json:"schedstat_thread_count"`
	SchedstatCounterResets    int    `json:"schedstat_counter_resets"`
}

type jsonThreadQuality struct {
	TrackedMS                     int64 `json:"tracked_ms"`
	Samples                       int   `json:"samples"`
	MaxSampleGapMS                int64 `json:"max_sample_gap_ms"`
	DetectedTransitions           int   `json:"detected_transitions"`
	DetectedTransitionUncertainty int64 `json:"detected_transition_uncertainty_ms"`
}

type jsonSchedulerMetrics struct {
	Available                      bool    `json:"available"`
	Source                         string  `json:"source"`
	CounterDeltasExactBetweenReads bool    `json:"counter_deltas_exact_between_reads"`
	WindowAllocation               string  `json:"window_allocation"`
	OnCPUMS                        int64   `json:"on_cpu_ms"`
	RunqueueWaitMS                 int64   `json:"runqueue_wait_ms"`
	Timeslices                     uint64  `json:"timeslices"`
	ObservedMS                     int64   `json:"observed_ms"`
	OnCPUPercent                   float64 `json:"on_cpu_percent"`
	RunqueueWaitPercent            float64 `json:"runqueue_wait_percent"`
	SamplePairs                    int     `json:"sample_pairs"`
	MaxSampleGapMS                 int64   `json:"max_sample_gap_ms"`
	CounterResets                  int     `json:"counter_resets"`
}

func durationsMS(stat model.ThreadIntervalStats) map[string]int64 {
	return map[string]int64{
		string(model.StateRunning):         stat.Duration(model.StateRunning).Milliseconds(),
		string(model.StateSleeping):        stat.Duration(model.StateSleeping).Milliseconds(),
		string(model.StateUninterruptible): stat.Duration(model.StateUninterruptible).Milliseconds(),
		string(model.StateStopped):         stat.Duration(model.StateStopped).Milliseconds(),
		string(model.StateTracingStop):     stat.Duration(model.StateTracingStop).Milliseconds(),
		string(model.StateZombie):          stat.Duration(model.StateZombie).Milliseconds(),
		string(model.StateDead):            stat.Duration(model.StateDead).Milliseconds(),
		string(model.StateIdle):            stat.Duration(model.StateIdle).Milliseconds(),
		string(model.StateUnknown):         stat.Duration(model.StateUnknown).Milliseconds(),
	}
}

func percents(stat model.ThreadIntervalStats) map[string]float64 {
	return map[string]float64{
		string(model.StateRunning):         stat.Percent(model.StateRunning),
		string(model.StateSleeping):        stat.Percent(model.StateSleeping),
		string(model.StateUninterruptible): stat.Percent(model.StateUninterruptible),
		string(model.StateUnknown):         stat.Percent(model.StateUnknown),
	}
}

func newJSONIntervalQuality(quality model.IntervalQuality) jsonIntervalQuality {
	return jsonIntervalQuality{
		SamplingMethod:            quality.SamplingMethod,
		SnapshotCount:             quality.SnapshotCount,
		MaxScanDurationMS:         quality.MaxScanDuration.Milliseconds(),
		MaxSampleGapMS:            quality.MaxSampleGap.Milliseconds(),
		MissedTransitionsPossible: quality.MissedTransitionsPossible,
		SchedstatAvailable:        quality.SchedstatAvailable,
		SchedstatThreadCount:      quality.SchedstatThreadCount,
		SchedstatCounterResets:    quality.SchedstatCounterResets,
	}
}

func newJSONSchedulerMetrics(stat model.ThreadIntervalStats) jsonSchedulerMetrics {
	available := stat.SchedstatAvailable()
	windowAllocation := "unavailable"
	if available {
		windowAllocation = "proportional_by_wall_time"
	}
	return jsonSchedulerMetrics{
		Available:                      available,
		Source:                         "proc_schedstat",
		CounterDeltasExactBetweenReads: available,
		WindowAllocation:               windowAllocation,
		OnCPUMS:                        stat.OnCPU.Milliseconds(),
		RunqueueWaitMS:                 stat.RunqueueWait.Milliseconds(),
		Timeslices:                     stat.Timeslices,
		ObservedMS:                     stat.SchedstatObserved.Milliseconds(),
		OnCPUPercent:                   stat.OnCPUPercent(),
		RunqueueWaitPercent:            stat.RunqueueWaitPercent(),
		SamplePairs:                    stat.SchedstatSamplePairs,
		MaxSampleGapMS:                 stat.SchedstatMaxSampleGap.Milliseconds(),
		CounterResets:                  stat.SchedstatCounterResets,
	}
}

func newJSONThreadQuality(stat model.ThreadIntervalStats) jsonThreadQuality {
	return jsonThreadQuality{
		TrackedMS:                     stat.TotalObserved.Milliseconds(),
		Samples:                       stat.SampleCount,
		MaxSampleGapMS:                stat.MaxSampleGap.Milliseconds(),
		DetectedTransitions:           stat.DetectedTransitions,
		DetectedTransitionUncertainty: stat.DetectedTransitionUncertainty.Milliseconds(),
	}
}

func intervalDuration(report model.IntervalReport, fallback time.Duration) time.Duration {
	if duration := report.IntervalEnd.Sub(report.IntervalStart); duration > 0 {
		return duration
	}
	return fallback
}
