package doctor

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/BogdanDolia/tsastat/internal/model"
)

func TestWriteAutoDocumentsSelectionAndFallback(t *testing.T) {
	var output bytes.Buffer
	writeAuto(&output)
	for _, want := range []string{"combine eBPF", "taskstats plus procfs", "active_sources"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("auto doctor output missing %q:\n%s", want, output.String())
		}
	}
}

func TestParseKernelVersion(t *testing.T) {
	tests := []struct {
		release string
		major   int
		minor   int
		ok      bool
	}{
		{release: "6.1.0-27-amd64", major: 6, minor: 1, ok: true},
		{release: "6.12.8", major: 6, minor: 12, ok: true},
		{release: "5.15.0", major: 5, minor: 15, ok: true},
		{release: "invalid", ok: false},
	}
	for _, test := range tests {
		major, minor, ok := parseKernelVersion(test.release)
		if major != test.major || minor != test.minor || ok != test.ok {
			t.Fatalf("parseKernelVersion(%q) = %d,%d,%t, want %d,%d,%t",
				test.release, major, minor, ok, test.major, test.minor, test.ok)
		}
	}
}

func TestAvailableDelayFields(t *testing.T) {
	stats := model.DelayCounters{
		CPU:     model.DelayCounter{Available: true},
		BlockIO: model.DelayCounter{Available: true},
		IRQ:     model.DelayCounter{Available: true},
	}
	want := []string{"cpu", "block_io", "irq"}
	if got := availableDelayFields(stats); !reflect.DeepEqual(got, want) {
		t.Fatalf("availableDelayFields() = %v, want %v", got, want)
	}
}
