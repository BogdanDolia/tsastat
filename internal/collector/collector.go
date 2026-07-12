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
		acc:            NewAccumulator(),
	}
}

func (c *Collector) Run(ctx context.Context, count int, emit func([]model.ThreadIntervalStats) error) error {
	samples, err := c.backend.Snapshot(ctx, c.pid)
	if err != nil {
		return err
	}
	c.acc.Observe(samples)
	windowStart, ok := snapshotTime(samples)
	if !ok {
		windowStart = time.Now()
	}
	nextReport := windowStart.Add(c.interval)
	ticker := time.NewTicker(c.sampleInterval)
	defer ticker.Stop()

	emitted := 0
	for count <= 0 || emitted < count {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}

		samples, err := c.backend.Snapshot(ctx, c.pid)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return nil
			}
			return err
		}

		c.acc.Observe(samples)
		now, ok := snapshotTime(samples)
		if !ok {
			now = time.Now()
		}
		if now.Before(nextReport) {
			continue
		}
		nextReport = now.Add(c.interval)

		stats := c.acc.Flush()
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
