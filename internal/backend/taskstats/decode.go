package taskstats

import (
	"encoding/binary"
	"fmt"

	"github.com/BogdanDolia/tsastat/internal/model"
)

const (
	versionOffset = 0

	cpuCountOffset   = 16
	cpuTotalOffset   = 24
	blkioCountOffset = 32
	blkioTotalOffset = 40
	swapCountOffset  = 48
	swapTotalOffset  = 56

	freepagesCountOffset = 312
	freepagesTotalOffset = 320
	thrashingCountOffset = 328
	thrashingTotalOffset = 336
	compactCountOffset   = 352
	compactTotalOffset   = 360
	wpcopyCountOffset    = 400
	wpcopyTotalOffset    = 408
	irqCountOffset       = 416
	irqTotalOffset       = 424

	uint64Size              = 8
	minimumTaskstatsPayload = swapTotalOffset + uint64Size
)

func decodeDelayCounters(payload []byte, accountingEnabled, accountingEnabledKnown bool) (model.DelayCounters, error) {
	if len(payload) < minimumTaskstatsPayload {
		return model.DelayCounters{}, fmt.Errorf("taskstats payload is %d bytes, want at least %d", len(payload), minimumTaskstatsPayload)
	}

	version := binary.NativeEndian.Uint16(payload[versionOffset : versionOffset+2])
	if version == 0 {
		return model.DelayCounters{}, fmt.Errorf("taskstats payload reports invalid version 0")
	}
	if version == 15 {
		return model.DelayCounters{}, fmt.Errorf("taskstats version 15 has an incompatible field layout; upgrade to a kernel exposing version 16 or newer")
	}

	return model.DelayCounters{
		Available:              true,
		Version:                version,
		AccountingEnabled:      accountingEnabled,
		AccountingEnabledKnown: accountingEnabledKnown,
		CPU:                    decodeVersionedDelayCounter(payload, version, 1, cpuCountOffset, cpuTotalOffset),
		BlockIO:                decodeVersionedDelayCounter(payload, version, 1, blkioCountOffset, blkioTotalOffset),
		SwapIn:                 decodeVersionedDelayCounter(payload, version, 1, swapCountOffset, swapTotalOffset),
		Reclaim:                decodeVersionedDelayCounter(payload, version, 7, freepagesCountOffset, freepagesTotalOffset),
		Thrashing:              decodeVersionedDelayCounter(payload, version, 9, thrashingCountOffset, thrashingTotalOffset),
		Compaction:             decodeVersionedDelayCounter(payload, version, 11, compactCountOffset, compactTotalOffset),
		WriteProtectCopy:       decodeVersionedDelayCounter(payload, version, 13, wpcopyCountOffset, wpcopyTotalOffset),
		IRQ:                    decodeVersionedDelayCounter(payload, version, 14, irqCountOffset, irqTotalOffset),
	}, nil
}

func decodeVersionedDelayCounter(payload []byte, version, minimumVersion uint16, countOffset, totalOffset int) model.DelayCounter {
	if version < minimumVersion {
		return model.DelayCounter{}
	}
	return decodeDelayCounter(payload, countOffset, totalOffset)
}

func decodeDelayCounter(payload []byte, countOffset, totalOffset int) model.DelayCounter {
	if countOffset < 0 || totalOffset < 0 || countOffset+uint64Size > len(payload) || totalOffset+uint64Size > len(payload) {
		return model.DelayCounter{}
	}
	return model.DelayCounter{
		Available:        true,
		Count:            binary.NativeEndian.Uint64(payload[countOffset : countOffset+uint64Size]),
		TotalNanoseconds: binary.NativeEndian.Uint64(payload[totalOffset : totalOffset+uint64Size]),
	}
}
