package doctor

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/BogdanDolia/tsastat/internal/backend/ebpf"
	"github.com/BogdanDolia/tsastat/internal/backend/proc"
	"github.com/BogdanDolia/tsastat/internal/backend/taskstats"
	"github.com/BogdanDolia/tsastat/internal/procfs"
)

func Run(w io.Writer) error {
	writeProc(w)
	fmt.Fprintln(w)
	writeTaskstats(w)
	fmt.Fprintln(w)
	writeEBPF(w)
	return nil
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
	fmt.Fprintln(w, "  status: NOT IMPLEMENTED")
	fmt.Fprintln(w, "  warning: taskstats depends on CONFIG_TASKSTATS, CONFIG_TASK_DELAY_ACCT, and kernel.task_delayacct")
	fmt.Fprintln(w, "  warning: taskstats counters may be lazily updated and should not be treated as exact live thread-state transitions")
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
	default:
		fmt.Fprintf(w, "  delayacct: %s\n", strings.TrimSpace(string(data)))
	}
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
