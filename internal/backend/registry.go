package backend

import (
	"errors"
	"fmt"

	"tsastat/internal/backend/ebpf"
	"tsastat/internal/backend/proc"
	"tsastat/internal/backend/taskstats"
	"tsastat/internal/model"
)

var (
	ErrNotImplemented = errors.New("backend not implemented")
	ErrUnsupported    = errors.New("unsupported backend")
)

type BackendError struct {
	Name string
	Err  error
}

func (e BackendError) Error() string {
	switch {
	case errors.Is(e.Err, ErrNotImplemented):
		return fmt.Sprintf("backend %q is not implemented yet", e.Name)
	case errors.Is(e.Err, ErrUnsupported):
		return fmt.Sprintf("unsupported backend %q", e.Name)
	default:
		return fmt.Sprintf("backend %q: %v", e.Name, e.Err)
	}
}

func (e BackendError) Unwrap() error {
	return e.Err
}

func New(name string) (Backend, error) {
	switch name {
	case "", "proc":
		return proc.New(), nil
	case "taskstats":
		return nil, BackendError{Name: name, Err: ErrNotImplemented}
	case "ebpf":
		return nil, BackendError{Name: name, Err: ErrNotImplemented}
	default:
		return nil, BackendError{Name: name, Err: ErrUnsupported}
	}
}

func Capabilities(name string) (model.BackendCapabilities, error) {
	switch name {
	case "", "proc":
		return proc.Capabilities(), nil
	case "taskstats":
		return taskstats.Capabilities(), nil
	case "ebpf":
		return ebpf.Capabilities(), nil
	default:
		return model.BackendCapabilities{}, BackendError{Name: name, Err: ErrUnsupported}
	}
}
