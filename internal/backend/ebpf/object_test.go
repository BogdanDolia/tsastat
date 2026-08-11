package ebpf

import (
	"bytes"
	_ "embed"
	"testing"

	ciliumebpf "github.com/cilium/ebpf"
)

//go:embed scheduler_bpfel.o
var testSchedulerObject []byte

func TestEmbeddedSchedulerObjectContract(t *testing.T) {
	spec, err := ciliumebpf.LoadCollectionSpecFromReader(bytes.NewReader(testSchedulerObject))
	if err != nil {
		t.Fatalf("LoadCollectionSpecFromReader returned error: %v", err)
	}
	for _, name := range []string{"config", "events", "lost_events"} {
		if spec.Maps[name] == nil {
			t.Fatalf("embedded object missing map %q", name)
		}
	}
	for name, attachTo := range map[string]string{
		"handle_sched_switch":     "sched_switch",
		"handle_sched_wakeup":     "sched_wakeup",
		"handle_sched_wakeup_new": "sched_wakeup_new",
	} {
		program := spec.Programs[name]
		if program == nil {
			t.Fatalf("embedded object missing program %q", name)
		}
		if program.Type != ciliumebpf.Tracing || program.AttachTo != attachTo {
			t.Fatalf("program %q = type %s attach-to %q", name, program.Type, program.AttachTo)
		}
	}
}
