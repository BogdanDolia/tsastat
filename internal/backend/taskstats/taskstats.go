package taskstats

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/BogdanDolia/tsastat/internal/model"
	"github.com/BogdanDolia/tsastat/internal/procfs"
)

const samplingMethod = "taskstats_counters_procfs_midpoint"

type statsClient interface {
	Stats(ctx context.Context, tid int, accountingEnabled, accountingEnabledKnown bool) (model.DelayCounters, error)
	Close() error
}

type Backend struct {
	root                 string
	openClient           func(context.Context) (statsClient, error)
	delayAccountingState func() (bool, bool)
	mu                   sync.Mutex
	client               statsClient
}

func (b *Backend) Name() string {
	return "taskstats"
}

func (b *Backend) Capabilities() model.BackendCapabilities {
	return Capabilities()
}

func (b *Backend) Snapshot(ctx context.Context, pid int) (model.ThreadSnapshot, error) {
	if pid <= 0 {
		return model.ThreadSnapshot{}, fmt.Errorf("invalid pid %d", pid)
	}
	startedAt := time.Now()
	client, err := b.ensureClient(ctx)
	if err != nil {
		return model.ThreadSnapshot{}, err
	}

	accountingEnabled, accountingEnabledKnown := b.delayAccountingState()
	taskDir := filepath.Join(b.root, strconv.Itoa(pid), "task")
	entries, err := os.ReadDir(taskDir)
	if err != nil {
		return model.ThreadSnapshot{}, snapshotReadError(b.root, taskDir, pid, err)
	}

	samples := make([]model.ThreadSample, 0, len(entries))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return model.ThreadSnapshot{}, err
		}
		if !entry.IsDir() {
			continue
		}
		tid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}

		statPath := filepath.Join(taskDir, entry.Name(), "stat")
		sample, ok, err := readStableThreadSample(ctx, client, pid, tid, statPath, accountingEnabled, accountingEnabledKnown)
		if err != nil {
			return model.ThreadSnapshot{}, err
		}
		if ok {
			samples = append(samples, sample)
		}
	}

	return model.ThreadSnapshot{
		Samples:        samples,
		StartedAt:      startedAt,
		FinishedAt:     time.Now(),
		SamplingMethod: samplingMethod,
	}, nil
}

func readStableThreadSample(
	ctx context.Context,
	client statsClient,
	pid, tid int,
	statPath string,
	accountingEnabled, accountingEnabledKnown bool,
) (model.ThreadSample, bool, error) {
	for attempt := 0; attempt < 2; attempt++ {
		readStartedAt := time.Now()
		before, exists, err := readTaskStat(statPath)
		if err != nil || !exists {
			return model.ThreadSample{}, false, err
		}
		delays, err := client.Stats(ctx, tid, accountingEnabled, accountingEnabledKnown)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return model.ThreadSample{}, false, fmt.Errorf("read taskstats for tid %d: %w", tid, err)
		}
		after, exists, err := readTaskStat(statPath)
		if err != nil {
			return model.ThreadSample{}, false, err
		}
		if !exists {
			continue
		}
		readFinishedAt := time.Now()
		if before.TID != after.TID || before.StartTimeTicks != after.StartTimeTicks {
			continue
		}
		return model.ThreadSample{
			PID:            pid,
			TID:            after.TID,
			StartTimeTicks: after.StartTimeTicks,
			Comm:           after.Comm,
			State:          model.StateFromProc(after.State),
			Timestamp:      readStartedAt.Add(readFinishedAt.Sub(readStartedAt) / 2),
			Delays:         delays,
		}, true, nil
	}
	return model.ThreadSample{}, false, nil
}

func readTaskStat(path string) (procfs.TaskStat, bool, error) {
	data, err := os.ReadFile(path)
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

func (b *Backend) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.client == nil {
		return nil
	}
	err := b.client.Close()
	b.client = nil
	return err
}

func (b *Backend) ensureClient(ctx context.Context) (statsClient, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.client != nil {
		return b.client, nil
	}
	client, err := b.openClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("open TASKSTATS Generic Netlink client: %w", err)
	}
	b.client = client
	return client, nil
}

func newWithClient(root string, client statsClient, accountingEnabled, accountingEnabledKnown bool) *Backend {
	return &Backend{
		root:                 root,
		openClient:           func(context.Context) (statsClient, error) { return client, nil },
		delayAccountingState: func() (bool, bool) { return accountingEnabled, accountingEnabledKnown },
		client:               client,
	}
}

func snapshotReadError(root, taskDir string, pid int, err error) error {
	if os.IsNotExist(err) {
		if _, rootErr := os.Stat(root); os.IsNotExist(rootErr) {
			return fmt.Errorf("%s is not available; taskstats requires Linux procfs", root)
		}
		return model.ProcessNotFoundError{PID: pid}
	}
	if os.IsPermission(err) {
		return fmt.Errorf("permission denied reading %s: %w", taskDir, err)
	}
	return fmt.Errorf("read %s: %w", taskDir, err)
}

func Capabilities() model.BackendCapabilities {
	return model.BackendCapabilities{
		SupportsThreadStates:      true,
		SupportsDelayCounters:     true,
		SupportsSchedulerCounters: false,
		SupportsSchedulerEvents:   false,
		RequiresRoot:              true,
		RequiresKernelConfig: []string{
			"CONFIG_TASKSTATS",
			"CONFIG_TASK_DELAY_ACCT",
		},
		Accuracy: "exact cumulative delay counter deltas between reads with sampled proc states and proportional window allocation",
		Warnings: []string{
			"taskstats counters may be lazily updated on modern kernels",
			"the cpu count and total fields are not updated atomically",
			"non-CPU resource delay accounting may be disabled at runtime",
			"tasks created before enabling delay accounting may not expose useful non-CPU counters",
			"counter deltas crossing report boundaries are allocated proportionally by wall time",
		},
	}
}
