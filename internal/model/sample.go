package model

import "time"

type ThreadSample struct {
	PID       int
	TID       int
	Comm      string
	State     ThreadState
	Timestamp time.Time
}
