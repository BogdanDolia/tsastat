package proc

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/BogdanDolia/tsastat/internal/model"
	"github.com/BogdanDolia/tsastat/internal/procfs"
)

type Backend struct {
	root string
}

func New() *Backend {
	return NewWithRoot("/proc")
}

func NewWithRoot(root string) *Backend {
	return &Backend{root: root}
}

func (b *Backend) Name() string {
	return "proc"
}

func (b *Backend) Capabilities() model.BackendCapabilities {
	return Capabilities()
}

func Capabilities() model.BackendCapabilities {
	return model.BackendCapabilities{
		SupportsThreadStates:      true,
		SupportsDelayCounters:     false,
		SupportsSchedulerCounters: true,
		SupportsSchedulerEvents:   false,
		RequiresRoot:              false,
		Accuracy:                  "sampled states with exact schedstat counter deltas between reads",
		Warnings: []string{
			"/proc polling can miss short-lived state transitions",
			"R means running or runnable, not necessarily on-CPU",
			"accuracy depends on sampling interval",
			"schedstat deltas crossing report boundaries are allocated proportionally by wall time",
		},
	}
}

func (b *Backend) Snapshot(ctx context.Context, pid int) (model.ThreadSnapshot, error) {
	if pid <= 0 {
		return model.ThreadSnapshot{}, fmt.Errorf("invalid pid %d", pid)
	}

	startedAt := time.Now()
	taskDir := filepath.Join(b.root, strconv.Itoa(pid), "task")
	entries, err := os.ReadDir(taskDir)
	if err != nil {
		if os.IsNotExist(err) {
			if _, rootErr := os.Stat(b.root); os.IsNotExist(rootErr) {
				return model.ThreadSnapshot{}, fmt.Errorf("%s is not available; tsastat requires Linux procfs", b.root)
			}
			return model.ThreadSnapshot{}, model.ProcessNotFoundError{PID: pid}
		}
		if os.IsPermission(err) {
			return model.ThreadSnapshot{}, fmt.Errorf("permission denied reading %s: %w", taskDir, err)
		}
		return model.ThreadSnapshot{}, fmt.Errorf("read %s: %w", taskDir, err)
	}

	samples := make([]model.ThreadSample, 0, len(entries))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return model.ThreadSnapshot{}, err
		}
		if !entry.IsDir() {
			continue
		}
		if _, err := strconv.Atoi(entry.Name()); err != nil {
			continue
		}

		statPath := filepath.Join(taskDir, entry.Name(), "stat")
		readStartedAt := time.Now()
		data, err := os.ReadFile(statPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			if os.IsPermission(err) {
				return model.ThreadSnapshot{}, fmt.Errorf("permission denied reading %s: %w", statPath, err)
			}
			return model.ThreadSnapshot{}, fmt.Errorf("read %s: %w", statPath, err)
		}

		stat, err := procfs.ParseProcStatLine(string(data))
		if err != nil {
			return model.ThreadSnapshot{}, fmt.Errorf("parse %s: %w", statPath, err)
		}

		schedstat, err := b.readSchedstat(filepath.Join(taskDir, entry.Name(), "schedstat"))
		if err != nil {
			return model.ThreadSnapshot{}, err
		}
		readFinishedAt := time.Now()
		samples = append(samples, model.ThreadSample{
			PID:            pid,
			TID:            stat.TID,
			StartTimeTicks: stat.StartTimeTicks,
			Comm:           stat.Comm,
			State:          model.StateFromProc(stat.State),
			Timestamp:      midpoint(readStartedAt, readFinishedAt),
			Schedstat:      schedstat,
		})
	}

	return model.ThreadSnapshot{
		Samples:    samples,
		StartedAt:  startedAt,
		FinishedAt: time.Now(),
	}, nil
}

func (b *Backend) Close() error {
	return nil
}

func midpoint(start, end time.Time) time.Time {
	return start.Add(end.Sub(start) / 2)
}

func (b *Backend) readSchedstat(path string) (model.SchedstatCounters, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) || os.IsPermission(err) {
			return model.SchedstatCounters{}, nil
		}
		return model.SchedstatCounters{}, fmt.Errorf("read %s: %w", path, err)
	}

	stat, err := procfs.ParseSchedstatLine(string(data))
	if err != nil {
		return model.SchedstatCounters{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return model.SchedstatCounters{
		Available:           true,
		OnCPUNanoseconds:    stat.OnCPUNanoseconds,
		RunqueueNanoseconds: stat.RunqueueNanoseconds,
		Timeslices:          stat.Timeslices,
	}, nil
}
