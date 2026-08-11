package collector

import (
	"context"
	"errors"
	"time"

	"github.com/BogdanDolia/tsastat/internal/backend"
	"github.com/BogdanDolia/tsastat/internal/model"
)

type Collector struct {
	backend        backend.Backend
	pid            int
	interval       time.Duration
	sampleInterval time.Duration
	acc            *Accumulator
}

func New(b backend.Backend, pid int, interval, sampleInterval time.Duration) *Collector {
	return &Collector{
		backend:        b,
		pid:            pid,
		interval:       interval,
		sampleInterval: sampleInterval,
		acc:            NewAccumulator(interval),
	}
}

func (c *Collector) Run(ctx context.Context, count int, emit func(model.IntervalReport) error) error {
	snapshot, err := c.backend.Snapshot(ctx, c.pid)
	if err != nil {
		return err
	}
	c.acc.Observe(snapshot)
	ticker := time.NewTicker(c.sampleInterval)
	defer ticker.Stop()

	emitted := 0
	for count <= 0 || emitted < count {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}

		snapshot, err := c.backend.Snapshot(ctx, c.pid)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return nil
			}
			return err
		}

		for _, report := range c.acc.Observe(snapshot) {
			if err := emit(report); err != nil {
				return err
			}
			emitted++
			if count > 0 && emitted >= count {
				return nil
			}
		}
	}

	return nil
}
