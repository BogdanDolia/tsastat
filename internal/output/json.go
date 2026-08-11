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
			TID:                  stat.TID,
			StartTimeTicks:       stat.StartTimeTicks,
			StartTimeNanoseconds: stat.StartTimeNanoseconds,
			Comm:                 stat.Comm,
			DurationsMS:          durationsMS(stat),
			Percent:              percents(stat),
			Quality:              newJSONThreadQuality(stat),
			Scheduler:            newJSONSchedulerMetrics(stat),
			Delays:               newJSONDelayMetrics(stat),
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
	TID                  int                  `json:"tid"`
	StartTimeTicks       uint64               `json:"start_time_ticks"`
	StartTimeNanoseconds uint64               `json:"start_time_ns,omitempty"`
	Comm                 string               `json:"comm"`
	DurationsMS          map[string]int64     `json:"durations_ms"`
	Percent              map[string]float64   `json:"percent"`
	Quality              jsonThreadQuality    `json:"quality"`
	Scheduler            jsonSchedulerMetrics `json:"scheduler"`
	Delays               jsonDelayMetrics     `json:"delays"`
}

type jsonIntervalQuality struct {
	ActiveSources                 []string `json:"active_sources"`
	UnavailableSources            []string `json:"unavailable_sources"`
	HybridIdentityMismatches      int      `json:"hybrid_identity_mismatches"`
	SamplingMethod                string   `json:"sampling_method"`
	SnapshotCount                 int      `json:"snapshot_count"`
	MaxScanDurationMS             int64    `json:"max_scan_duration_ms"`
	MaxSampleGapMS                int64    `json:"max_sample_gap_ms"`
	MissedTransitionsPossible     bool     `json:"missed_transitions_possible"`
	SchedstatAvailable            bool     `json:"schedstat_available"`
	SchedstatThreadCount          int      `json:"schedstat_thread_count"`
	SchedstatCounterResets        int      `json:"schedstat_counter_resets"`
	SchedulerEventTimeline        bool     `json:"scheduler_event_timeline"`
	SchedulerEventCount           int      `json:"scheduler_event_count"`
	SchedulerLostEventsTotal      uint64   `json:"scheduler_lost_events_total"`
	SchedulerLateEvents           int      `json:"scheduler_late_events"`
	SchedulerIncompleteWakeups    int      `json:"scheduler_incomplete_wakeups"`
	InitializationRaces           int      `json:"initialization_races"`
	ClockCalibrationUncertaintyNS int64    `json:"clock_calibration_uncertainty_ns"`
	TaskstatsAvailable            bool     `json:"taskstats_available"`
	TaskstatsThreadCount          int      `json:"taskstats_thread_count"`
	TaskstatsVersionMin           uint16   `json:"taskstats_version_min"`
	TaskstatsVersionMax           uint16   `json:"taskstats_version_max"`
	TaskstatsCounterResets        int      `json:"taskstats_counter_resets"`
	DelayAccountingEnabled        bool     `json:"kernel_task_delayacct_enabled"`
	DelayAccountingEnabledKnown   bool     `json:"kernel_task_delayacct_enabled_known"`
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
	EventTimed                     bool    `json:"event_timed"`
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
	EventCount                     int     `json:"event_count"`
	WakeupCount                    int     `json:"wakeup_count"`
	WakeupLatencyTotalUS           int64   `json:"wakeup_latency_total_us"`
	WakeupLatencyAverageUS         int64   `json:"wakeup_latency_avg_us"`
	WakeupLatencyMaxUS             int64   `json:"wakeup_latency_max_us"`
	IncompleteWakeupCount          int     `json:"incomplete_wakeup_count"`
}

type jsonDelayMetrics struct {
	Available                      bool             `json:"available"`
	Source                         string           `json:"source"`
	Version                        uint16           `json:"version"`
	AccountingEnabled              bool             `json:"kernel_task_delayacct_enabled"`
	AccountingEnabledKnown         bool             `json:"kernel_task_delayacct_enabled_known"`
	CounterDeltasExactBetweenReads bool             `json:"counter_deltas_exact_between_reads"`
	FieldPairsAtomic               bool             `json:"field_pairs_atomic"`
	WindowAllocation               string           `json:"window_allocation"`
	ObservedMS                     int64            `json:"observed_ms"`
	SamplePairs                    int              `json:"sample_pairs"`
	MaxSampleGapMS                 int64            `json:"max_sample_gap_ms"`
	CounterResets                  int              `json:"counter_resets"`
	CPU                            jsonDelayCounter `json:"cpu"`
	BlockIO                        jsonDelayCounter `json:"block_io"`
	SwapIn                         jsonDelayCounter `json:"swap_in"`
	Reclaim                        jsonDelayCounter `json:"reclaim"`
	Thrashing                      jsonDelayCounter `json:"thrashing"`
	Compaction                     jsonDelayCounter `json:"compaction"`
	WriteProtectCopy               jsonDelayCounter `json:"write_protect_copy"`
	IRQ                            jsonDelayCounter `json:"irq"`
}

type jsonDelayCounter struct {
	Available bool   `json:"available"`
	Count     uint64 `json:"count"`
	TotalNS   int64  `json:"total_ns"`
	AverageNS int64  `json:"average_ns"`
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
		ActiveSources:                 append([]string{}, quality.ActiveSources...),
		UnavailableSources:            append([]string{}, quality.UnavailableSources...),
		HybridIdentityMismatches:      quality.HybridIdentityMismatches,
		SamplingMethod:                quality.SamplingMethod,
		SnapshotCount:                 quality.SnapshotCount,
		MaxScanDurationMS:             quality.MaxScanDuration.Milliseconds(),
		MaxSampleGapMS:                quality.MaxSampleGap.Milliseconds(),
		MissedTransitionsPossible:     quality.MissedTransitionsPossible,
		SchedstatAvailable:            quality.SchedstatAvailable,
		SchedstatThreadCount:          quality.SchedstatThreadCount,
		SchedstatCounterResets:        quality.SchedstatCounterResets,
		SchedulerEventTimeline:        quality.SchedulerEventTimeline,
		SchedulerEventCount:           quality.SchedulerEventCount,
		SchedulerLostEventsTotal:      quality.SchedulerLostEventsTotal,
		SchedulerLateEvents:           quality.SchedulerLateEvents,
		SchedulerIncompleteWakeups:    quality.SchedulerIncompleteWakeups,
		InitializationRaces:           quality.InitializationRaces,
		ClockCalibrationUncertaintyNS: quality.ClockCalibrationUncertainty.Nanoseconds(),
		TaskstatsAvailable:            quality.TaskstatsAvailable,
		TaskstatsThreadCount:          quality.TaskstatsThreadCount,
		TaskstatsVersionMin:           quality.TaskstatsVersionMin,
		TaskstatsVersionMax:           quality.TaskstatsVersionMax,
		TaskstatsCounterResets:        quality.TaskstatsCounterResets,
		DelayAccountingEnabled:        quality.DelayAccountingEnabled,
		DelayAccountingEnabledKnown:   quality.DelayAccountingEnabledKnown,
	}
}

func newJSONDelayMetrics(stat model.ThreadIntervalStats) jsonDelayMetrics {
	available := stat.DelayCountersAvailable()
	source := ""
	windowAllocation := "unavailable"
	if available {
		source = "linux_taskstats_delayacct"
		windowAllocation = "proportional_by_wall_time"
	}
	return jsonDelayMetrics{
		Available:                      available,
		Source:                         source,
		Version:                        stat.DelayVersion,
		AccountingEnabled:              stat.DelayAccountingEnabled,
		AccountingEnabledKnown:         stat.DelayAccountingEnabledKnown,
		CounterDeltasExactBetweenReads: available,
		FieldPairsAtomic:               false,
		WindowAllocation:               windowAllocation,
		ObservedMS:                     stat.DelayObserved.Milliseconds(),
		SamplePairs:                    stat.DelaySamplePairs,
		MaxSampleGapMS:                 stat.DelayMaxSampleGap.Milliseconds(),
		CounterResets:                  stat.DelayCounterResets,
		CPU:                            newJSONDelayCounter(stat.Delays.CPU),
		BlockIO:                        newJSONDelayCounter(stat.Delays.BlockIO),
		SwapIn:                         newJSONDelayCounter(stat.Delays.SwapIn),
		Reclaim:                        newJSONDelayCounter(stat.Delays.Reclaim),
		Thrashing:                      newJSONDelayCounter(stat.Delays.Thrashing),
		Compaction:                     newJSONDelayCounter(stat.Delays.Compaction),
		WriteProtectCopy:               newJSONDelayCounter(stat.Delays.WriteProtectCopy),
		IRQ:                            newJSONDelayCounter(stat.Delays.IRQ),
	}
}

func newJSONDelayCounter(counter model.DelayIntervalCounter) jsonDelayCounter {
	return jsonDelayCounter{
		Available: counter.Available,
		Count:     counter.Count,
		TotalNS:   counter.Total.Nanoseconds(),
		AverageNS: counter.Average().Nanoseconds(),
	}
}

func newJSONSchedulerMetrics(stat model.ThreadIntervalStats) jsonSchedulerMetrics {
	available := stat.SchedulerAvailable() || stat.SchedstatAvailable()
	source := stat.SchedulerSource
	if source == "" && stat.SchedstatAvailable() {
		source = "proc_schedstat"
	}
	eventTimed := source == "ebpf_sched_events"
	windowAllocation := "unavailable"
	if eventTimed {
		windowAllocation = "exact_event_timestamps"
	} else if available {
		windowAllocation = "proportional_by_wall_time"
	}
	observed := stat.SchedulerObserved
	if observed <= 0 {
		observed = stat.SchedstatObserved
	}
	return jsonSchedulerMetrics{
		Available:                      available,
		Source:                         source,
		CounterDeltasExactBetweenReads: source == "proc_schedstat" && available,
		EventTimed:                     eventTimed,
		WindowAllocation:               windowAllocation,
		OnCPUMS:                        stat.OnCPU.Milliseconds(),
		RunqueueWaitMS:                 stat.RunqueueWait.Milliseconds(),
		Timeslices:                     stat.Timeslices,
		ObservedMS:                     observed.Milliseconds(),
		OnCPUPercent:                   stat.OnCPUPercent(),
		RunqueueWaitPercent:            stat.RunqueueWaitPercent(),
		SamplePairs:                    stat.SchedstatSamplePairs,
		MaxSampleGapMS:                 stat.SchedstatMaxSampleGap.Milliseconds(),
		CounterResets:                  stat.SchedstatCounterResets,
		EventCount:                     stat.SchedulerEventCount,
		WakeupCount:                    stat.WakeupCount,
		WakeupLatencyTotalUS:           stat.WakeupLatencyTotal.Microseconds(),
		WakeupLatencyAverageUS:         stat.AverageWakeupLatency().Microseconds(),
		WakeupLatencyMaxUS:             stat.WakeupLatencyMax.Microseconds(),
		IncompleteWakeupCount:          stat.IncompleteWakeupCount,
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
