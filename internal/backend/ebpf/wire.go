package ebpf

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/BogdanDolia/tsastat/internal/model"
)

const (
	bpfEventWakeup    uint32 = 1
	bpfEventSwitchIn  uint32 = 2
	bpfEventSwitchOut uint32 = 3
	bpfEventSize             = 64
)

type bpfSchedulerEvent struct {
	TimestampNS uint64
	StartTimeNS uint64
	State       uint64
	TGID        uint32
	TID         uint32
	CPU         uint32
	Type        uint32
	Flags       uint32
	Comm        [16]byte
	Padding     uint32
}

func decodeSchedulerEvent(raw []byte) (uint64, model.SchedulerEvent, error) {
	if len(raw) != bpfEventSize {
		return 0, model.SchedulerEvent{}, fmt.Errorf("invalid scheduler event size %d, want %d", len(raw), bpfEventSize)
	}
	var rawEvent bpfSchedulerEvent
	if err := binary.Read(bytes.NewReader(raw), binary.NativeEndian, &rawEvent); err != nil {
		return 0, model.SchedulerEvent{}, fmt.Errorf("decode scheduler event: %w", err)
	}

	kind := model.SchedulerEventKind(0)
	state := model.SchedulerStateUnknown
	switch rawEvent.Type {
	case bpfEventWakeup:
		kind = model.SchedulerEventWakeup
		state = model.SchedulerStateRunnable
	case bpfEventSwitchIn:
		kind = model.SchedulerEventSwitchIn
		state = model.SchedulerStateOnCPU
	case bpfEventSwitchOut:
		kind = model.SchedulerEventSwitchOut
		state = stateAfterSwitch(rawEvent.State, rawEvent.Flags)
	default:
		return 0, model.SchedulerEvent{}, fmt.Errorf("unknown scheduler event type %d", rawEvent.Type)
	}

	return rawEvent.TimestampNS, model.SchedulerEvent{
		PID:                  int(rawEvent.TGID),
		TID:                  int(rawEvent.TID),
		StartTimeNanoseconds: rawEvent.StartTimeNS,
		Comm:                 nullTerminatedString(rawEvent.Comm[:]),
		CPU:                  int(rawEvent.CPU),
		Kind:                 kind,
		State:                state,
	}, nil
}

func nullTerminatedString(value []byte) string {
	if index := bytes.IndexByte(value, 0); index >= 0 {
		value = value[:index]
	}
	return string(value)
}
