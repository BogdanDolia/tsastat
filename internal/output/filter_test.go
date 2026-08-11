package output

import (
	"testing"
	"time"

	"github.com/BogdanDolia/tsastat/internal/model"
)

func TestFilterAndSortSchedulerMetrics(t *testing.T) {
	stats := []model.ThreadIntervalStats{
		{TID: 1, OnCPU: 10 * time.Millisecond, RunqueueWait: 30 * time.Millisecond, WakeupLatencyMax: 20 * time.Microsecond},
		{TID: 2, OnCPU: 20 * time.Millisecond, RunqueueWait: 10 * time.Millisecond, WakeupLatencyMax: 40 * time.Microsecond},
	}
	for field, wantTID := range map[string]int{
		"on_cpu":         2,
		"runnable":       1,
		"wakeup_latency": 2,
	} {
		got, err := FilterAndSort(stats, FilterSortOptions{Sort: field})
		if err != nil {
			t.Fatalf("FilterAndSort(%q) returned error: %v", field, err)
		}
		if len(got) != 2 || got[0].TID != wantTID {
			t.Fatalf("FilterAndSort(%q) = %#v, want TID %d first", field, got, wantTID)
		}
	}
}
