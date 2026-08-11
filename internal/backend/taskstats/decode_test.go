package taskstats

import (
	"encoding/binary"
	"testing"
)

func TestDecodeDelayCountersIsVersionAndLengthAware(t *testing.T) {
	payload := make([]byte, irqTotalOffset+uint64Size)
	binary.NativeEndian.PutUint16(payload[versionOffset:], 14)
	writeCounter(payload, cpuCountOffset, cpuTotalOffset, 2, 20)
	writeCounter(payload, blkioCountOffset, blkioTotalOffset, 3, 30)
	writeCounter(payload, swapCountOffset, swapTotalOffset, 4, 40)
	writeCounter(payload, freepagesCountOffset, freepagesTotalOffset, 5, 50)
	writeCounter(payload, thrashingCountOffset, thrashingTotalOffset, 6, 60)
	writeCounter(payload, compactCountOffset, compactTotalOffset, 7, 70)
	writeCounter(payload, wpcopyCountOffset, wpcopyTotalOffset, 8, 80)
	writeCounter(payload, irqCountOffset, irqTotalOffset, 9, 90)

	got, err := decodeDelayCounters(payload, true, true)
	if err != nil {
		t.Fatalf("decodeDelayCounters returned error: %v", err)
	}
	if !got.Available || got.Version != 14 || !got.AccountingEnabled || !got.AccountingEnabledKnown {
		t.Fatalf("metadata = %#v", got)
	}
	if got.CPU.Count != 2 || got.CPU.TotalNanoseconds != 20 || got.IRQ.Count != 9 || got.IRQ.TotalNanoseconds != 90 {
		t.Fatalf("decoded counters = %#v", got)
	}
}

func TestDecodeDelayCountersMarksNewerFieldsUnavailableInShortPayload(t *testing.T) {
	payload := make([]byte, freepagesTotalOffset+uint64Size)
	binary.NativeEndian.PutUint16(payload[versionOffset:], 9)
	writeCounter(payload, freepagesCountOffset, freepagesTotalOffset, 5, 50)

	got, err := decodeDelayCounters(payload, false, true)
	if err != nil {
		t.Fatalf("decodeDelayCounters returned error: %v", err)
	}
	if !got.Reclaim.Available || got.Thrashing.Available || got.Compaction.Available || got.WriteProtectCopy.Available || got.IRQ.Available {
		t.Fatalf("field availability = %#v", got)
	}
}

func TestDecodeDelayCountersUsesVersionFeatureGates(t *testing.T) {
	payload := make([]byte, irqTotalOffset+uint64Size)
	binary.NativeEndian.PutUint16(payload[versionOffset:], 8)
	writeCounter(payload, freepagesCountOffset, freepagesTotalOffset, 5, 50)
	writeCounter(payload, thrashingCountOffset, thrashingTotalOffset, 6, 60)

	got, err := decodeDelayCounters(payload, true, true)
	if err != nil {
		t.Fatalf("decodeDelayCounters returned error: %v", err)
	}
	if !got.Reclaim.Available || got.Thrashing.Available || got.Compaction.Available || got.WriteProtectCopy.Available || got.IRQ.Available {
		t.Fatalf("version-gated availability = %#v", got)
	}
}

func TestDecodeDelayCountersRejectsTruncatedOrInvalidPayload(t *testing.T) {
	if _, err := decodeDelayCounters(make([]byte, minimumTaskstatsPayload-1), false, false); err == nil {
		t.Fatal("truncated payload returned nil error")
	}
	payload := make([]byte, minimumTaskstatsPayload)
	if _, err := decodeDelayCounters(payload, false, false); err == nil {
		t.Fatal("version zero returned nil error")
	}
	binary.NativeEndian.PutUint16(payload[versionOffset:], 15)
	if _, err := decodeDelayCounters(payload, false, false); err == nil {
		t.Fatal("incompatible version 15 returned nil error")
	}
}

func writeCounter(payload []byte, countOffset, totalOffset int, count, total uint64) {
	binary.NativeEndian.PutUint64(payload[countOffset:], count)
	binary.NativeEndian.PutUint64(payload[totalOffset:], total)
}
