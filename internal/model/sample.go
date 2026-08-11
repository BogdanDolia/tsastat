package model

import "time"

type ThreadIdentity struct {
	TID            int
	StartTimeTicks uint64
}

type ThreadSample struct {
	PID            int
	TID            int
	StartTimeTicks uint64
	Comm           string
	State          ThreadState
	Timestamp      time.Time
	Schedstat      SchedstatCounters
}

func (s ThreadSample) Identity() ThreadIdentity {
	return ThreadIdentity{
		TID:            s.TID,
		StartTimeTicks: s.StartTimeTicks,
	}
}

type ThreadSnapshot struct {
	Samples    []ThreadSample
	StartedAt  time.Time
	FinishedAt time.Time
}

type SchedstatCounters struct {
	Available           bool
	OnCPUNanoseconds    uint64
	RunqueueNanoseconds uint64
	Timeslices          uint64
}
