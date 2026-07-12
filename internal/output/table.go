package output

import (
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/BogdanDolia/tsastat/internal/model"
)

type TableRenderer struct {
	w        io.Writer
	noHeader bool
}

func NewTableRenderer(w io.Writer, noHeader bool) *TableRenderer {
	return &TableRenderer{w: w, noHeader: noHeader}
}

func (r *TableRenderer) Render(stats []model.ThreadIntervalStats) error {
	tw := tabwriter.NewWriter(r.w, 0, 0, 2, ' ', 0)
	if !r.noHeader {
		if _, err := fmt.Fprintln(tw, "TIME\tPID\tTID\tCOMM\tRUN_ms\tSLEEP_ms\tD_ms\tSTOP_ms\tZ_ms\tRUN_%\tSLEEP_%\tD_%"); err != nil {
			return err
		}
	}

	for _, stat := range stats {
		if _, err := fmt.Fprintf(
			tw,
			"%s\t%d\t%d\t%s\t%d\t%d\t%d\t%d\t%d\t%.1f\t%.1f\t%.1f\n",
			formatTime(stat.IntervalEnd),
			stat.PID,
			stat.TID,
			stat.Comm,
			millis(stat.Duration(model.StateRunning)),
			millis(stat.Duration(model.StateSleeping)),
			millis(stat.Duration(model.StateUninterruptible)),
			millis(stat.StopDuration()),
			millis(stat.Duration(model.StateZombie)),
			stat.Percent(model.StateRunning),
			stat.Percent(model.StateSleeping),
			stat.Percent(model.StateUninterruptible),
		); err != nil {
			return err
		}
	}

	return tw.Flush()
}

func millis(d time.Duration) int64 {
	return d.Milliseconds()
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("15:04:05")
}
