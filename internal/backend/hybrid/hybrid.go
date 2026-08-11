package hybrid

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/BogdanDolia/tsastat/internal/backend/ebpf"
	"github.com/BogdanDolia/tsastat/internal/backend/proc"
	"github.com/BogdanDolia/tsastat/internal/backend/taskstats"
	"github.com/BogdanDolia/tsastat/internal/model"
)

const (
	hybridSamplingMethod = "hybrid_taskstats_procfs_midpoint"
	procSamplingMethod   = "procfs_midpoint"
)

type schedulerSource interface {
	OpenSchedulerEvents(context.Context, int) (model.SchedulerEventStream, error)
	Close() error
}

type snapshotSource interface {
	Snapshot(context.Context, int) (model.ThreadSnapshot, error)
	Close() error
}

// Backend combines scheduler events, taskstats delay counters, and procfs.
// Source failures are isolated so the backend can degrade without discarding
// data from the sources that remain usable.
type Backend struct {
	name      string
	scheduler schedulerSource
	taskstats snapshotSource
	proc      snapshotSource

	mu                sync.Mutex
	taskstatsDisabled bool
	active            map[string]bool
	unavailable       map[string]bool
}

func New(name string) *Backend {
	if name != "hybrid" {
		name = "auto"
	}

	stats, err := taskstats.New()
	var statsSource snapshotSource
	if err == nil {
		statsSource = stats
	}
	b := newWithSources(name, ebpf.New(), statsSource, proc.New())
	if err != nil {
		b.taskstatsDisabled = true
		b.markUnavailable("taskstats")
	}
	return b
}

func newWithSources(name string, scheduler schedulerSource, stats, procSource snapshotSource) *Backend {
	return &Backend{
		name:        name,
		scheduler:   scheduler,
		taskstats:   stats,
		proc:        procSource,
		active:      make(map[string]bool),
		unavailable: make(map[string]bool),
	}
}

func (b *Backend) Name() string {
	return b.name
}

func (b *Backend) Capabilities() model.BackendCapabilities {
	return Capabilities()
}

func Capabilities() model.BackendCapabilities {
	return model.BackendCapabilities{
		SupportsThreadStates:      true,
		SupportsDelayCounters:     true,
		SupportsSchedulerCounters: true,
		SupportsSchedulerEvents:   true,
		RequiresRoot:              false,
		MinimumKernel:             "6.1 for the eBPF event path",
		RequiresKernelConfig: []string{
			"CONFIG_BPF (optional)",
			"CONFIG_TASKSTATS (optional)",
		},
		Accuracy: "event-timed scheduler states plus exact taskstats counter deltas, with automatic procfs fallbacks",
		Warnings: []string{
			"eBPF and taskstats still require their respective kernel features and permissions",
			"when eBPF is unavailable, scheduler states fall back to sampled procfs data and schedstat counters",
			"when taskstats is unavailable, resource delay metrics are omitted",
			"only counters with a proven thread identity match are merged into the event timeline",
		},
	}
}

func (b *Backend) OpenSchedulerEvents(ctx context.Context, pid int) (model.SchedulerEventStream, error) {
	stream, err := b.scheduler.OpenSchedulerEvents(ctx, pid)
	if err != nil {
		b.markUnavailable("ebpf")
		return nil, err
	}
	b.markActive("ebpf")
	// The eBPF stream uses procfs for its initial identity and state scan.
	b.markActive("proc")
	return stream, nil
}

func (b *Backend) Snapshot(ctx context.Context, pid int) (model.ThreadSnapshot, error) {
	if !b.isTaskstatsDisabled() && b.taskstats != nil {
		statsSnapshot, err := b.taskstats.Snapshot(ctx, pid)
		if err == nil {
			procSnapshot, procErr := b.proc.Snapshot(ctx, pid)
			if procErr != nil {
				return model.ThreadSnapshot{}, procErr
			}
			b.markActive("taskstats")
			b.markActive("proc")
			return mergeSnapshots(statsSnapshot, procSnapshot), nil
		}
		if isTargetError(err) {
			return model.ThreadSnapshot{}, err
		}
		b.disableTaskstats()
	}

	snapshot, err := b.proc.Snapshot(ctx, pid)
	if err != nil {
		return model.ThreadSnapshot{}, err
	}
	b.markActive("proc")
	snapshot.SamplingMethod = procSamplingMethod
	return snapshot, nil
}

func mergeSnapshots(statsSnapshot, procSnapshot model.ThreadSnapshot) model.ThreadSnapshot {
	statsByIdentity := make(map[model.ThreadIdentity]model.ThreadSample, len(statsSnapshot.Samples))
	for _, sample := range statsSnapshot.Samples {
		statsByIdentity[sample.Identity()] = sample
	}

	merged := make([]model.ThreadSample, 0, len(procSnapshot.Samples)+len(statsByIdentity))
	for _, sample := range procSnapshot.Samples {
		statsSample, ok := statsByIdentity[sample.Identity()]
		if ok {
			sample.Delays = statsSample.Delays
			sample.Timestamp = midpointTimestamp(statsSample.Timestamp, sample.Timestamp)
			delete(statsByIdentity, sample.Identity())
		}
		merged = append(merged, sample)
	}
	// Preserve a taskstats sample when a short-lived thread disappears between
	// the two scans. Its stable identity prevents it from being confused with a
	// reused TID, and the missing schedstat fields remain explicitly unavailable.
	for _, sample := range statsByIdentity {
		merged = append(merged, sample)
	}
	sort.Slice(merged, func(i, j int) bool {
		if merged[i].TID != merged[j].TID {
			return merged[i].TID < merged[j].TID
		}
		return merged[i].StartTimeTicks < merged[j].StartTimeTicks
	})

	return model.ThreadSnapshot{
		Samples:        merged,
		StartedAt:      earlierTimestamp(statsSnapshot.StartedAt, procSnapshot.StartedAt),
		FinishedAt:     laterTimestamp(statsSnapshot.FinishedAt, procSnapshot.FinishedAt),
		SamplingMethod: hybridSamplingMethod,
	}
}

func midpointTimestamp(left, right time.Time) time.Time {
	if left.IsZero() {
		return right
	}
	if right.IsZero() {
		return left
	}
	if right.Before(left) {
		left, right = right, left
	}
	return left.Add(right.Sub(left) / 2)
}

func earlierTimestamp(left, right time.Time) time.Time {
	if left.IsZero() || (!right.IsZero() && right.Before(left)) {
		return right
	}
	return left
}

func laterTimestamp(left, right time.Time) time.Time {
	if right.After(left) {
		return right
	}
	return left
}

func isTargetError(err error) bool {
	var processGone model.ProcessNotFoundError
	return errors.As(err, &processGone) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func (b *Backend) disableTaskstats() {
	b.mu.Lock()
	b.taskstatsDisabled = true
	delete(b.active, "taskstats")
	b.unavailable["taskstats"] = true
	b.mu.Unlock()
	if b.taskstats != nil {
		_ = b.taskstats.Close()
	}
}

func (b *Backend) isTaskstatsDisabled() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.taskstatsDisabled
}

func (b *Backend) markActive(source string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.active[source] = true
	delete(b.unavailable, source)
}

func (b *Backend) markUnavailable(source string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.active, source)
	b.unavailable[source] = true
}

func (b *Backend) SourceStatus() model.BackendSourceStatus {
	b.mu.Lock()
	defer b.mu.Unlock()
	status := model.BackendSourceStatus{
		Active:      make([]string, 0, len(b.active)),
		Unavailable: make([]string, 0, len(b.unavailable)),
	}
	for source := range b.active {
		status.Active = append(status.Active, source)
	}
	for source := range b.unavailable {
		status.Unavailable = append(status.Unavailable, source)
	}
	sort.Strings(status.Active)
	sort.Strings(status.Unavailable)
	return status
}

func (b *Backend) Close() error {
	var closeErrors []error
	if b.scheduler != nil {
		closeErrors = append(closeErrors, b.scheduler.Close())
	}
	if b.taskstats != nil {
		closeErrors = append(closeErrors, b.taskstats.Close())
	}
	if b.proc != nil {
		closeErrors = append(closeErrors, b.proc.Close())
	}
	return errors.Join(closeErrors...)
}
