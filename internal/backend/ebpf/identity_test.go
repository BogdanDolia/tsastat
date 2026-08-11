package ebpf

import (
	"encoding/binary"
	"testing"
)

func TestParseBoottimeOffset(t *testing.T) {
	offset, ok := parseBoottimeOffset([]byte("monotonic 0 0\nboottime -2 500000000\n"))
	if !ok || offset != -1_500_000_000 {
		t.Fatalf("offset = %d, ok=%t", offset, ok)
	}
	if _, ok := parseBoottimeOffset([]byte("boottime 0 1000000000\n")); ok {
		t.Fatal("accepted non-normalized nanoseconds")
	}
}

func TestParseClockTicks(t *testing.T) {
	auxv := make([]byte, 4*16)
	binary.NativeEndian.PutUint64(auxv[0:8], 6)
	binary.NativeEndian.PutUint64(auxv[8:16], 4096)
	binary.NativeEndian.PutUint64(auxv[16:24], atClockTicks)
	binary.NativeEndian.PutUint64(auxv[24:32], 100)

	ticks, ok := parseClockTicks(auxv, 8)
	if !ok || ticks != 100 {
		t.Fatalf("ticks = %d, ok=%t", ticks, ok)
	}
}

func TestProcStartTimeTicksUsesTimeNamespaceOffset(t *testing.T) {
	ticks, ok := procStartTimeTicks(5_000_000_000, -1_000_000_000, 100)
	if !ok || ticks != 400 {
		t.Fatalf("ticks = %d, ok=%t", ticks, ok)
	}
	if _, ok := procStartTimeTicks(1, -2, 100); ok {
		t.Fatal("accepted negative adjusted start time")
	}
}
