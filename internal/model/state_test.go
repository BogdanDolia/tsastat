package model

import "testing"

func TestStateFromProc(t *testing.T) {
	tests := []struct {
		input byte
		want  ThreadState
	}{
		{'R', StateRunning},
		{'S', StateSleeping},
		{'D', StateUninterruptible},
		{'T', StateStopped},
		{'t', StateTracingStop},
		{'Z', StateZombie},
		{'X', StateDead},
		{'x', StateDead},
		{'I', StateIdle},
		{'?', StateUnknown},
	}

	for _, tt := range tests {
		if got := StateFromProc(tt.input); got != tt.want {
			t.Fatalf("StateFromProc(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
