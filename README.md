# tsastat

[![CI](https://github.com/BogdanDolia/tsastat/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/BogdanDolia/tsastat/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/BogdanDolia/tsastat)](https://github.com/BogdanDolia/tsastat/releases)
[![Go version](https://img.shields.io/github/go-mod/go-version/BogdanDolia/tsastat)](https://go.dev/)
[![License](https://img.shields.io/github/license/BogdanDolia/tsastat)](LICENSE)
[![Platform](https://img.shields.io/badge/platform-Linux-blue)](#requirements)

`tsastat` is a lightweight Linux CLI for per-thread scheduler analysis. Its
eBPF backend builds an event-timed scheduler timeline, while the proc backend
provides a lower-privilege sampling fallback.

```text
tsastat: pid=4242 backend=ebpf interval=1s mode=event-driven

TIME      PID   TID   START_TICKS  START_NS     COMM       RUN_ms  CPU_ms  RQ_ms  SLICES  SCHED_OBS_ms  EVTS  WAKEUPS  INCOMP_WAKE  WAKE_AVG_us  WAKE_MAX_us  LOST_TOTAL  LATE  INIT_RACE  CLK_UNCERT_ns  SLEEP_ms  D_ms  STOP_ms  Z_ms  UNK_ms  OBS_ms  GAP_ms  UNCERT_ms  RUN_%  SLEEP_%  D_%
12:10:01  4242  4243  0            98765500000  worker-1   240     210     30     28      1000          83    12       0            41           115          0           0     0          180            750       10    0        0     0       1000    0       0          24.0   75.0     1.0
```

> [!IMPORTANT]
> The `ebpf` backend is exact only while its scheduler event stream is
> complete. Always inspect the lost-event and late-event quality fields. The
> `proc` backend remains a sampling approximation.

## Why tsastat?

Tools such as `top` and `pidstat` provide broad process statistics. `tsastat`
focuses on one question: **how did each thread of this process spend the
observed interval?**

It is useful for:

- finding unexpectedly busy or permanently sleeping threads;
- spotting threads observed in uninterruptible sleep (`D`);
- separating actual on-CPU time from runnable runqueue wait;
- measuring wakeup-to-switch-in scheduler latency;
- comparing thread behavior before and during a workload;
- exporting interval data as JSON Lines for later analysis;
- learning how Linux exposes task state through procfs.

## Requirements

- Linux with procfs mounted at `/proc`;
- Go 1.22 or newer to build from source;
- permission to read `/proc/<pid>/task` for the target process.

The `proc` backend does not normally require root when inspecting your own
processes.

The `ebpf` backend additionally requires:

- upstream Linux 6.1 or newer for the four-argument `sched_switch`
  `prev_state` tracepoint contract (vendor backports may also work);
- kernel BTF at `/sys/kernel/btf/vmlinux`;
- `CONFIG_BPF`, `CONFIG_BPF_SYSCALL`, `CONFIG_BPF_EVENTS`, and
  `CONFIG_TRACEPOINTS`;
- permission to load tracing BPF programs, normally root or an appropriate
  `CAP_BPF`/`CAP_PERFMON` configuration.

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

Capture an event-timed scheduler timeline:

```bash
sudo tsastat -p 1234 --backend ebpf --interval 1s --count 10
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
| `--sample` | Time between procfs samples; ignored by `ebpf` | `10ms` |
| `-c`, `--count` | Number of reports; omitted means continuous | continuous |
| `--tid` | Show only one thread ID | all threads |
| `--comm` | Filter thread names by substring or glob | no filter |
| `--sort` | Sort by `tid`, `comm`, `running`, `sleeping`, `uninterruptible`, `on_cpu`, `runnable`, `wakeup_latency`, or `total` | `tid` |
| `-o`, `--output` | Output format: `table` or `json` | `table` |
| `--show-idle` | Include threads observed only in the idle state | disabled |
| `--no-header` | Suppress table headers | disabled |

For `proc`, the sampling interval must be shorter than the report interval.
Shorter sampling intervals can capture more transitions, but they also add
overhead. The `ebpf` backend has no sampling cadence.

## JSON Lines output

Use JSON Lines when piping data into tools such as `jq` or saving it for later:

```bash
sudo tsastat -p 1234 --backend ebpf --interval 1s --count 10 \
  --output json > thread-states.jsonl
```

Each line represents one reporting interval:

```json
{"timestamp":"2026-05-08T12:01:01Z","interval_start":"2026-05-08T12:01:00Z","interval_end":"2026-05-08T12:01:01Z","pid":1234,"backend":"ebpf","interval_ms":1000,"sample_interval_ms":0,"quality":{"sampling_method":"ebpf_sched_events","snapshot_count":0,"max_scan_duration_ms":0,"max_sample_gap_ms":0,"missed_transitions_possible":false,"schedstat_available":false,"schedstat_thread_count":0,"schedstat_counter_resets":0,"scheduler_event_timeline":true,"scheduler_event_count":83,"scheduler_lost_events_total":0,"scheduler_late_events":0,"scheduler_incomplete_wakeups":0,"initialization_races":0,"clock_calibration_uncertainty_ns":180},"threads":[{"tid":1235,"start_time_ticks":0,"start_time_ns":98765500000,"comm":"worker-1","durations_ms":{"running":240,"sleeping":750,"uninterruptible":10,"stopped":0,"tracing_stop":0,"zombie":0,"dead":0,"idle":0,"unknown":0},"percent":{"running":24,"sleeping":75,"uninterruptible":1,"unknown":0},"quality":{"tracked_ms":1000,"samples":0,"max_sample_gap_ms":0,"detected_transitions":0,"detected_transition_uncertainty_ms":0},"scheduler":{"available":true,"source":"ebpf_sched_events","counter_deltas_exact_between_reads":false,"event_timed":true,"window_allocation":"exact_event_timestamps","on_cpu_ms":210,"runqueue_wait_ms":30,"timeslices":28,"observed_ms":1000,"on_cpu_percent":21,"runqueue_wait_percent":3,"sample_pairs":0,"max_sample_gap_ms":0,"counter_resets":0,"event_count":83,"wakeup_count":12,"wakeup_latency_total_us":492,"wakeup_latency_avg_us":41,"wakeup_latency_max_us":115,"incomplete_wakeup_count":0}}]}
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
| `proc` | Available | Sampled state plus schedstat scheduler-counter deltas when available |
| `taskstats` | Planned | Linux Delay Accounting counters |
| `ebpf` | Available on supported Linux kernels | Event-timed on-CPU, runnable, sleep, D-state, and wakeup latency |

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
```

Backends with different semantics are not treated as interchangeable ground
truth.

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
- experimental taskstats support;
- optional TUI frontend.

## License

Licensed under the [Apache License 2.0](LICENSE).
