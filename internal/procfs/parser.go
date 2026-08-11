package procfs

import (
	"fmt"
	"strconv"
	"strings"
)

type TaskStat struct {
	TID            int
	Comm           string
	State          byte
	StartTimeTicks uint64
}

type Schedstat struct {
	OnCPUNanoseconds    uint64
	RunqueueNanoseconds uint64
	Timeslices          uint64
}

func ParseProcStatLine(line string) (TaskStat, error) {
	open := strings.IndexByte(line, '(')
	if open < 0 {
		return TaskStat{}, fmt.Errorf("malformed proc stat line: missing open parenthesis")
	}
	close := strings.LastIndexByte(line, ')')
	if close < 0 || close < open {
		return TaskStat{}, fmt.Errorf("malformed proc stat line: missing close parenthesis")
	}

	prefix := strings.TrimSpace(line[:open])
	if prefix == "" {
		return TaskStat{}, fmt.Errorf("malformed proc stat line: missing tid")
	}

	tid, err := strconv.Atoi(prefix)
	if err != nil {
		return TaskStat{}, fmt.Errorf("malformed proc stat line: invalid tid %q: %w", prefix, err)
	}

	comm := line[open+1 : close]
	fields := strings.Fields(line[close+1:])
	if len(fields) == 0 {
		return TaskStat{}, fmt.Errorf("malformed proc stat line: missing state")
	}
	if len(fields) < 20 {
		return TaskStat{}, fmt.Errorf("malformed proc stat line: missing starttime field")
	}
	if len(fields[0]) != 1 {
		return TaskStat{}, fmt.Errorf("malformed proc stat line: invalid state %q", fields[0])
	}

	startTimeTicks, err := strconv.ParseUint(fields[19], 10, 64)
	if err != nil {
		return TaskStat{}, fmt.Errorf("malformed proc stat line: invalid starttime %q: %w", fields[19], err)
	}

	return TaskStat{
		TID:            tid,
		Comm:           comm,
		State:          fields[0][0],
		StartTimeTicks: startTimeTicks,
	}, nil
}

func ParseSchedstatLine(line string) (Schedstat, error) {
	fields := strings.Fields(line)
	if len(fields) < 3 {
		return Schedstat{}, fmt.Errorf("malformed schedstat line: got %d fields, want at least 3", len(fields))
	}

	values := make([]uint64, 3)
	for index := range values {
		value, err := strconv.ParseUint(fields[index], 10, 64)
		if err != nil {
			return Schedstat{}, fmt.Errorf("malformed schedstat line: invalid field %d %q: %w", index+1, fields[index], err)
		}
		values[index] = value
	}

	return Schedstat{
		OnCPUNanoseconds:    values[0],
		RunqueueNanoseconds: values[1],
		Timeslices:          values[2],
	}, nil
}
