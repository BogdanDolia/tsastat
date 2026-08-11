package ebpf

import "github.com/BogdanDolia/tsastat/internal/model"

const eventFlagPreempted uint32 = 1

const (
	taskInterruptible   uint64 = 0x0001
	taskUninterruptible uint64 = 0x0002
	taskStopped         uint64 = 0x0004
	taskTraced          uint64 = 0x0008
	exitDead            uint64 = 0x0010
	exitZombie          uint64 = 0x0020
	taskParked          uint64 = 0x0040
	taskDead            uint64 = 0x0080
	taskNoLoad          uint64 = 0x0400
	taskRTLockWait      uint64 = 0x1000
)

type Backend struct{}

func New() *Backend {
	return &Backend{}
}

func (b *Backend) Name() string {
	return "ebpf"
}

func (b *Backend) Capabilities() model.BackendCapabilities {
	return Capabilities()
}

func (b *Backend) Close() error {
	return nil
}

func Capabilities() model.BackendCapabilities {
	return model.BackendCapabilities{
		SupportsThreadStates:      true,
		SupportsDelayCounters:     false,
		SupportsSchedulerCounters: false,
		SupportsSchedulerEvents:   true,
		RequiresRoot:              false,
		MinimumKernel:             "6.1",
		RequiresKernelConfig: []string{
			"CONFIG_BPF",
			"CONFIG_BPF_SYSCALL",
			"CONFIG_BPF_EVENTS",
			"CONFIG_DEBUG_INFO_BTF",
			"CONFIG_TRACEPOINTS",
		},
		Accuracy: "event timestamps from sched_switch and sched_wakeup tracepoints",
		Warnings: []string{
			"requires Linux kernel BTF and permission to load tracing BPF programs",
			"ring-buffer loss makes affected intervals incomplete and is reported explicitly",
			"the initial state scan is reconciled with buffered events but cannot be fully atomic",
		},
	}
}

func stateAfterSwitch(rawState uint64, flags uint32) model.SchedulerState {
	if flags&eventFlagPreempted != 0 || rawState == 0 {
		return model.SchedulerStateRunnable
	}
	if rawState&taskRTLockWait != 0 {
		return model.SchedulerStateUninterruptible
	}
	if rawState&(taskUninterruptible|taskNoLoad) == taskUninterruptible|taskNoLoad {
		return model.SchedulerStateIdle
	}
	if rawState&taskUninterruptible != 0 {
		return model.SchedulerStateUninterruptible
	}
	if rawState&taskInterruptible != 0 || rawState&taskParked != 0 {
		return model.SchedulerStateSleeping
	}
	if rawState&taskStopped != 0 {
		return model.SchedulerStateStopped
	}
	if rawState&taskTraced != 0 {
		return model.SchedulerStateTracingStop
	}
	if rawState&(exitDead|exitZombie|taskDead) != 0 {
		return model.SchedulerStateDead
	}
	return model.SchedulerStateUnknown
}
