package backend

import (
	"context"

	"github.com/BogdanDolia/tsastat/internal/model"
)

type Backend interface {
	Name() string
	Capabilities() model.BackendCapabilities
	Close() error
}

type SnapshotBackend interface {
	Backend
	Snapshot(ctx context.Context, pid int) (model.ThreadSnapshot, error)
}

type SchedulerEventBackend interface {
	Backend
	OpenSchedulerEvents(ctx context.Context, pid int) (model.SchedulerEventStream, error)
}
