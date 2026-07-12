package model

type BackendCapabilities struct {
	SupportsThreadStates    bool
	SupportsDelayCounters   bool
	SupportsSchedulerEvents bool
	RequiresRoot            bool
	RequiresKernelConfig    []string
	Accuracy                string
	Warnings                []string
}
