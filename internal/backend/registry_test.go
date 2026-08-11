package backend

import (
	"errors"
	"testing"
)

func TestRegistry(t *testing.T) {
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
		_, err := New("taskstats")
		if !errors.Is(err, ErrNotImplemented) {
			t.Fatalf("New(taskstats) error = %v, want ErrNotImplemented", err)
		}
		if got, want := err.Error(), `backend "taskstats" is not implemented yet`; got != want {
			t.Fatalf("error = %q, want %q", got, want)
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
