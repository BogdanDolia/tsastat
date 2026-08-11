package ebpf

import (
	"encoding/binary"
	"math"
	"strconv"
	"strings"
)

const (
	atClockTicks         = 17
	nanosecondsPerSecond = uint64(1_000_000_000)
)

// Linux exposes task start time in /proc/<pid>/stat by applying the reader's
// time-namespace boottime offset to task->start_boottime and converting the
// result to USER_HZ ticks. AT_CLKTCK supplies that tick rate to userspace.
func loadIdentityConversion(readFile func(string) ([]byte, error), wordSize int) (int64, uint64, bool) {
	offsetData, err := readFile("/proc/self/timens_offsets")
	if err != nil {
		return 0, 0, false
	}
	offset, ok := parseBoottimeOffset(offsetData)
	if !ok {
		return 0, 0, false
	}
	auxv, err := readFile("/proc/self/auxv")
	if err != nil {
		return 0, 0, false
	}
	ticks, ok := parseClockTicks(auxv, wordSize)
	if !ok || ticks == 0 || nanosecondsPerSecond%ticks != 0 {
		return 0, 0, false
	}
	return offset, ticks, true
}

func parseBoottimeOffset(data []byte) (int64, bool) {
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 3 || fields[0] != "boottime" {
			continue
		}
		seconds, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			return 0, false
		}
		nanoseconds, err := strconv.ParseInt(fields[2], 10, 64)
		if err != nil || nanoseconds < 0 || nanoseconds >= int64(nanosecondsPerSecond) {
			return 0, false
		}
		if seconds > math.MaxInt64/int64(nanosecondsPerSecond) || seconds < math.MinInt64/int64(nanosecondsPerSecond) {
			return 0, false
		}
		offset := seconds * int64(nanosecondsPerSecond)
		if nanoseconds > 0 && offset > math.MaxInt64-nanoseconds {
			return 0, false
		}
		return offset + nanoseconds, true
	}
	return 0, false
}

func parseClockTicks(auxv []byte, wordSize int) (uint64, bool) {
	if wordSize != 4 && wordSize != 8 {
		return 0, false
	}
	pairSize := wordSize * 2
	for offset := 0; offset+pairSize <= len(auxv); offset += pairSize {
		tag := nativeWord(auxv[offset:offset+wordSize], wordSize)
		value := nativeWord(auxv[offset+wordSize:offset+pairSize], wordSize)
		if tag == 0 {
			break
		}
		if tag == atClockTicks {
			return value, value != 0
		}
	}
	return 0, false
}

func nativeWord(value []byte, wordSize int) uint64 {
	if wordSize == 4 {
		return uint64(binary.NativeEndian.Uint32(value))
	}
	return binary.NativeEndian.Uint64(value)
}

func procStartTimeTicks(startBoottimeNS uint64, boottimeOffsetNS int64, ticksPerSecond uint64) (uint64, bool) {
	if ticksPerSecond == 0 || nanosecondsPerSecond%ticksPerSecond != 0 {
		return 0, false
	}
	adjusted := startBoottimeNS
	if boottimeOffsetNS < 0 {
		delta := uint64(-(boottimeOffsetNS + 1)) + 1
		if adjusted < delta {
			return 0, false
		}
		adjusted -= delta
	} else {
		delta := uint64(boottimeOffsetNS)
		if adjusted > math.MaxUint64-delta {
			return 0, false
		}
		adjusted += delta
	}
	return adjusted / (nanosecondsPerSecond / ticksPerSecond), true
}
