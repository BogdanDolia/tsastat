package ebpf

import "github.com/BogdanDolia/tsastat/internal/model"

func Capabilities() model.BackendCapabilities {
	return model.BackendCapabilities{
		SupportsThreadStates:    true,
		SupportsDelayCounters:   false,
		SupportsSchedulerEvents: true,
		RequiresRoot:            true,
		Accuracy:                "future event-driven scheduler timeline",
		Warnings: []string{
			"eBPF backend is not implemented yet",
			"will require scheduler tracepoints and appropriate privileges",
		},
	}
}
