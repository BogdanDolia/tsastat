# tsastat

[![CI](https://github.com/BogdanDolia/tsastat/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/BogdanDolia/tsastat/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/BogdanDolia/tsastat)](https://github.com/BogdanDolia/tsastat/releases)
[![Go version](https://img.shields.io/github/go-mod/go-version/BogdanDolia/tsastat)](https://go.dev/)
[![License](https://img.shields.io/github/license/BogdanDolia/tsastat)](LICENSE)
[![Platform](https://img.shields.io/badge/platform-Linux-blue)](#requirements)

`tsastat` is a lightweight Linux CLI for sampled per-thread state analysis.
Point it at a process to see which threads are running, sleeping, waiting on
I/O, stopped, or becoming zombies during each reporting interval.

```text
tsastat: pid=4242 backend=proc interval=1s sample=10ms

TIME      PID   TID   COMM          RUN_ms  SLEEP_ms  D_ms  STOP_ms  Z_ms  RUN_%  SLEEP_%  D_%
12:10:01  4242  4242  app           18      982       0     0        0     1.8    98.2     0.0
12:10:01  4242  4243  worker-1      240     760       0     0        0     24.0   76.0     0.0
12:10:01  4242  4244  io-worker     10      900       90    0        0     1.0    90.0     9.0
```

> [!IMPORTANT]
> The current `proc` backend is a sampling approximation, not a scheduler
> timeline. It can miss state transitions that happen between samples.

## Why tsastat?

Tools such as `top` and `pidstat` provide broad process statistics. `tsastat`
focuses on one question: **how did each thread of this process spend the
observed interval?**

It is useful for:

- finding unexpectedly busy or permanently sleeping threads;
- spotting threads observed in uninterruptible I/O wait (`D`);
- comparing thread behavior before and during a workload;
- exporting interval data as JSON Lines for later analysis;
- learning how Linux exposes task state through procfs.

## Requirements

- Linux with procfs mounted at `/proc`;
- Go 1.22 or newer to build from source;
- permission to read `/proc/<pid>/task` for the target process.

The `proc` backend does not normally require root when inspecting your own
processes.

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
| `--sample` | Time between procfs samples | `10ms` |
| `-c`, `--count` | Number of reports; omitted means continuous | continuous |
| `--tid` | Show only one thread ID | all threads |
| `--comm` | Filter thread names by substring or glob | no filter |
| `--sort` | Sort by `tid`, `comm`, `running`, `sleeping`, `uninterruptible`, or `total` | `tid` |
| `-o`, `--output` | Output format: `table` or `json` | `table` |
| `--show-idle` | Include threads observed only in the idle state | disabled |
| `--no-header` | Suppress table headers | disabled |

The sampling interval must be shorter than the report interval. Shorter
sampling intervals can capture more transitions, but they also add overhead.

## JSON Lines output

Use JSON Lines when piping data into tools such as `jq` or saving it for later:

```bash
tsastat -p 1234 --sample 10ms --interval 1s --count 10 \
  --output json > thread-states.jsonl
```

Each line represents one reporting interval:

```json
{"timestamp":"2026-05-08T12:01:01Z","pid":1234,"backend":"proc","interval_ms":1000,"sample_interval_ms":10,"threads":[{"tid":1235,"comm":"worker-1","durations_ms":{"running":120,"sleeping":870,"uninterruptible":10,"stopped":0,"tracing_stop":0,"zombie":0},"percent":{"running":12,"sleeping":87,"uninterruptible":1}}]}
```

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

The procfs backend reads `/proc/<pid>/task/<tid>/stat` repeatedly. The time
between two samples is attributed to the state seen in the earlier sample.
Multiple samples are accumulated into each report.

Consequences of this approach:

- transitions shorter than the sampling interval can be missed;
- accuracy and overhead depend on `--sample`;
- report windows can be slightly longer than requested because of scheduling;
- disappearing threads are finalized at the next process snapshot;
- new threads are tracked from their first observation;
- percentages describe sampled state, not exact on-CPU time.

Use scheduler tracing with `perf`, ftrace, or eBPF when exact event timing is
required.

## Backends

| Backend | Status | Semantics |
| --- | --- | --- |
| `proc` | Available | Sampled observed thread state |
| `taskstats` | Planned | Linux Delay Accounting counters |
| `ebpf` | Planned | Event-driven scheduler timeline |

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
    Snapshot(ctx context.Context, pid int) ([]model.ThreadSample, error)
    Close() error
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
- eBPF scheduler-event tracing;
- runnable wait and on-CPU time;
- context-switch and wakeup counters;
- optional TUI frontend.

## License

Licensed under the [Apache License 2.0](LICENSE).
