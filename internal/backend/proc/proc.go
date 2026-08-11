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
	root     string
	readFile func(string) ([]byte, error)
}

func New() *Backend {
	return NewWithRoot("/proc")
}

func NewWithRoot(root string) *Backend {
	return &Backend{root: root, readFile: os.ReadFile}
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

		sample, stable, err := b.readStableTask(pid, filepath.Join(taskDir, entry.Name()))
		if err != nil {
			return model.ThreadSnapshot{}, err
		}
		if stable {
			samples = append(samples, sample)
		}
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

func (b *Backend) readStableTask(pid int, taskPath string) (model.ThreadSample, bool, error) {
	const attempts = 2
	statPath := filepath.Join(taskPath, "stat")
	schedstatPath := filepath.Join(taskPath, "schedstat")
	for attempt := 0; attempt < attempts; attempt++ {
		readStartedAt := time.Now()
		before, exists, err := b.readTaskStat(statPath)
		if err != nil {
			return model.ThreadSample{}, false, err
		}
		if !exists {
			continue
		}

		schedstat, err := b.readSchedstat(schedstatPath)
		if err != nil {
			return model.ThreadSample{}, false, err
		}
		after, exists, err := b.readTaskStat(statPath)
		if err != nil {
			return model.ThreadSample{}, false, err
		}
		readFinishedAt := time.Now()
		if !exists || before.TID != after.TID || before.StartTimeTicks != after.StartTimeTicks {
			continue
		}

		return model.ThreadSample{
			PID:            pid,
			TID:            after.TID,
			StartTimeTicks: after.StartTimeTicks,
			Comm:           after.Comm,
			State:          model.StateFromProc(after.State),
			Timestamp:      midpoint(readStartedAt, readFinishedAt),
			Schedstat:      schedstat,
		}, true, nil
	}
	return model.ThreadSample{}, false, nil
}

func (b *Backend) readTaskStat(path string) (procfs.TaskStat, bool, error) {
	data, err := b.readFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return procfs.TaskStat{}, false, nil
		}
		if os.IsPermission(err) {
			return procfs.TaskStat{}, false, fmt.Errorf("permission denied reading %s: %w", path, err)
		}
		return procfs.TaskStat{}, false, fmt.Errorf("read %s: %w", path, err)
	}
	stat, err := procfs.ParseProcStatLine(string(data))
	if err != nil {
		return procfs.TaskStat{}, false, fmt.Errorf("parse %s: %w", path, err)
	}
	return stat, true, nil
}

func (b *Backend) readSchedstat(path string) (model.SchedstatCounters, error) {
	data, err := b.readFile(path)
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
