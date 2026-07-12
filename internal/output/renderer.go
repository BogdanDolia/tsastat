package output

import (
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/BogdanDolia/tsastat/internal/model"
)

var ErrOutputNotImplemented = errors.New("output format not implemented")

type Renderer interface {
	Render([]model.ThreadIntervalStats) error
}

type RendererOptions struct {
	Format         string
	Writer         io.Writer
	PID            int
	Backend        string
	Interval       time.Duration
	SampleInterval time.Duration
	NoHeader       bool
}

type OutputError struct {
	Format string
	Err    error
}

func (e OutputError) Error() string {
	if errors.Is(e.Err, ErrOutputNotImplemented) {
		return fmt.Sprintf("output format %q is not implemented yet", e.Format)
	}
	return fmt.Sprintf("output format %q: %v", e.Format, e.Err)
}

func (e OutputError) Unwrap() error {
	return e.Err
}

func NewRenderer(opts RendererOptions) (Renderer, error) {
	switch opts.Format {
	case "", "table":
		return NewTableRenderer(opts.Writer, opts.NoHeader), nil
	case "json":
		return NewJSONRenderer(opts.Writer, opts.PID, opts.Backend, opts.Interval, opts.SampleInterval), nil
	case "csv":
		return nil, OutputError{Format: "csv", Err: ErrOutputNotImplemented}
	default:
		return nil, fmt.Errorf("unsupported output format %q", opts.Format)
	}
}
