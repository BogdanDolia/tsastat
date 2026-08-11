package ebpf

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"

	"github.com/BogdanDolia/tsastat/internal/model"
)

func TestSchedulerEventWireContract(t *testing.T) {
	if size := binary.Size(bpfSchedulerEvent{}); size != bpfEventSize {
		t.Fatalf("wire struct size = %d, want %d", size, bpfEventSize)
	}
	rawEvent := bpfSchedulerEvent{
		TimestampNS: 123456,
		StartTimeNS: 987654,
		State:       taskUninterruptible,
		TGID:        100,
		TID:         101,
		CPU:         3,
		Type:        bpfEventSwitchOut,
	}
	copy(rawEvent.Comm[:], "worker")
	var raw bytes.Buffer
	if err := binary.Write(&raw, binary.NativeEndian, rawEvent); err != nil {
		t.Fatalf("binary.Write returned error: %v", err)
	}

	timestampNS, event, err := decodeSchedulerEvent(raw.Bytes())
	if err != nil {
		t.Fatalf("decodeSchedulerEvent returned error: %v", err)
	}
	if timestampNS != 123456 || event.PID != 100 || event.TID != 101 || event.CPU != 3 || event.Comm != "worker" {
		t.Fatalf("decoded event = %#v timestamp=%d", event, timestampNS)
	}
	if event.Kind != model.SchedulerEventSwitchOut || event.State != model.SchedulerStateUninterruptible {
		t.Fatalf("decoded kind/state = %d/%d", event.Kind, event.State)
	}
}

func TestSchedulerEventWireRejectsInvalidRecords(t *testing.T) {
	if _, _, err := decodeSchedulerEvent(make([]byte, bpfEventSize-1)); err == nil || !strings.Contains(err.Error(), "size") {
		t.Fatalf("invalid-size error = %v", err)
	}
	rawEvent := bpfSchedulerEvent{Type: 99}
	var raw bytes.Buffer
	if err := binary.Write(&raw, binary.NativeEndian, rawEvent); err != nil {
		t.Fatalf("binary.Write returned error: %v", err)
	}
	if _, _, err := decodeSchedulerEvent(raw.Bytes()); err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("unknown-type error = %v", err)
	}
}
