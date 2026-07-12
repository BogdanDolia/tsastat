package app

import (
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// Version is replaced at release time through the Go linker -X flag.
var Version = "dev"

type Config struct {
	PID            int
	Interval       time.Duration
	SampleInterval time.Duration
	Count          int
	Backend        string
	Output         string
	TID            int
	Comm           string
	ShowIdle       bool
	Sort           string
	NoHeader       bool
	Version        bool
}

func parseConfig(args []string, stderr io.Writer) (Config, error) {
	var cfg Config
	cfg.Backend = "proc"
	cfg.Output = "table"
	cfg.Sort = "tid"
	cfg.SampleInterval = 10 * time.Millisecond

	fs := flag.NewFlagSet("tsastat", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprint(stderr, usage())
		fs.PrintDefaults()
	}

	intervalValue := durationFlag{}
	sampleValue := durationFlag{value: cfg.SampleInterval}
	countValue := countFlag{}

	fs.IntVar(&cfg.PID, "p", 0, "target process ID")
	fs.IntVar(&cfg.PID, "pid", 0, "target process ID")
	fs.Var(&intervalValue, "i", "report interval (for example 500ms, 1s, 2s)")
	fs.Var(&intervalValue, "interval", "report interval (for example 500ms, 1s, 2s)")
	fs.Var(&sampleValue, "sample", "proc sampling interval")
	fs.Var(&sampleValue, "sample-interval", "proc sampling interval")
	fs.Var(&countValue, "c", "number of intervals to print")
	fs.Var(&countValue, "count", "number of intervals to print")
	fs.StringVar(&cfg.Backend, "b", cfg.Backend, "backend: proc, taskstats, ebpf")
	fs.StringVar(&cfg.Backend, "backend", cfg.Backend, "backend: proc, taskstats, ebpf")
	fs.StringVar(&cfg.Output, "o", cfg.Output, "output format: table, json, csv")
	fs.StringVar(&cfg.Output, "output", cfg.Output, "output format: table, json, csv")
	fs.IntVar(&cfg.TID, "tid", 0, "filter by thread ID")
	fs.StringVar(&cfg.Comm, "comm", "", "filter by thread name substring or glob pattern")
	fs.BoolVar(&cfg.ShowIdle, "show-idle", false, "show threads observed in the idle state")
	fs.StringVar(&cfg.Sort, "sort", cfg.Sort, "sort field: tid, comm, running, sleeping, uninterruptible, total")
	fs.BoolVar(&cfg.NoHeader, "no-header", false, "suppress table headers")
	fs.BoolVar(&cfg.Version, "version", false, "print version")

	if err := fs.Parse(args); err != nil {
		return cfg, err
	}

	cfg.Interval = intervalValue.value
	cfg.SampleInterval = sampleValue.value
	cfg.Count = countValue.value

	if cfg.Version {
		return cfg, nil
	}

	positionals := fs.Args()
	if len(positionals) > 1 {
		return cfg, fmt.Errorf("unexpected arguments: %s", strings.Join(positionals[1:], " "))
	}
	if len(positionals) == 1 {
		if intervalValue.set {
			return cfg, fmt.Errorf("interval specified both with --interval and positional argument")
		}
		interval, err := parseInterval(positionals[0])
		if err != nil {
			return cfg, err
		}
		cfg.Interval = interval
		intervalValue.set = true
	}

	if cfg.PID <= 0 {
		return cfg, fmt.Errorf("invalid PID %d", cfg.PID)
	}
	if !intervalValue.set {
		return cfg, fmt.Errorf("invalid interval: interval is required")
	}
	if cfg.Interval <= 0 {
		return cfg, fmt.Errorf("invalid interval %s", cfg.Interval)
	}
	if cfg.SampleInterval <= 0 {
		return cfg, fmt.Errorf("invalid sample interval %s", cfg.SampleInterval)
	}
	if cfg.SampleInterval >= cfg.Interval {
		return cfg, fmt.Errorf("sample interval %s must be shorter than report interval %s", cfg.SampleInterval, cfg.Interval)
	}
	if countValue.set && cfg.Count <= 0 {
		return cfg, fmt.Errorf("invalid count %d", cfg.Count)
	}
	if cfg.TID < 0 {
		return cfg, fmt.Errorf("invalid tid %d", cfg.TID)
	}

	return cfg, nil
}

type durationFlag struct {
	value time.Duration
	set   bool
}

func (f *durationFlag) String() string {
	return f.value.String()
}

func (f *durationFlag) Set(raw string) error {
	value, err := parseInterval(raw)
	if err != nil {
		return err
	}
	f.value = value
	f.set = true
	return nil
}

func parseInterval(raw string) (time.Duration, error) {
	if raw == "" {
		return 0, fmt.Errorf("invalid interval %q", raw)
	}
	if isPositiveInteger(raw) {
		seconds, err := strconv.Atoi(raw)
		if err != nil {
			return 0, fmt.Errorf("invalid interval %q: %w", raw, err)
		}
		if seconds <= 0 {
			return 0, fmt.Errorf("invalid interval %q", raw)
		}
		return time.Duration(seconds) * time.Second, nil
	}
	value, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid interval %q: %w", raw, err)
	}
	if value <= 0 {
		return 0, fmt.Errorf("invalid interval %q", raw)
	}
	return value, nil
}

func isPositiveInteger(raw string) bool {
	for _, r := range raw {
		if r < '0' || r > '9' {
			return false
		}
	}
	return raw != ""
}

type countFlag struct {
	value int
	set   bool
}

func (f *countFlag) String() string {
	return strconv.Itoa(f.value)
}

func (f *countFlag) Set(raw string) error {
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fmt.Errorf("invalid count %q: %w", raw, err)
	}
	f.value = value
	f.set = true
	return nil
}

func usage() string {
	return `Usage:
  tsastat -p <pid> <interval>
  tsastat -p <pid> -i <duration> [--sample duration] [--count n]
  tsastat doctor

Examples:
  tsastat -p 1234 1
  tsastat -p 1234 -i 1s --sample 10ms --count 10
  tsastat -p 1234 -i 1s --sample 20ms --output json

tsastat samples Linux thread states repeatedly within each report interval.
The proc backend is a sampling approximation: it can miss short-lived state
transitions between samples, and R means running or runnable, not necessarily
actively on CPU.

Flags:
`
}
