//go:build linux

package taskstats

import (
	"context"
	"os"
	"strings"

	"github.com/BogdanDolia/tsastat/internal/model"
)

func New() (*Backend, error) {
	return &Backend{
		root:                 "/proc",
		openClient:           newNetlinkClient,
		delayAccountingState: readDelayAccountingState,
	}, nil
}

func Probe(ctx context.Context) (model.DelayCounters, error) {
	client, err := newNetlinkClient(ctx)
	if err != nil {
		return model.DelayCounters{}, err
	}
	defer client.Close()
	enabled, known := readDelayAccountingState()
	return client.Stats(ctx, os.Getpid(), enabled, known)
}

func readDelayAccountingState() (bool, bool) {
	data, err := os.ReadFile("/proc/sys/kernel/task_delayacct")
	if err != nil {
		return false, false
	}
	switch strings.TrimSpace(string(data)) {
	case "1":
		return true, true
	case "0":
		return false, true
	default:
		return false, false
	}
}
