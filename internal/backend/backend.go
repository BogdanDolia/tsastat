package backend

import (
	"context"

	"github.com/BogdanDolia/tsastat/internal/model"
)

type Backend interface {
	Name() string
	Capabilities() model.BackendCapabilities
	Snapshot(ctx context.Context, pid int) (model.ThreadSnapshot, error)
	Close() error
}
