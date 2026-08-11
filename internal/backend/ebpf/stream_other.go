//go:build !linux

package ebpf

import (
	"context"
	"fmt"
	"runtime"

	"github.com/BogdanDolia/tsastat/internal/model"
)

func (b *Backend) OpenSchedulerEvents(context.Context, int) (model.SchedulerEventStream, error) {
	return nil, fmt.Errorf("ebpf backend requires Linux; current platform is %s/%s", runtime.GOOS, runtime.GOARCH)
}
