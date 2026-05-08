package output

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"tsastat/internal/model"
)

func TestTableOutputContainsExpectedColumns(t *testing.T) {
	var buf bytes.Buffer
	renderer := NewTableRenderer(&buf, false)
	err := renderer.Render([]model.ThreadIntervalStats{
		{
			PID:           123,
			TID:           124,
			Comm:          "worker",
			IntervalEnd:   time.Unix(0, 0),
			Durations:     map[model.ThreadState]time.Duration{model.StateRunning: time.Second},
			TotalObserved: time.Second,
		},
	})
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}

	out := buf.String()
	for _, column := range []string{"TIME", "PID", "TID", "COMM", "RUN_ms", "SLEEP_ms", "D_ms", "STOP_ms", "Z_ms", "RUN_%", "SLEEP_%", "D_%"} {
		if !strings.Contains(out, column) {
			t.Fatalf("table output missing column %q:\n%s", column, out)
		}
	}
	if !strings.Contains(out, "worker") {
		t.Fatalf("table output missing row:\n%s", out)
	}
}
