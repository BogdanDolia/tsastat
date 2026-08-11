//go:build linux

package taskstats

import (
	"encoding/binary"
	"errors"
	"testing"

	"golang.org/x/sys/unix"
)

func TestBuildNetlinkRequest(t *testing.T) {
	request := buildNetlinkRequest(42, 1, 1, 7, 9, 3, []byte{1, 2, 3, 4, 5})
	if got := binary.NativeEndian.Uint32(request[0:4]); got != uint32(len(request)) {
		t.Fatalf("nlmsg_len = %d, want %d", got, len(request))
	}
	if got := binary.NativeEndian.Uint16(request[4:6]); got != 42 {
		t.Fatalf("nlmsg_type = %d, want 42", got)
	}
	if request[16] != 1 || request[17] != 1 {
		t.Fatalf("generic netlink header = %v", request[16:20])
	}
	attributes, err := parseNetlinkAttributes(request[20:])
	if err != nil {
		t.Fatalf("parseNetlinkAttributes returned error: %v", err)
	}
	if len(attributes) != 1 || attributes[0].typeID != 3 || string(attributes[0].data) != string([]byte{1, 2, 3, 4, 5}) {
		t.Fatalf("attributes = %#v", attributes)
	}
}

func TestParseNetlinkResponseReturnsKernelErrno(t *testing.T) {
	message := make([]byte, netlinkHeaderLength+4)
	binary.NativeEndian.PutUint32(message[0:4], uint32(len(message)))
	binary.NativeEndian.PutUint16(message[4:6], netlinkErrorType)
	binary.NativeEndian.PutUint32(message[8:12], 7)
	code := int32(-int(unix.EPERM))
	binary.NativeEndian.PutUint32(message[16:20], uint32(code))

	_, _, err := parseNetlinkResponse(message, 7)
	if !errors.Is(err, unix.EPERM) {
		t.Fatalf("error = %v, want EPERM", err)
	}
}

func TestDecodeTaskstatsNestedResponse(t *testing.T) {
	stats := make([]byte, irqTotalOffset+uint64Size)
	binary.NativeEndian.PutUint16(stats[versionOffset:], 14)
	writeCounter(stats, cpuCountOffset, cpuTotalOffset, 4, 80)
	pid := make([]byte, 4)
	binary.NativeEndian.PutUint32(pid, 123)
	nested := append(netlinkTestAttribute(taskstatsTypePID, pid), netlinkTestAttribute(taskstatsTypeStats, stats)...)
	response := append(make([]byte, genlHeaderLength), netlinkTestAttribute(taskstatsTypeAggregate|0x8000, nested)...)

	got, err := decodeTaskstatsResponse(response, 123, true, true)
	if err != nil {
		t.Fatalf("decodeTaskstatsResponse returned error: %v", err)
	}
	if got.Version != 14 || got.CPU.Count != 4 || got.CPU.TotalNanoseconds != 80 {
		t.Fatalf("stats = %#v", got)
	}
}

func netlinkTestAttribute(typeID uint16, payload []byte) []byte {
	length := netlinkAttrLength + len(payload)
	attribute := make([]byte, align4(length))
	binary.NativeEndian.PutUint16(attribute[0:2], uint16(length))
	binary.NativeEndian.PutUint16(attribute[2:4], typeID)
	copy(attribute[4:], payload)
	return attribute
}
