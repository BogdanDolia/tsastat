package doctor

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/BogdanDolia/tsastat/internal/backend/ebpf"
	"github.com/BogdanDolia/tsastat/internal/backend/proc"
	"github.com/BogdanDolia/tsastat/internal/backend/taskstats"
	"github.com/BogdanDolia/tsastat/internal/model"
	"github.com/BogdanDolia/tsastat/internal/procfs"
)

func Run(w io.Writer) error {
	writeAuto(w)
	fmt.Fprintln(w)
	writeProc(w)
	fmt.Fprintln(w)
	writeTaskstats(w)
	fmt.Fprintln(w)
	writeEBPF(w)
	return nil
}

func writeAuto(w io.Writer) {
	fmt.Fprintln(w, "auto/hybrid backend:")
	fmt.Fprintln(w, "  policy: combine eBPF scheduler events, taskstats delay counters, and procfs identity/state data")
	fmt.Fprintln(w, "  fallback: taskstats plus procfs when eBPF is unavailable; procfs alone when taskstats is also unavailable")
	fmt.Fprintln(w, "  output: active_sources, unavailable_sources, and hybrid_identity_mismatches report the selected path")
}

func writeProc(w io.Writer) {
	fmt.Fprintln(w, "proc backend:")
	if stat, err := os.Stat("/proc"); err != nil || !stat.IsDir() {
		fmt.Fprintln(w, "  status: NOT OK")
		if err != nil {
			fmt.Fprintf(w, "  reason: %v\n", err)
		} else {
			fmt.Fprintln(w, "  reason: /proc is not a directory")
		}
		return
	}
	if _, err := os.ReadDir("/proc"); err != nil {
		fmt.Fprintln(w, "  status: NOT OK")
		fmt.Fprintf(w, "  reason: /proc is not readable: %v\n", err)
		return
	}

	fmt.Fprintln(w, "  status: OK")
	fmt.Fprintln(w, "  reason: /proc is available and readable")
	writeSchedstatStatus(w)
	for _, warning := range proc.Capabilities().Warnings {
		fmt.Fprintf(w, "  warning: %s\n", warning)
	}
}

func writeSchedstatStatus(w io.Writer) {
	data, err := os.ReadFile("/proc/self/schedstat")
	if err != nil {
		fmt.Fprintf(w, "  schedstat: unavailable (%v)\n", err)
		return
	}
	if _, err := procfs.ParseSchedstatLine(string(data)); err != nil {
		fmt.Fprintf(w, "  schedstat: invalid (%v)\n", err)
		return
	}
	fmt.Fprintln(w, "  schedstat: available")
	if data, err := os.ReadFile("/proc/sys/kernel/sched_schedstats"); err == nil {
		value := strings.TrimSpace(string(data))
		fmt.Fprintf(w, "  kernel.sched_schedstats: %s\n", value)
		if value == "0" {
			fmt.Fprintln(w, "  warning: scheduler statistics are runtime-disabled; verify schedstat counters advance on this kernel")
		}
	}
}

func writeTaskstats(w io.Writer) {
	fmt.Fprintln(w, "taskstats backend:")
	if runtime.GOOS != "linux" {
		fmt.Fprintln(w, "  status: NOT OK")
		fmt.Fprintf(w, "  reason: taskstats requires Linux; current platform is %s/%s\n", runtime.GOOS, runtime.GOARCH)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	stats, err := taskstats.Probe(ctx)
	if err != nil {
		fmt.Fprintln(w, "  status: NOT OK")
		fmt.Fprintf(w, "  reason: TASKSTATS query failed: %v\n", err)
	} else {
		fmt.Fprintln(w, "  status: OK")
		fmt.Fprintln(w, "  reason: TASKSTATS Generic Netlink query succeeded")
		fmt.Fprintf(w, "  taskstats version: %d\n", stats.Version)
		fmt.Fprintf(w, "  delay fields: %s\n", strings.Join(availableDelayFields(stats), ", "))
	}
	fmt.Fprintln(w, "  warning: taskstats depends on CONFIG_TASKSTATS, CONFIG_TASK_DELAY_ACCT, CAP_NET_ADMIN, and kernel.task_delayacct")
	for _, warning := range taskstats.Capabilities().Warnings {
		fmt.Fprintf(w, "  warning: %s\n", warning)
	}

	data, err := os.ReadFile("/proc/sys/kernel/task_delayacct")
	if err != nil {
		fmt.Fprintf(w, "  delayacct: unavailable (%v)\n", err)
		return
	}
	switch strings.TrimSpace(string(data)) {
	case "1":
		fmt.Fprintln(w, "  delayacct: enabled")
	case "0":
		fmt.Fprintln(w, "  delayacct: disabled")
		fmt.Fprintln(w, "  warning: non-CPU resource counters remain zero unless the target task started after delay accounting was enabled")
	default:
		fmt.Fprintf(w, "  delayacct: %s\n", strings.TrimSpace(string(data)))
	}
}

func availableDelayFields(stats model.DelayCounters) []string {
	fields := make([]string, 0, 8)
	for _, field := range []struct {
		name      string
		available bool
	}{
		{name: "cpu", available: stats.CPU.Available},
		{name: "block_io", available: stats.BlockIO.Available},
		{name: "swap_in", available: stats.SwapIn.Available},
		{name: "reclaim", available: stats.Reclaim.Available},
		{name: "thrashing", available: stats.Thrashing.Available},
		{name: "compaction", available: stats.Compaction.Available},
		{name: "write_protect_copy", available: stats.WriteProtectCopy.Available},
		{name: "irq", available: stats.IRQ.Available},
	} {
		if field.available {
			fields = append(fields, field.name)
		}
	}
	if len(fields) == 0 {
		return []string{"none"}
	}
	return fields
}

func writeEBPF(w io.Writer) {
	fmt.Fprintln(w, "ebpf backend:")
	if runtime.GOOS != "linux" {
		fmt.Fprintln(w, "  status: NOT OK")
		fmt.Fprintf(w, "  reason: eBPF scheduler tracing requires Linux; current platform is %s/%s\n", runtime.GOOS, runtime.GOARCH)
		return
	}

	releaseData, releaseErr := os.ReadFile("/proc/sys/kernel/osrelease")
	release := strings.TrimSpace(string(releaseData))
	kernelSupported := false
	if releaseErr == nil {
		fmt.Fprintf(w, "  kernel: %s\n", release)
		major, minor, ok := parseKernelVersion(release)
		kernelSupported = ok && (major > 6 || major == 6 && minor >= 1)
		if !kernelSupported {
			fmt.Fprintln(w, "  warning: upstream Linux 6.1 or newer is required for the sched_switch prev_state contract")
		}
	} else {
		fmt.Fprintf(w, "  kernel: unavailable (%v)\n", releaseErr)
	}

	btfAvailable := false
	if stat, err := os.Stat("/sys/kernel/btf/vmlinux"); err == nil && !stat.IsDir() {
		btfAvailable = true
		fmt.Fprintln(w, "  kernel BTF: available")
	} else if err != nil {
		fmt.Fprintf(w, "  kernel BTF: unavailable (%v)\n", err)
	}
	if btfAvailable && kernelSupported {
		fmt.Fprintln(w, "  status: prerequisites detected")
		fmt.Fprintln(w, "  reason: the embedded CO-RE programs can be attempted on this kernel")
	} else {
		fmt.Fprintln(w, "  status: NOT OK")
		if !btfAvailable {
			fmt.Fprintln(w, "  reason: /sys/kernel/btf/vmlinux is required for tp_btf CO-RE relocation")
		} else {
			fmt.Fprintln(w, "  reason: the detected upstream kernel version is older than 6.1")
		}
	}

	for _, warning := range ebpf.Capabilities().Warnings {
		fmt.Fprintf(w, "  warning: %s\n", warning)
	}
	fmt.Fprintf(w, "  euid: %d\n", os.Geteuid())
	if data, err := os.ReadFile("/proc/sys/kernel/unprivileged_bpf_disabled"); err == nil {
		fmt.Fprintf(w, "  kernel.unprivileged_bpf_disabled: %s\n", strings.TrimSpace(string(data)))
	}
}

func parseKernelVersion(release string) (int, int, bool) {
	parts := strings.SplitN(release, ".", 3)
	if len(parts) < 2 {
		return 0, 0, false
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, false
	}
	minorText := parts[1]
	if index := strings.IndexFunc(minorText, func(r rune) bool { return r < '0' || r > '9' }); index >= 0 {
		minorText = minorText[:index]
	}
	minor, err := strconv.Atoi(minorText)
	if err != nil {
		return 0, 0, false
	}
	return major, minor, true
}
