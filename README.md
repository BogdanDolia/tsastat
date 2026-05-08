# tsastat

`tsastat` is a Linux-only CLI for sampled thread state analysis. It behaves
similarly to `pidstat(1)`: point it at a process, choose an interval, and it
prints rolling per-thread state statistics.

The MVP uses `/proc/<pid>/task/<tid>/stat` polling. The proc backend is a
sampling approximation. It can miss short-lived state transitions between
samples.

## Build

```bash
go build ./cmd/tsastat
go test ./...
```

## Examples

```bash
tsastat -p 1234 1
tsastat -p 1234 -i 1s --count 5
tsastat -p 1234 -i 500ms --sort running
tsastat -p 1234 -i 1s --output json > thread-states.jsonl
```

Future backends are visible but not implemented:

```bash
tsastat -p 1234 -i 1s --backend taskstats
tsastat -p 1234 -i 1s --backend ebpf
```

Both return clear errors until those backends exist.

## Table Output

```text
tsastat: pid=4242 backend=proc interval=1s

TIME      PID   TID   COMM          RUN_ms  SLEEP_ms  D_ms  STOP_ms  Z_ms  RUN_%  SLEEP_%  D_%
12:10:01  4242  4242  nginx         0       1000      0     0        0     0.0    100.0    0.0
12:10:01  4242  4243  nginx-worker  42      958       0     0        0     4.2    95.8     0.0
12:10:01  4242  4244  nginx-worker  130     870       0     0        0     13.0   87.0     0.0
```

Use `--no-header` to suppress table headers for scripts.

## JSON Output

`--output json` prints JSON Lines, one object per interval:

```json
{"timestamp":"2026-05-08T12:01:01Z","pid":1234,"backend":"proc","interval_ms":1000,"threads":[{"tid":1235,"comm":"java-worker-1","durations_ms":{"running":120,"sleeping":870,"uninterruptible":10,"stopped":0,"tracing_stop":0,"zombie":0},"percent":{"running":12,"sleeping":87,"uninterruptible":1}}]}
```

This format is intended for tools such as `jq`.

## Linux Thread States

The proc backend maps Linux state letters to normalized states:

| Letter | State |
| --- | --- |
| `R` | `running` |
| `S` | `sleeping` |
| `D` | `uninterruptible` |
| `T` | `stopped` |
| `t` | `tracing_stop` |
| `Z` | `zombie` |
| `X`, `x` | `dead` |
| `I` | `idle` |
| other | `unknown` |

`R` means running or runnable. It does not necessarily mean the thread was
actively on CPU for the whole interval.

## Polling Limitations

The proc backend samples the state observed at each interval and attributes the
time until the next sample to that previously observed state. This is simple and
automatable, but it is not a scheduler timeline.

Limitations:

- short-lived transitions between samples can be missed;
- accuracy depends on the sampling interval;
- `R` includes runnable threads waiting on CPU;
- disappearing threads are finalized using the next process snapshot time;
- threads that appear between samples are tracked from their first observation.

## Backend Architecture

The code is layered:

```text
CLI
  -> app config
  -> collector
  -> backend interface
  -> accumulator
  -> output renderer
```

The backend interface is intentionally small for the MVP:

```go
type Backend interface {
    Name() string
    Capabilities() model.BackendCapabilities
    Snapshot(ctx context.Context, pid int) ([]model.ThreadSample, error)
    Close() error
}
```

The current backend semantics are:

- `proc`: sampled observed thread state;
- `taskstats`: future Linux Delay Accounting counters;
- `ebpf`: future event-driven scheduler timeline.

These sources are not treated as interchangeable ground truth.

## Why Taskstats Is Future/Experimental

The taskstats backend, when implemented, should not be treated as exact live
thread-state tracing. It exposes Linux Delay Accounting counters and may be
affected by lazy accounting on modern kernels.

Taskstats also depends on kernel configuration, runtime delay accounting, and
privileges. A future implementation must parse Generic Netlink TLVs safely,
validate lengths, avoid unsafe binary casts, validate taskstats versions, and
include binary fixture tests.

## Why eBPF Is the Accurate Future Backend

An eBPF backend can observe scheduler events rather than periodic snapshots.
That makes it the right long-term direction for precise scheduler-event
analysis, runnable wait time, on-CPU time, wakeups, and context switches.

Likely tracepoints:

- `sched:sched_switch`
- `sched:sched_wakeup`
- `sched:sched_wakeup_new`
- `sched:sched_process_exit`

## Doctor

```bash
tsastat doctor
```

Example:

```text
proc backend:
  status: OK
  reason: /proc is available and readable

taskstats backend:
  status: NOT IMPLEMENTED
  warning: taskstats depends on CONFIG_TASKSTATS, CONFIG_TASK_DELAY_ACCT, and kernel.task_delayacct
  warning: taskstats counters may be lazily updated and should not be treated as exact live thread-state transitions

ebpf backend:
  status: NOT IMPLEMENTED
  warning: eBPF backend will require scheduler tracepoints and appropriate capabilities
```

Doctor reports future backend limitations without failing the whole command.

## Roadmap

1. CSV output.
2. Process-level summary rows.
3. Better sorting.
4. Better filtering.
5. `--all-threads` and `--top` modes.
6. Experimental taskstats backend.
7. Taskstats doctor checks.
8. eBPF backend using scheduler tracepoints.
9. Runnable wait time.
10. On-CPU time.
11. Context switch counters.
12. Wakeup counters.
13. Optional TUI frontend.
14. Performance tests with high thread counts.
