# tsastat

[![CI](https://github.com/BogdanDolia/tsastat/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/BogdanDolia/tsastat/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/BogdanDolia/tsastat)](https://github.com/BogdanDolia/tsastat/releases)
[![Go version](https://img.shields.io/github/go-mod/go-version/BogdanDolia/tsastat)](https://go.dev/)
[![License](https://img.shields.io/github/license/BogdanDolia/tsastat)](LICENSE)
[![Platform](https://img.shields.io/badge/platform-Linux-blue)](#requirements)

`tsastat` is a lightweight Linux CLI for per-thread scheduler and delay
analysis. Its default `auto` backend combines an event-timed eBPF scheduler
timeline, cumulative taskstats resource delays, and procfs identity/state
data. It degrades automatically when a privileged source is unavailable.

```text
tsastat: pid=4242 backend=auto interval=1s sample=10ms mode=hybrid

TIME      PID   TID   COMM      RUN_ms  CPU_ms  RQ_ms  WAKE_AVG_us  CPU_DLY_ms  IO_DLY_ms  RECLAIM_ms  IRQ_DLY_ms
12:10:01  4242  4243  worker-1  240     210     30     41           -           -          -           -
```

The real table includes identity, quality, all taskstats memory-delay
subtypes, and sampled-state columns; the example selects the most common
fields for readability.

> [!IMPORTANT]
> `auto` and `hybrid` are aliases for the same adaptive backend. Check
> `active_sources`, `unavailable_sources`, and `hybrid_identity_mismatches`
> before interpreting a report. A fallback report is intentionally less
> complete than one produced with all sources.
>
> The `ebpf` backend is exact only while its scheduler event stream is
> complete. Always inspect the lost-event and late-event quality fields. The
> `taskstats` backend has exact cumulative counter deltas but approximate
> placement inside sampling gaps, while `proc` state remains a sampling
> approximation.

## Why tsastat?

Tools such as `top` and `pidstat` provide broad process statistics. `tsastat`
focuses on one question: **how did each thread of this process spend the
observed interval?**

It is useful for:

- finding unexpectedly busy or permanently sleeping threads;
- spotting threads observed in uninterruptible sleep (`D`);
- separating actual on-CPU time from runnable runqueue wait;
- measuring wakeup-to-switch-in scheduler latency;
- attributing delays to CPU contention, synchronous block I/O, swap-in,
  reclaim, thrashing, compaction, write-protect copy, and IRQ/SOFTIRQ;
- comparing thread behavior before and during a workload;
- exporting interval data as JSON Lines for later analysis;
- learning how Linux exposes task state through procfs.

## Requirements

- Linux with procfs mounted at `/proc`;
- Go 1.22 or newer to build from source;
- permission to read `/proc/<pid>/task` for the target process.

The `proc` backend does not normally require root when inspecting your own
processes.

The `taskstats` backend additionally requires:

- `CONFIG_TASKSTATS` and `CONFIG_TASK_DELAY_ACCT`;
- `CAP_NET_ADMIN`, normally provided by root, because the kernel marks the
  TASKSTATS Generic Netlink command as an administrative operation;
- `kernel.task_delayacct=1` before the target task starts for non-CPU resource
  delay counters. CPU runqueue delay can still be populated from scheduler
  information when this runtime switch is off.

The `ebpf` backend additionally requires:

- upstream Linux 6.1 or newer for the four-argument `sched_switch`
  `prev_state` tracepoint contract (vendor backports may also work);
- kernel BTF at `/sys/kernel/btf/vmlinux`;
- `CONFIG_BPF`, `CONFIG_BPF_SYSCALL`, `CONFIG_BPF_EVENTS`, and
  `CONFIG_TRACEPOINTS`;
- permission to load tracing BPF programs, normally root or an appropriate
  `CAP_BPF`/`CAP_PERFMON` configuration.

When `tsastat` itself runs in a container, use the host PID namespace (for
example Docker `--pid=host`) and pass the host-visible PID. The in-kernel eBPF
filter compares `task_struct.tgid`, which is not the remapped PID shown by a
private container PID namespace.

The CO-RE BPF object is embedded in the binary. Clang, bpftool, and kernel
headers are not required at runtime.

## Installation

Download a prebuilt archive from the
[latest release](https://github.com/BogdanDolia/tsastat/releases/latest) when
available, or build the CLI with Go:

```bash
git clone https://github.com/BogdanDolia/tsastat.git
cd tsastat
go build -o tsastat ./cmd/tsastat
```

Optionally install it on your `PATH`:

```bash
sudo install -m 0755 tsastat /usr/local/bin/tsastat
```

## Quick start

Check which backends are available:

```bash
tsastat doctor
```

Automatically combine every usable source (this is the default):

```bash
sudo tsastat -p 1234 --backend auto --sample 10ms \
  --interval 1s --count 10
```

Capture an event-timed scheduler timeline:

```bash
sudo tsastat -p 1234 --backend ebpf --interval 1s --count 10
```

Attribute per-thread resource delays:

```bash
sudo tsastat -p 1234 --backend taskstats --sample 50ms \
  --interval 1s --count 10 --sort block_io_delay
```

Monitor a process for ten one-second intervals while sampling every 10ms:

```bash
tsastat -p 1234 --sample 10ms --interval 1s --count 10
```

Show the most active threads first:

```bash
tsastat -p 1234 --sample 10ms --interval 1s --sort running
```

The report interval can also be passed positionally:

```bash
tsastat -p 1234 1
```

Press `Ctrl-C` to stop continuous monitoring.

## Common options

| Option | Description | Default |
| --- | --- | --- |
| `-p`, `--pid` | Target process ID | required |
| `-i`, `--interval` | Time between aggregated reports | required |
| `-b`, `--backend` | `auto`, `hybrid`, `proc`, `taskstats`, or `ebpf` | `auto` |
| `--sample` | Time between snapshots used by `auto`, `hybrid`, `proc`, and `taskstats`; ignored by explicit `ebpf` | `10ms` |
| `-c`, `--count` | Number of reports; omitted means continuous | continuous |
| `--tid` | Show only one thread ID | all threads |
| `--comm` | Filter thread names by substring or glob | no filter |
| `--sort` | Sort by state, scheduler, or delay metric; delay fields include `cpu_delay`, `block_io_delay`, `swap_delay`, `reclaim_delay`, `thrashing_delay`, `compaction_delay`, `wpcopy_delay`, and `irq_delay` | `tid` |
| `-o`, `--output` | Output format: `table` or `json` | `table` |
| `--show-idle` | Include threads observed only in the idle state | disabled |
| `--no-header` | Suppress table headers | disabled |

For `auto`, `hybrid`, `proc`, and `taskstats`, the sampling interval must be
shorter than the report interval.
Shorter sampling intervals can capture more transitions, but they also add
overhead. The `ebpf` backend has no sampling cadence.

## JSON Lines output

Use JSON Lines when piping data into tools such as `jq` or saving it for later:

```bash
sudo tsastat -p 1234 --backend ebpf --interval 1s --count 10 \
  --output json > thread-states.jsonl
```

Each line represents one reporting interval. Adaptive reports expose their
runtime selection explicitly, for example:

```json
{"backend":"auto","quality":{"active_sources":["ebpf","proc","taskstats"],"unavailable_sources":[],"hybrid_identity_mismatches":0,"sampling_method":"ebpf_sched_events+taskstats_counters","scheduler_event_timeline":true,"taskstats_available":true}}
```

The eBPF-shaped example below focuses on scheduler fields; current output also
includes adaptive/taskstats quality keys and an unavailable `delays` object
for that explicit backend:

```json
{"timestamp":"2026-05-08T12:01:01Z","interval_start":"2026-05-08T12:01:00Z","interval_end":"2026-05-08T12:01:01Z","pid":1234,"backend":"ebpf","interval_ms":1000,"sample_interval_ms":0,"quality":{"sampling_method":"ebpf_sched_events","snapshot_count":0,"max_scan_duration_ms":0,"max_sample_gap_ms":0,"missed_transitions_possible":false,"schedstat_available":false,"schedstat_thread_count":0,"schedstat_counter_resets":0,"scheduler_event_timeline":true,"scheduler_event_count":83,"scheduler_lost_events_total":0,"scheduler_late_events":0,"scheduler_incomplete_wakeups":0,"initialization_races":0,"clock_calibration_uncertainty_ns":180},"threads":[{"tid":1235,"start_time_ticks":0,"start_time_ns":98765500000,"comm":"worker-1","durations_ms":{"running":240,"sleeping":750,"uninterruptible":10,"stopped":0,"tracing_stop":0,"zombie":0,"dead":0,"idle":0,"unknown":0},"percent":{"running":24,"sleeping":75,"uninterruptible":1,"unknown":0},"quality":{"tracked_ms":1000,"samples":0,"max_sample_gap_ms":0,"detected_transitions":0,"detected_transition_uncertainty_ms":0},"scheduler":{"available":true,"source":"ebpf_sched_events","counter_deltas_exact_between_reads":false,"event_timed":true,"window_allocation":"exact_event_timestamps","on_cpu_ms":210,"runqueue_wait_ms":30,"timeslices":28,"observed_ms":1000,"on_cpu_percent":21,"runqueue_wait_percent":3,"sample_pairs":0,"max_sample_gap_ms":0,"counter_resets":0,"event_count":83,"wakeup_count":12,"wakeup_latency_total_us":492,"wakeup_latency_avg_us":41,"wakeup_latency_max_us":115,"incomplete_wakeup_count":0}}]}
```

For `taskstats`, each thread also has a versioned `delays` object. Durations
are emitted in nanoseconds so sub-millisecond stalls are not truncated:

```json
{"backend":"taskstats","quality":{"sampling_method":"taskstats_counters_procfs_midpoint","taskstats_available":true,"taskstats_thread_count":1,"taskstats_version_min":14,"taskstats_version_max":14,"taskstats_counter_resets":0,"kernel_task_delayacct_enabled":true,"kernel_task_delayacct_enabled_known":true},"threads":[{"tid":1235,"comm":"worker-1","delays":{"available":true,"source":"linux_taskstats_delayacct","version":14,"kernel_task_delayacct_enabled":true,"kernel_task_delayacct_enabled_known":true,"counter_deltas_exact_between_reads":true,"field_pairs_atomic":false,"window_allocation":"proportional_by_wall_time","observed_ms":1000,"sample_pairs":20,"max_sample_gap_ms":52,"counter_resets":0,"cpu":{"available":true,"count":4,"total_ns":73000,"average_ns":18250},"block_io":{"available":true,"count":1,"total_ns":2100000,"average_ns":2100000},"swap_in":{"available":true,"count":0,"total_ns":0,"average_ns":0},"reclaim":{"available":true,"count":0,"total_ns":0,"average_ns":0},"thrashing":{"available":true,"count":0,"total_ns":0,"average_ns":0},"compaction":{"available":true,"count":0,"total_ns":0,"average_ns":0},"write_protect_copy":{"available":true,"count":0,"total_ns":0,"average_ns":0},"irq":{"available":true,"count":2,"total_ns":18000,"average_ns":9000}}}]}
```

For `ebpf`, `CPU_ms` is time between switch-in and switch-out, `RQ_ms` is time
runnable but not on a CPU, and `SLICES` counts switch-ins. `WAKE_AVG_us` and
`WAKE_MAX_us` describe latency from a successful wakeup to the next switch-in.
`SCHED_OBS_ms` is the event-timeline coverage. `scheduler_lost_events_total`,
`scheduler_late_events`, and `incomplete_wakeup_count` must all be considered
when judging completeness.

For `proc`, `CPU_ms`, `RQ_ms`, and `SLICES` come from differences of cumulative
`/proc/<pid>/task/<tid>/schedstat` counters. `SCHED_OBS_ms` is the wall-clock
coverage of valid counter pairs.

`GAP_ms` is the largest actual gap between observations affecting the row.
`UNCERT_ms` is the accumulated per-window timing ambiguity for state changes
detected at adjacent samples. For a transition whose complete observation gap
falls inside one window, this is half of that gap. `UNK_ms` is time that could
not be attributed to a known state, for example around a disappearing thread.

## Linux thread states

| procfs letter | Reported state | Meaning |
| --- | --- | --- |
| `R` | `running` | Running or runnable |
| `S` | `sleeping` | Interruptible sleep |
| `D` | `uninterruptible` | Usually waiting for I/O |
| `T` | `stopped` | Stopped by job control or a signal |
| `t` | `tracing_stop` | Stopped while being traced |
| `Z` | `zombie` | Exited but not yet reaped |
| `X`, `x` | `dead` | Dead task |
| `I` | `idle` | Idle kernel thread |

`R` includes runnable threads waiting for CPU time. It does not prove that a
thread was actively executing for the entire attributed duration.

## Accuracy and limitations

### auto/hybrid backend

`auto` and `hybrid` use the same source policy:

1. attach eBPF and build the scheduler timeline from exact event timestamps;
2. sample taskstats and procfs on `--sample`, merge their cumulative delay and
   schedstat counters by `(tid, starttime)`, and align those counter windows to
   the eBPF report boundaries;
3. if eBPF cannot be opened, continue with combined taskstats/proc snapshots;
4. if taskstats is also unavailable, continue with proc state and schedstat
   only.

When the eBPF timeline is active, proc schedstat values are not added to eBPF
scheduler durations, which prevents double counting. Only taskstats delay
counters are copied into the event-derived thread row. A copy requires a
proven identity match: equal TID plus equal non-zero proc `starttime`, or equal
non-zero kernel start time. A counter series that cannot be matched safely is
omitted and counted in `hybrid_identity_mismatches`; it is never attached by
TID alone.

Taskstats and proc scans are bracketed into one hybrid snapshot. Their counter
deltas remain exact between reads, while placement inside a report window is
proportional to wall time. eBPF scheduler segments retain their exact event
timestamps. `active_sources` and `unavailable_sources` describe the actual
path used for each emitted interval.

### eBPF backend

The embedded CO-RE programs attach to `tp_btf/sched_switch`,
`tp_btf/sched_wakeup`, and `tp_btf/sched_wakeup_new`. Every event is filtered
in-kernel by `task_struct.tgid`, so threads created after startup are included
without a userspace TID polling race.

The event state machine applies these rules:

- switch-in starts `on_cpu` time and one timeslice;
- a preempted or otherwise runnable switch-out starts `runnable` time;
- an interruptible switch-out starts `sleeping` time;
- an uninterruptible switch-out starts `D` time;
- a successful wakeup ends sleep or D-state and starts runnable time;
- wakeup latency ends at the next switch-in and excludes preemption-only
  runqueue waits.

Event timestamps are converted from kernel monotonic time using a bracketed
userspace clock calibration whose uncertainty is reported in nanoseconds.
Segments are split at exact fixed report boundaries. Before emitting a window,
the collector flushes the shared BPF ring buffer to establish a userspace
watermark. A kernel counter records failed ring-buffer reservations.

Limitations are explicit:

- a non-zero lost-event counter means transitions may be missing;
- events decoded after their report window are rejected and counted as late;
- the initial proc scan is not atomic with scheduler events;
- an initial proc `R` state is reported as `unknown` until the first scheduler
  event proves whether the thread is on-CPU or merely runnable;
- D-state means uninterruptible sleep and does not by itself prove I/O wait;
- wakeup latency is scheduler latency, not end-to-end application latency.
- containerized tracing requires the host PID namespace; a namespace-local PID
  does not match the kernel TGID used by the eBPF filter.

### taskstats backend

The taskstats backend resolves the `TASKSTATS` Generic Netlink family and sends
`TASKSTATS_CMD_GET` for every live TID found under `/proc/<pid>/task`. It uses
`TASKSTATS_CMD_ATTR_PID`, not the process-wide TGID aggregate, so each output
row remains a per-thread measurement. Proc `starttime` is retained as the
stable identity and proc state is sampled at the midpoint of the combined
stat/taskstats read.

The proc stat record is read again after each Netlink reply. If `(tid,
starttime)` changed, the sample is retried and never attached to the stale
identity.

Successive cumulative counter differences report these delay reasons:

- `cpu`: runnable time waiting for a CPU;
- `block_io`: waiting for synchronous block I/O completion; I/O submission
  delay is not included;
- `swap_in`: waiting for page-fault swap-in I/O;
- `reclaim`: direct memory reclaim (`freepages` in the kernel UAPI);
- `thrashing`: page-cache thrashing delay;
- `compaction`: direct memory compaction delay;
- `write_protect_copy`: write-protect copy delay;
- `irq`: IRQ and softirq time charged to the task.

The decoder checks both the taskstats version and returned payload length.
Every reason has its own `available` flag, so an older kernel does not silently
turn an unsupported counter into a real zero. Counter decreases are rejected
per field and counted in `taskstats_counter_resets`; valid reasons from the
same pair are still retained.

Taskstats version 15 used an incompatible field layout before version 16
restored the append-only UAPI. The backend rejects version 15 rather than
silently decoding shifted fields. Older versions retain accurate partial
results through per-field availability; all P3 reasons require v14 or v16+.

Kernel totals are cumulative nanoseconds, and their differences are exact
between successful reads. Placement inside a fixed report window is not
event-timed: a pair crossing a boundary is divided proportionally by wall
time, while preserving its total across windows. `max_sample_gap_ms` exposes
that placement uncertainty. The CPU `count` and `delay_total` fields are not
updated atomically, so their independently correct snapshots can produce a
slightly inconsistent per-interval average.

`kernel.task_delayacct` controls allocation of the task structure used by the
non-CPU resource counters. Those counters require the switch to be enabled
before the task starts. CPU wait comes from scheduler information and may
still advance while the switch is off; the JSON contract therefore reports
the switch separately as `kernel_task_delayacct_enabled` rather than treating
all reasons as unavailable.

This backend polls live tasks and does not register for task-exit records.
Very short-lived threads can therefore disappear before their next query.
Sampled proc states have the same missed-transition limitations as the proc
backend, and a complete taskstats scan is not atomic across threads.

### proc backend

The procfs backend reads `/proc/<pid>/task/<tid>/stat` repeatedly. Each read is
timestamped near its midpoint and the complete scan records its start and end.
When adjacent samples have different states, the transition is estimated at
the midpoint between them. Multiple samples are accumulated into fixed report
windows; intervals crossing a boundary are split at the exact boundary.

Threads are identified by `(tid, starttime)`, where `starttime` is field 22 of
the proc stat record. Reuse of a TID therefore does not merge two different
thread lifetimes.

When `/proc/<pid>/task/<tid>/schedstat` is available, each state read is paired
with the kernel's cumulative on-CPU, runqueue-wait, and timeslice counters.
Successive counter differences are preserved exactly. If a counter pair spans
a fixed report boundary, its delta is divided proportionally by the wall time
on each side; for a fully tracked pair, totals across the affected windows
remain equal to the kernel delta. `tsastat doctor` reports schedstat
availability and the runtime `kernel.sched_schedstats` value when exposed by
the kernel. On kernels where scheduler statistics are runtime-disabled, the
file may still exist while its counters remain zero; `doctor` warns when the
runtime switch is visibly disabled.

Consequences of this approach:

- transitions shorter than the sampling interval can be missed;
- accuracy and overhead depend on `--sample`;
- output reports the actual maximum sample gap and proc scan duration;
- schedstat counter deltas are exact between reads, but their placement inside
  a report window is limited by the sample gap;
- detected transition uncertainty is an estimate, not an upper error bound,
  because multiple transitions can occur between equal endpoint states;
- report windows have fixed boundaries but may be emitted slightly late while
  waiting for a complete observation past the boundary;
- disappearing threads are estimated at the gap midpoint and the remainder is
  reported as `unknown`;
- new threads are tracked from their first observation;
- percentages describe sampled state, not exact on-CPU time.

## Backends

| Backend | Status | Semantics |
| --- | --- | --- |
| `auto` / `hybrid` | Default | Combines all usable sources; falls back from eBPF + taskstats + proc, to taskstats + proc, to proc |
| `proc` | Available | Sampled state plus schedstat scheduler-counter deltas when available |
| `taskstats` | Available on supported Linux kernels | Per-TID CPU, block I/O, swap-in, reclaim, thrashing, compaction, write-protect-copy, and IRQ delay-counter deltas |
| `ebpf` | Available on supported Linux kernels | Event-timed on-CPU, runnable, sleep, D-state, and wakeup latency |

The eBPF stream monitors the original process through a pidfd and stops as soon
as that process exits, so a recycled numeric PID cannot silently become the new
target. Scheduler events carry the boot-time identity used by the kernel and
convert it to the same clock-tick identity exposed by `/proc`; this lets hybrid
mode safely attach taskstats counters to threads created after startup. Proc
samples bracket every `schedstat` read with two `stat` reads and discard an
unstable TID identity instead of combining counters from different threads.

Run `tsastat doctor` to see backend availability and relevant kernel warnings.

## Architecture

```text
CLI
  -> app config
  -> collector
  -> backend interface
  -> accumulator
  -> output renderer
```

The backend interface is deliberately small:

```go
type Backend interface {
    Name() string
    Capabilities() model.BackendCapabilities
    Close() error
}

type SnapshotBackend interface {
    Backend
    Snapshot(ctx context.Context, pid int) (model.ThreadSnapshot, error)
}

type SchedulerEventBackend interface {
    Backend
    OpenSchedulerEvents(ctx context.Context, pid int) (model.SchedulerEventStream, error)
}

type HybridBackend interface {
    SchedulerEventBackend
    SnapshotBackend
}
```

The collector recognizes the combined interface, aligns snapshot counter
windows with the event origin, and merges only non-overlapping metric families.

## Development

Format, test, vet, and build before opening a pull request:

```bash
gofmt -w .
go test -race ./...
go vet ./...
go build ./cmd/tsastat
```

GitHub Actions runs these checks on every pull request and every push to
`main`.

## Releases

Pushing a semantic version tag such as `v0.0.2` runs GoReleaser. The release
workflow publishes Linux AMD64 and ARM64 archives, a SHA-256 checksum file, and
build provenance attestations. Release binaries report their version through
`tsastat --version`.

## Roadmap

- process-level summary rows;
- CSV output;
- multi-process and process-tree monitoring;
- optional TUI frontend.

## License

Licensed under the [Apache License 2.0](LICENSE).
