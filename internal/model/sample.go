package model

import "time"

type ThreadIdentity struct {
	TID                  int
	StartTimeTicks       uint64
	StartTimeNanoseconds uint64
}

type ThreadSample struct {
	PID            int
	TID            int
	StartTimeTicks uint64
	// StartTimeNanoseconds is the kernel task start_time used by the eBPF
	// backend as a stable identity. Proc snapshots leave it at zero.
	StartTimeNanoseconds uint64
	Comm                 string
	State                ThreadState
	Timestamp            time.Time
	Schedstat            SchedstatCounters
	Delays               DelayCounters
}

func (s ThreadSample) Identity() ThreadIdentity {
	return ThreadIdentity{
		TID:                  s.TID,
		StartTimeTicks:       s.StartTimeTicks,
		StartTimeNanoseconds: s.StartTimeNanoseconds,
	}
}

type ThreadSnapshot struct {
	Samples        []ThreadSample
	StartedAt      time.Time
	FinishedAt     time.Time
	SamplingMethod string
}

type SchedstatCounters struct {
	Available           bool
	OnCPUNanoseconds    uint64
	RunqueueNanoseconds uint64
	Timeslices          uint64
}

type DelayCounter struct {
	Available        bool
	Count            uint64
	TotalNanoseconds uint64
}

type DelayCounters struct {
	Available              bool
	Version                uint16
	AccountingEnabled      bool
	AccountingEnabledKnown bool
	CPU                    DelayCounter
	BlockIO                DelayCounter
	SwapIn                 DelayCounter
	Reclaim                DelayCounter
	Thrashing              DelayCounter
	Compaction             DelayCounter
	WriteProtectCopy       DelayCounter
	IRQ                    DelayCounter
}
