package proc

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestCapabilitiesIncludeSchedulerCounters(t *testing.T) {
	capabilities := Capabilities()
	if !capabilities.SupportsThreadStates || !capabilities.SupportsSchedulerCounters {
		t.Fatalf("capabilities = %#v", capabilities)
	}
	if capabilities.SupportsSchedulerEvents {
		t.Fatalf("proc backend must not claim event-timed scheduler data: %#v", capabilities)
	}
}

func TestSnapshotIncludesStableIdentityAndScanBounds(t *testing.T) {
	root := t.TempDir()
	statDir := filepath.Join(root, "123", "task", "124")
	if err := os.MkdirAll(statDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	line := fmt.Sprintf("124 (worker thread) S 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 %d", 987654)
	if err := os.WriteFile(filepath.Join(statDir, "stat"), []byte(line), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(statDir, "schedstat"), []byte("123456789 987654321 42\n"), 0o644); err != nil {
		t.Fatalf("WriteFile schedstat: %v", err)
	}

	snapshot, err := NewWithRoot(root).Snapshot(context.Background(), 123)
	if err != nil {
		t.Fatalf("Snapshot returned error: %v", err)
	}
	if snapshot.StartedAt.IsZero() || snapshot.FinishedAt.IsZero() || snapshot.FinishedAt.Before(snapshot.StartedAt) {
		t.Fatalf("invalid scan bounds: [%s,%s]", snapshot.StartedAt, snapshot.FinishedAt)
	}
	if len(snapshot.Samples) != 1 {
		t.Fatalf("samples = %d, want 1", len(snapshot.Samples))
	}
	sample := snapshot.Samples[0]
	if sample.TID != 124 || sample.StartTimeTicks != 987654 || sample.Comm != "worker thread" {
		t.Fatalf("sample identity = %#v", sample)
	}
	if !sample.Schedstat.Available || sample.Schedstat.OnCPUNanoseconds != 123456789 ||
		sample.Schedstat.RunqueueNanoseconds != 987654321 || sample.Schedstat.Timeslices != 42 {
		t.Fatalf("schedstat = %#v", sample.Schedstat)
	}
	if sample.Timestamp.Before(snapshot.StartedAt) || sample.Timestamp.After(snapshot.FinishedAt) {
		t.Fatalf("sample timestamp %s outside scan [%s,%s]", sample.Timestamp, snapshot.StartedAt, snapshot.FinishedAt)
	}
}

func TestSnapshotKeepsProcSampleWhenSchedstatIsUnavailable(t *testing.T) {
	root := t.TempDir()
	statDir := filepath.Join(root, "123", "task", "124")
	if err := os.MkdirAll(statDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	line := "124 (worker) S 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 987654"
	if err := os.WriteFile(filepath.Join(statDir, "stat"), []byte(line), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	snapshot, err := NewWithRoot(root).Snapshot(context.Background(), 123)
	if err != nil {
		t.Fatalf("Snapshot returned error: %v", err)
	}
	if len(snapshot.Samples) != 1 || snapshot.Samples[0].Schedstat.Available {
		t.Fatalf("samples = %#v, want proc sample with unavailable schedstat", snapshot.Samples)
	}
}
