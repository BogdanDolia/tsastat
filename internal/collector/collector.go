package collector

import (
	"context"
	"errors"
	"fmt"
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
	if eventBackend, ok := c.backend.(backend.SchedulerEventBackend); ok {
		return c.runSchedulerEvents(ctx, eventBackend, count, emit)
	}
	snapshotBackend, ok := c.backend.(backend.SnapshotBackend)
	if !ok {
		return fmt.Errorf("backend %q provides neither snapshots nor scheduler events", c.backend.Name())
	}
	return c.runSnapshots(ctx, snapshotBackend, count, emit)
}

func (c *Collector) runSnapshots(ctx context.Context, snapshotBackend backend.SnapshotBackend, count int, emit func(model.IntervalReport) error) error {
	snapshot, err := snapshotBackend.Snapshot(ctx, c.pid)
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

		snapshot, err := snapshotBackend.Snapshot(ctx, c.pid)
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

func (c *Collector) runSchedulerEvents(ctx context.Context, eventBackend backend.SchedulerEventBackend, count int, emit func(model.IntervalReport) error) error {
	stream, err := eventBackend.OpenSchedulerEvents(ctx, c.pid)
	if err != nil {
		return err
	}
	defer stream.Close()

	acc := NewEventAccumulator(c.interval, stream.InitialSnapshot())
	acc.SetClockCalibrationUncertainty(stream.ClockCalibrationUncertainty())
	timer := time.NewTimer(durationUntil(acc.NextBoundary()))
	defer timer.Stop()

	emitted := 0
	for count <= 0 || emitted < count {
		select {
		case <-ctx.Done():
			return nil
		case event, ok := <-stream.Events():
			if !ok {
				return fmt.Errorf("ebpf scheduler event stream closed unexpectedly")
			}
			acc.Observe(event)
		case streamErr, ok := <-stream.Errors():
			if !ok {
				return fmt.Errorf("ebpf scheduler error stream closed unexpectedly")
			}
			if streamErr != nil {
				return streamErr
			}
		case <-timer.C:
			boundary := acc.NextBoundary()
			if err := flushSchedulerEvents(ctx, stream, acc); err != nil {
				if errors.Is(err, context.Canceled) {
					return nil
				}
				return err
			}
			for _, report := range acc.Advance(boundary, stream.LostEvents()) {
				if err := emit(report); err != nil {
					return err
				}
				emitted++
				if count > 0 && emitted >= count {
					return nil
				}
			}
			timer.Reset(durationUntil(acc.NextBoundary()))
		}
	}
	return nil
}

func flushSchedulerEvents(ctx context.Context, stream model.SchedulerEventStream, acc *EventAccumulator) error {
	if err := stream.Flush(); err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event, ok := <-stream.Events():
			if !ok {
				return fmt.Errorf("ebpf scheduler event stream closed during flush")
			}
			acc.Observe(event)
		case streamErr, ok := <-stream.Errors():
			if !ok {
				return fmt.Errorf("ebpf scheduler error stream closed during flush")
			}
			if streamErr != nil {
				return streamErr
			}
		case _, ok := <-stream.Flushed():
			if !ok {
				return fmt.Errorf("ebpf scheduler flush stream closed unexpectedly")
			}
			drainSchedulerEvents(acc, stream.Events())
			return nil
		}
	}
}

func drainSchedulerEvents(acc *EventAccumulator, events <-chan model.SchedulerEvent) {
	for {
		select {
		case event, ok := <-events:
			if !ok {
				return
			}
			acc.Observe(event)
		default:
			return
		}
	}
}

func durationUntil(at time.Time) time.Duration {
	delay := time.Until(at)
	if delay < 0 {
		return 0
	}
	return delay
}
