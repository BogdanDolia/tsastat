package app

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os/signal"
	"syscall"

	"tsastat/internal/backend"
	"tsastat/internal/collector"
	"tsastat/internal/doctor"
	"tsastat/internal/model"
	"tsastat/internal/output"
)

func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 && args[0] == "doctor" {
		if err := doctor.Run(stdout); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}

	cfg, err := parseConfig(args, stderr)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return 0
		}
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		if err.Error() != "flag: help requested" {
			fmt.Fprintln(stderr, err)
		}
		return 1
	}

	if cfg.Version {
		fmt.Fprintf(stdout, "tsastat %s\n", Version)
		return 0
	}

	b, err := backend.New(cfg.Backend)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer b.Close()

	renderer, err := output.NewRenderer(output.RendererOptions{
		Format:         cfg.Output,
		Writer:         stdout,
		PID:            cfg.PID,
		Backend:        b.Name(),
		Interval:       cfg.Interval,
		SampleInterval: cfg.SampleInterval,
		NoHeader:       cfg.NoHeader,
	})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	if cfg.Output == "table" && !cfg.NoHeader {
		fmt.Fprintf(stdout, "tsastat: pid=%d backend=%s interval=%s sample=%s\n\n", cfg.PID, b.Name(), cfg.Interval, cfg.SampleInterval)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	c := collector.New(b, cfg.PID, cfg.Interval, cfg.SampleInterval)
	err = c.Run(ctx, cfg.Count, func(stats []model.ThreadIntervalStats) error {
		filtered, err := output.FilterAndSort(stats, output.FilterSortOptions{
			TID:      cfg.TID,
			Comm:     cfg.Comm,
			ShowIdle: cfg.ShowIdle,
			Sort:     cfg.Sort,
		})
		if err != nil {
			return err
		}
		return renderer.Render(filtered)
	})
	if err != nil {
		var processGone model.ProcessNotFoundError
		if errors.As(err, &processGone) {
			fmt.Fprintln(stderr, processGone.Error())
			return 1
		}
		if errors.Is(err, context.Canceled) {
			return 0
		}
		fmt.Fprintln(stderr, err)
		return 1
	}

	return 0
}
