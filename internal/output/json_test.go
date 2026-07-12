package output

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"tsastat/internal/model"
)

func TestJSONOutputIsJSONLines(t *testing.T) {
	var buf bytes.Buffer
	renderer := NewJSONRenderer(&buf, 123, "proc", time.Second, 10*time.Millisecond)
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

	var decoded map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &decoded); err != nil {
		t.Fatalf("JSON line is invalid: %v\n%s", err, buf.String())
	}
	if decoded["backend"] != "proc" {
		t.Fatalf("backend = %v, want proc", decoded["backend"])
	}
	if decoded["sample_interval_ms"] != float64(10) {
		t.Fatalf("sample_interval_ms = %v, want 10", decoded["sample_interval_ms"])
	}
	if _, ok := decoded["threads"].([]any); !ok {
		t.Fatalf("threads missing or wrong type: %#v", decoded["threads"])
	}
}
