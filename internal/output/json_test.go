package output

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/BogdanDolia/tsastat/internal/model"
)

func TestJSONOutputIsJSONLines(t *testing.T) {
	var buf bytes.Buffer
	renderer := NewJSONRenderer(&buf, 123, "proc", time.Second, 10*time.Millisecond)
	err := renderer.Render(model.IntervalReport{
		IntervalStart: time.Unix(0, 0),
		IntervalEnd:   time.Unix(1, 0),
		Quality: model.IntervalQuality{
			SamplingMethod:            "procfs_midpoint",
			SnapshotCount:             101,
			MaxScanDuration:           2 * time.Millisecond,
			MaxSampleGap:              12 * time.Millisecond,
			MissedTransitionsPossible: true,
			SchedstatAvailable:        true,
			SchedstatThreadCount:      1,
		},
		Threads: []model.ThreadIntervalStats{{
			PID:                   123,
			TID:                   124,
			StartTimeTicks:        456,
			Comm:                  "worker",
			IntervalStart:         time.Unix(0, 0),
			IntervalEnd:           time.Unix(1, 0),
			Durations:             map[model.ThreadState]time.Duration{model.StateRunning: time.Second},
			TotalObserved:         time.Second,
			SampleCount:           101,
			OnCPU:                 250 * time.Millisecond,
			RunqueueWait:          50 * time.Millisecond,
			Timeslices:            17,
			SchedstatObserved:     time.Second,
			SchedstatSamplePairs:  100,
			SchedstatMaxSampleGap: 12 * time.Millisecond,
		}},
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
	threads, ok := decoded["threads"].([]any)
	if !ok || len(threads) != 1 {
		t.Fatalf("threads missing or wrong type: %#v", decoded["threads"])
	}
	thread := threads[0].(map[string]any)
	if thread["start_time_ticks"] != float64(456) {
		t.Fatalf("start_time_ticks = %v, want 456", thread["start_time_ticks"])
	}
	quality := decoded["quality"].(map[string]any)
	if quality["sampling_method"] != "procfs_midpoint" || quality["missed_transitions_possible"] != true {
		t.Fatalf("quality = %#v, want procfs midpoint warning", quality)
	}
	if quality["schedstat_available"] != true || quality["schedstat_thread_count"] != float64(1) {
		t.Fatalf("schedstat quality = %#v, want available for one thread", quality)
	}
	scheduler := thread["scheduler"].(map[string]any)
	if scheduler["on_cpu_ms"] != float64(250) || scheduler["runqueue_wait_ms"] != float64(50) || scheduler["timeslices"] != float64(17) {
		t.Fatalf("scheduler = %#v, want 250ms CPU, 50ms runqueue, 17 slices", scheduler)
	}
	if scheduler["window_allocation"] != "proportional_by_wall_time" || scheduler["counter_deltas_exact_between_reads"] != true {
		t.Fatalf("scheduler quality = %#v", scheduler)
	}
}
