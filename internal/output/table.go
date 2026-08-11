package output

import (
	"fmt"
	"io"
	"strconv"
	"strings"
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

func (r *TableRenderer) Render(report model.IntervalReport) error {
	tw := tabwriter.NewWriter(r.w, 0, 0, 2, ' ', 0)
	if !r.noHeader {
		if _, err := fmt.Fprintln(tw, "TIME\tPID\tTID\tSTART_TICKS\tSTART_NS\tCOMM\tSOURCES\tUNAVAILABLE\tID_MISMATCH\tRUN_ms\tCPU_ms\tRQ_ms\tSLICES\tSCHED_OBS_ms\tEVTS\tWAKEUPS\tINCOMP_WAKE\tWAKE_AVG_us\tWAKE_MAX_us\tLOST_TOTAL\tLATE\tINIT_RACE\tCLK_UNCERT_ns\tTS_VER\tCPU_DLY_ms\tIO_DLY_ms\tSWAP_DLY_ms\tRECLAIM_ms\tTHRASH_ms\tCOMPACT_ms\tWPCOPY_ms\tIRQ_DLY_ms\tDLY_PAIRS\tDLY_RESETS\tSLEEP_ms\tD_ms\tSTOP_ms\tZ_ms\tUNK_ms\tOBS_ms\tGAP_ms\tUNCERT_ms\tRUN_%\tSLEEP_%\tD_%"); err != nil {
			return err
		}
	}

	for _, stat := range report.Threads {
		fields := []string{
			formatTime(stat.IntervalEnd),
			strconv.Itoa(stat.PID),
			strconv.Itoa(stat.TID),
			strconv.FormatUint(stat.StartTimeTicks, 10),
			optionalUint64(stat.StartTimeNanoseconds, stat.StartTimeNanoseconds != 0),
			stat.Comm,
			optionalList(report.Quality.ActiveSources),
			optionalList(report.Quality.UnavailableSources),
			optionalInt(report.Quality.HybridIdentityMismatches, len(report.Quality.ActiveSources) > 1 || len(report.Quality.UnavailableSources) > 0),
			strconv.FormatInt(millis(stat.Duration(model.StateRunning)), 10),
			optionalMillis(stat.OnCPU, stat.SchedulerAvailable()),
			optionalMillis(stat.RunqueueWait, stat.SchedulerAvailable()),
			optionalUint64(stat.Timeslices, stat.SchedulerAvailable()),
			optionalMillis(stat.SchedulerObserved, stat.SchedulerAvailable()),
			optionalInt(stat.SchedulerEventCount, stat.SchedulerSource == "ebpf_sched_events"),
			optionalInt(stat.WakeupCount, stat.SchedulerSource == "ebpf_sched_events"),
			optionalInt(stat.IncompleteWakeupCount, stat.SchedulerSource == "ebpf_sched_events"),
			optionalMicroseconds(stat.AverageWakeupLatency(), stat.SchedulerSource == "ebpf_sched_events"),
			optionalMicroseconds(stat.WakeupLatencyMax, stat.SchedulerSource == "ebpf_sched_events"),
			optionalUint64(report.Quality.SchedulerLostEventsTotal, report.Quality.SchedulerEventTimeline),
			optionalInt(report.Quality.SchedulerLateEvents, report.Quality.SchedulerEventTimeline),
			optionalInt(report.Quality.InitializationRaces, report.Quality.SchedulerEventTimeline),
			optionalNanoseconds(report.Quality.ClockCalibrationUncertainty, report.Quality.SchedulerEventTimeline),
			optionalUint64(uint64(stat.DelayVersion), stat.DelayVersion != 0),
			optionalDelayMillis(stat.Delays.CPU),
			optionalDelayMillis(stat.Delays.BlockIO),
			optionalDelayMillis(stat.Delays.SwapIn),
			optionalDelayMillis(stat.Delays.Reclaim),
			optionalDelayMillis(stat.Delays.Thrashing),
			optionalDelayMillis(stat.Delays.Compaction),
			optionalDelayMillis(stat.Delays.WriteProtectCopy),
			optionalDelayMillis(stat.Delays.IRQ),
			optionalInt(stat.DelaySamplePairs, stat.DelayCountersAvailable()),
			optionalInt(stat.DelayCounterResets, stat.DelayVersion != 0),
			strconv.FormatInt(millis(stat.Duration(model.StateSleeping)), 10),
			strconv.FormatInt(millis(stat.Duration(model.StateUninterruptible)), 10),
			strconv.FormatInt(millis(stat.StopDuration()), 10),
			strconv.FormatInt(millis(stat.Duration(model.StateZombie)), 10),
			strconv.FormatInt(millis(stat.UnknownDuration()), 10),
			strconv.FormatInt(millis(stat.TotalObserved), 10),
			strconv.FormatInt(millis(stat.MaxSampleGap), 10),
			strconv.FormatInt(millis(stat.DetectedTransitionUncertainty), 10),
			fmt.Sprintf("%.1f", stat.Percent(model.StateRunning)),
			fmt.Sprintf("%.1f", stat.Percent(model.StateSleeping)),
			fmt.Sprintf("%.1f", stat.Percent(model.StateUninterruptible)),
		}
		if _, err := fmt.Fprintln(tw, strings.Join(fields, "\t")); err != nil {
			return err
		}
	}

	return tw.Flush()
}

func optionalList(values []string) string {
	if len(values) == 0 {
		return "-"
	}
	return strings.Join(values, ",")
}

func optionalDelayMillis(counter model.DelayIntervalCounter) string {
	return optionalMillis(counter.Total, counter.Available)
}

func millis(d time.Duration) int64 {
	return d.Milliseconds()
}

func optionalMillis(d time.Duration, available bool) string {
	if !available {
		return "-"
	}
	return fmt.Sprintf("%d", millis(d))
}

func optionalUint64(value uint64, available bool) string {
	if !available {
		return "-"
	}
	return fmt.Sprintf("%d", value)
}

func optionalInt(value int, available bool) string {
	if !available {
		return "-"
	}
	return fmt.Sprintf("%d", value)
}

func optionalMicroseconds(d time.Duration, available bool) string {
	if !available {
		return "-"
	}
	return fmt.Sprintf("%d", d.Microseconds())
}

func optionalNanoseconds(d time.Duration, available bool) string {
	if !available {
		return "-"
	}
	return fmt.Sprintf("%d", d.Nanoseconds())
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("15:04:05")
}
