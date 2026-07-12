package doctor

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"

	"tsastat/internal/backend/ebpf"
	"tsastat/internal/backend/proc"
	"tsastat/internal/backend/taskstats"
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
	for _, warning := range proc.Capabilities().Warnings {
		fmt.Fprintf(w, "  warning: %s\n", warning)
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
	fmt.Fprintln(w, "  status: NOT IMPLEMENTED")
	fmt.Fprintln(w, "  warning: eBPF backend will require scheduler tracepoints and appropriate capabilities")
	for _, warning := range ebpf.Capabilities().Warnings {
		fmt.Fprintf(w, "  warning: %s\n", warning)
	}
	fmt.Fprintf(w, "  euid: %d\n", os.Geteuid())
	if data, err := os.ReadFile("/proc/sys/kernel/osrelease"); err == nil {
		fmt.Fprintf(w, "  kernel: %s\n", strings.TrimSpace(string(data)))
	} else {
		fmt.Fprintf(w, "  kernel: unavailable on %s/%s\n", runtime.GOOS, runtime.GOARCH)
	}
}
