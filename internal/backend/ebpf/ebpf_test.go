package ebpf

import (
	"testing"

	"github.com/BogdanDolia/tsastat/internal/model"
)

func TestStateAfterSwitch(t *testing.T) {
	tests := []struct {
		name  string
		state uint64
		flags uint32
		want  model.SchedulerState
	}{
		{name: "voluntary runnable", want: model.SchedulerStateRunnable},
		{name: "preempted overrides state", state: taskUninterruptible, flags: eventFlagPreempted, want: model.SchedulerStateRunnable},
		{name: "interruptible", state: taskInterruptible, want: model.SchedulerStateSleeping},
		{name: "uninterruptible", state: taskUninterruptible, want: model.SchedulerStateUninterruptible},
		{name: "killable D state", state: taskUninterruptible | 0x100, want: model.SchedulerStateUninterruptible},
		{name: "rt lock wait", state: taskRTLockWait, want: model.SchedulerStateUninterruptible},
		{name: "idle wait", state: taskUninterruptible | taskNoLoad, want: model.SchedulerStateIdle},
		{name: "stopped", state: taskStopped, want: model.SchedulerStateStopped},
		{name: "traced", state: taskTraced, want: model.SchedulerStateTracingStop},
		{name: "zombie", state: exitZombie, want: model.SchedulerStateDead},
		{name: "parked", state: taskParked, want: model.SchedulerStateSleeping},
		{name: "unknown", state: 0x2000, want: model.SchedulerStateUnknown},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := stateAfterSwitch(test.state, test.flags); got != test.want {
				t.Fatalf("stateAfterSwitch(%#x, %#x) = %d, want %d", test.state, test.flags, got, test.want)
			}
		})
	}
}

func TestCapabilitiesAdvertiseEventTimeline(t *testing.T) {
	capabilities := Capabilities()
	if !capabilities.SupportsSchedulerEvents || !capabilities.SupportsThreadStates {
		t.Fatalf("capabilities = %#v", capabilities)
	}
	if capabilities.SupportsSchedulerCounters {
		t.Fatalf("event backend must not claim schedstat counters: %#v", capabilities)
	}
}
