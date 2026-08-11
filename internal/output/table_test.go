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
		}},
	})
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}

	out := buf.String()
	for _, column := range []string{"TIME", "PID", "TID", "START_TICKS", "COMM", "RUN_ms", "CPU_ms", "RQ_ms", "SLICES", "SS_OBS_ms", "SLEEP_ms", "D_ms", "STOP_ms", "Z_ms", "UNK_ms", "OBS_ms", "GAP_ms", "UNCERT_ms", "RUN_%", "SLEEP_%", "D_%"} {
		if !strings.Contains(out, column) {
			t.Fatalf("table output missing column %q:\n%s", column, out)
		}
	}
	if !strings.Contains(out, "worker") {
		t.Fatalf("table output missing row:\n%s", out)
	}
}
