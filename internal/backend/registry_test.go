package backend

import (
	"errors"
	"runtime"
	"testing"
)

func TestRegistry(t *testing.T) {
	t.Run("auto", func(t *testing.T) {
		got, err := New("auto")
		if err != nil {
			t.Fatalf("New(auto) returned error: %v", err)
		}
		capabilities := got.Capabilities()
		if got.Name() != "auto" || !capabilities.SupportsSchedulerEvents || !capabilities.SupportsDelayCounters {
			t.Fatalf("auto backend = %#v, capabilities = %#v", got, capabilities)
		}
	})

	t.Run("hybrid", func(t *testing.T) {
		got, err := New("hybrid")
		if err != nil {
			t.Fatalf("New(hybrid) returned error: %v", err)
		}
		if got.Name() != "hybrid" {
			t.Fatalf("Name() = %q, want hybrid", got.Name())
		}
	})

	t.Run("proc", func(t *testing.T) {
		got, err := New("proc")
		if err != nil {
			t.Fatalf("New(proc) returned error: %v", err)
		}
		if got.Name() != "proc" {
			t.Fatalf("Name() = %q, want proc", got.Name())
		}
	})

	t.Run("taskstats", func(t *testing.T) {
		got, err := New("taskstats")
		if runtime.GOOS != "linux" {
			if err == nil {
				t.Fatal("New(taskstats) returned nil error outside Linux")
			}
			return
		}
		if err != nil {
			t.Fatalf("New(taskstats) returned error: %v", err)
		}
		if got.Name() != "taskstats" || !got.Capabilities().SupportsDelayCounters {
			t.Fatalf("taskstats backend = %#v", got)
		}
	})

	t.Run("ebpf", func(t *testing.T) {
		got, err := New("ebpf")
		if err != nil {
			t.Fatalf("New(ebpf) returned error: %v", err)
		}
		if got.Name() != "ebpf" || !got.Capabilities().SupportsSchedulerEvents {
			t.Fatalf("ebpf backend = %#v", got)
		}
	})

	t.Run("unknown", func(t *testing.T) {
		_, err := New("unknown")
		if !errors.Is(err, ErrUnsupported) {
			t.Fatalf("New(unknown) error = %v, want ErrUnsupported", err)
		}
	})
}
