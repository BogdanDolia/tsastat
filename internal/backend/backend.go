package backend

import (
	"context"

	"tsastat/internal/model"
)

type Backend interface {
	Name() string
	Capabilities() model.BackendCapabilities
	Snapshot(ctx context.Context, pid int) ([]model.ThreadSample, error)
	Close() error
}
