package output

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/BogdanDolia/tsastat/internal/model"
)

func TestTableOutputContainsExpectedColumns(t *testing.T) {
	var buf bytes.Buffer
	renderer := NewTableRenderer(&buf, false)
	err := renderer.Render(model.IntervalReport{
		IntervalStart: time.Unix(0, 0),
		IntervalEnd:   time.Unix(1, 0),
		Threads: []model.ThreadIntervalStats{{
			PID:                  123,
			TID:                  124,
			Comm:                 "worker",
			IntervalEnd:          time.Unix(1, 0),
			Durations:            map[model.ThreadState]time.Duration{model.StateRunning: time.Second},
			TotalObserved:        time.Second,
			MaxSampleGap:         10 * time.Millisecond,
			OnCPU:                250 * time.Millisecond,
			RunqueueWait:         50 * time.Millisecond,
			Timeslices:           17,
			SchedstatObserved:    time.Second,
			SchedstatSamplePairs: 100,
			SchedulerSource:      "proc_schedstat",
			SchedulerObserved:    time.Second,
		}},
	})
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}

	out := buf.String()
	for _, column := range []string{"TIME", "PID", "TID", "START_TICKS", "START_NS", "COMM", "RUN_ms", "CPU_ms", "RQ_ms", "SLICES", "SCHED_OBS_ms", "EVTS", "WAKEUPS", "INCOMP_WAKE", "WAKE_AVG_us", "WAKE_MAX_us", "LOST_TOTAL", "LATE", "INIT_RACE", "CLK_UNCERT_ns", "SLEEP_ms", "D_ms", "STOP_ms", "Z_ms", "UNK_ms", "OBS_ms", "GAP_ms", "UNCERT_ms", "RUN_%", "SLEEP_%", "D_%"} {
		if !strings.Contains(out, column) {
			t.Fatalf("table output missing column %q:\n%s", column, out)
		}
	}
	if !strings.Contains(out, "worker") {
		t.Fatalf("table output missing row:\n%s", out)
	}
}

func TestTableOutputContainsEventMetrics(t *testing.T) {
	var buf bytes.Buffer
	renderer := NewTableRenderer(&buf, false)
	err := renderer.Render(model.IntervalReport{
		Quality: model.IntervalQuality{
			SchedulerEventTimeline:      true,
			SchedulerLostEventsTotal:    3,
			SchedulerLateEvents:         1,
			InitializationRaces:         2,
			ClockCalibrationUncertainty: 180 * time.Nanosecond,
		},
		Threads: []model.ThreadIntervalStats{{
			PID:                  123,
			TID:                  124,
			StartTimeNanoseconds: 987654,
			Comm:                 "worker",
			IntervalEnd:          time.Unix(1, 0),
			Durations:            map[model.ThreadState]time.Duration{model.StateRunning: 300 * time.Millisecond},
			TotalObserved:        time.Second,
			OnCPU:                250 * time.Millisecond,
			RunqueueWait:         50 * time.Millisecond,
			Timeslices:           4,
			SchedulerSource:      "ebpf_sched_events",
			SchedulerObserved:    time.Second,
			SchedulerEventCount:  8,
			WakeupCount:          2,
			WakeupLatencyTotal:   80 * time.Microsecond,
			WakeupLatencyMax:     60 * time.Microsecond,
		}},
	})
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}
	for _, value := range []string{"987654", "250", "50", "1000", "8", "2", "40", "60", "180"} {
		if !strings.Contains(buf.String(), value) {
			t.Fatalf("table output missing %q:\n%s", value, buf.String())
		}
	}
}
