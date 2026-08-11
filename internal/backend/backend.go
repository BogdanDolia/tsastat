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

// HybridBackend intentionally provides both an event timeline and snapshots.
// The collector combines them when events are available and falls back to
// snapshots when the event source cannot be opened.
type HybridBackend interface {
	SchedulerEventBackend
	SnapshotBackend
}

type SourceStatusBackend interface {
	Backend
	SourceStatus() model.BackendSourceStatus
}
