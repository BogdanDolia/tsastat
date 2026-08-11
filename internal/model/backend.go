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

// BackendSourceStatus describes which concrete data sources an adaptive
// backend currently uses and which sources it had to disable.
type BackendSourceStatus struct {
	Active      []string
	Unavailable []string
}
