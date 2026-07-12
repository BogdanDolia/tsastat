package model

import "fmt"

type ProcessNotFoundError struct {
	PID int
}

func (e ProcessNotFoundError) Error() string {
	return fmt.Sprintf("process %d no longer exists", e.PID)
}
