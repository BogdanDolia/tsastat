package model

type BackendCapabilities struct {
	SupportsThreadStates      bool
	SupportsDelayCounters     bool
	SupportsSchedulerCounters bool
	SupportsSchedulerEvents   bool
	RequiresRoot              bool
	MinimumKernel             string
	RequiresKernelConfig      []string
	Accuracy                  string
	Warnings                  []string
}
