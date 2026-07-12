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
		SupportsThreadStates:    true,
		SupportsDelayCounters:   false,
		SupportsSchedulerEvents: false,
		RequiresRoot:            false,
		Accuracy:                "sampling approximation",
		Warnings: []string{
			"/proc polling can miss short-lived state transitions",
			"R means running or runnable, not necessarily on-CPU",
			"accuracy depends on sampling interval",
		},
	}
}

func (b *Backend) Snapshot(ctx context.Context, pid int) ([]model.ThreadSample, error) {
	if pid <= 0 {
		return nil, fmt.Errorf("invalid pid %d", pid)
	}

	taskDir := filepath.Join(b.root, strconv.Itoa(pid), "task")
	entries, err := os.ReadDir(taskDir)
	if err != nil {
		if os.IsNotExist(err) {
			if _, rootErr := os.Stat(b.root); os.IsNotExist(rootErr) {
				return nil, fmt.Errorf("%s is not available; tsastat requires Linux procfs", b.root)
			}
			return nil, model.ProcessNotFoundError{PID: pid}
		}
		if os.IsPermission(err) {
			return nil, fmt.Errorf("permission denied reading %s: %w", taskDir, err)
		}
		return nil, fmt.Errorf("read %s: %w", taskDir, err)
	}

	ts := time.Now()
	samples := make([]model.ThreadSample, 0, len(entries))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !entry.IsDir() {
			continue
		}
		if _, err := strconv.Atoi(entry.Name()); err != nil {
			continue
		}

		statPath := filepath.Join(taskDir, entry.Name(), "stat")
		data, err := os.ReadFile(statPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			if os.IsPermission(err) {
				return nil, fmt.Errorf("permission denied reading %s: %w", statPath, err)
			}
			return nil, fmt.Errorf("read %s: %w", statPath, err)
		}

		tid, comm, stateByte, err := procfs.ParseProcStatLine(string(data))
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", statPath, err)
		}
		samples = append(samples, model.ThreadSample{
			PID:       pid,
			TID:       tid,
			Comm:      comm,
			State:     model.StateFromProc(stateByte),
			Timestamp: ts,
		})
	}

	return samples, nil
}

func (b *Backend) Close() error {
	return nil
}
