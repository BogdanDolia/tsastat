package taskstats

import "tsastat/internal/model"

func Capabilities() model.BackendCapabilities {
	return model.BackendCapabilities{
		SupportsThreadStates:    false,
		SupportsDelayCounters:   true,
		SupportsSchedulerEvents: false,
		RequiresRoot:            true,
		RequiresKernelConfig: []string{
			"CONFIG_TASKSTATS",
			"CONFIG_TASK_DELAY_ACCT",
		},
		Accuracy: "delay accounting counters, not exact live state transitions",
		Warnings: []string{
			"taskstats counters may be lazily updated on modern kernels",
			"delay accounting may be disabled at runtime",
			"tasks created before enabling delay accounting may not expose useful counters",
		},
	}
}
