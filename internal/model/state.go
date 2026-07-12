package model

type ThreadState string

const (
	StateRunning         ThreadState = "running"
	StateSleeping        ThreadState = "sleeping"
	StateUninterruptible ThreadState = "uninterruptible"
	StateStopped         ThreadState = "stopped"
	StateTracingStop     ThreadState = "tracing_stop"
	StateZombie          ThreadState = "zombie"
	StateDead            ThreadState = "dead"
	StateIdle            ThreadState = "idle"
	StateUnknown         ThreadState = "unknown"
)

func StateFromProc(stat byte) ThreadState {
	switch stat {
	case 'R':
		return StateRunning
	case 'S':
		return StateSleeping
	case 'D':
		return StateUninterruptible
	case 'T':
		return StateStopped
	case 't':
		return StateTracingStop
	case 'Z':
		return StateZombie
	case 'X', 'x':
		return StateDead
	case 'I':
		return StateIdle
	default:
		return StateUnknown
	}
}

func KnownStates() []ThreadState {
	return []ThreadState{
		StateRunning,
		StateSleeping,
		StateUninterruptible,
		StateStopped,
		StateTracingStop,
		StateZombie,
		StateDead,
		StateIdle,
		StateUnknown,
	}
}
