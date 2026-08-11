package model

import "time"

type SchedulerEventKind uint8

const (
	SchedulerEventWakeup SchedulerEventKind = iota + 1
	SchedulerEventSwitchIn
	SchedulerEventSwitchOut
)

type SchedulerState uint8

const (
	SchedulerStateUnknown SchedulerState = iota
	SchedulerStateOnCPU
	SchedulerStateRunnable
	SchedulerStateSleeping
	SchedulerStateUninterruptible
	SchedulerStateStopped
	SchedulerStateTracingStop
	SchedulerStateIdle
	SchedulerStateDead
)

type SchedulerEvent struct {
	Timestamp            time.Time
	PID                  int
	TID                  int
	StartTimeTicks       uint64
	StartTimeNanoseconds uint64
	Comm                 string
	CPU                  int
	Kind                 SchedulerEventKind
	State                SchedulerState
}

type SchedulerEventStream interface {
	InitialSnapshot() ThreadSnapshot
	Events() <-chan SchedulerEvent
	Errors() <-chan error
	TargetExited() <-chan struct{}
	Flush() error
	Flushed() <-chan struct{}
	LostEvents() uint64
	ClockCalibrationUncertainty() time.Duration
	Close() error
}
