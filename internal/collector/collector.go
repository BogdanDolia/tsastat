package collector

import (
	"context"
	"errors"
	"time"

	"tsastat/internal/backend"
	"tsastat/internal/model"
)

type Collector struct {
	backend  backend.Backend
	pid      int
	interval time.Duration
	acc      *Accumulator
}

func New(b backend.Backend, pid int, interval time.Duration) *Collector {
	return &Collector{
		backend:  b,
		pid:      pid,
		interval: interval,
		acc:      NewAccumulator(),
	}
}

func (c *Collector) Run(ctx context.Context, count int, emit func([]model.ThreadIntervalStats) error) error {
	samples, err := c.backend.Snapshot(ctx, c.pid)
	if err != nil {
		return err
	}
	c.acc.Observe(samples)

	emitted := 0
	for count <= 0 || emitted < count {
		timer := time.NewTimer(c.interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}

		samples, err := c.backend.Snapshot(ctx, c.pid)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return nil
			}
			return err
		}

		stats := c.acc.Observe(samples)
		if len(stats) == 0 {
			continue
		}
		if err := emit(stats); err != nil {
			return err
		}
		emitted++
	}

	return nil
}
