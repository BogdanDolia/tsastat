//go:build !linux

package taskstats

import (
	"context"
	"fmt"
	"runtime"

	"github.com/BogdanDolia/tsastat/internal/model"
)

func New() (*Backend, error) {
	return nil, fmt.Errorf("taskstats requires Linux; current platform is %s/%s", runtime.GOOS, runtime.GOARCH)
}

func Probe(context.Context) (model.DelayCounters, error) {
	return model.DelayCounters{}, fmt.Errorf("taskstats requires Linux; current platform is %s/%s", runtime.GOOS, runtime.GOARCH)
}
