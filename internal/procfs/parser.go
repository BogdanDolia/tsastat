package procfs

import (
	"fmt"
	"strconv"
	"strings"
)

func ParseProcStatLine(line string) (tid int, comm string, state byte, err error) {
	open := strings.IndexByte(line, '(')
	if open < 0 {
		return 0, "", 0, fmt.Errorf("malformed proc stat line: missing open parenthesis")
	}
	close := strings.LastIndexByte(line, ')')
	if close < 0 || close < open {
		return 0, "", 0, fmt.Errorf("malformed proc stat line: missing close parenthesis")
	}

	prefix := strings.TrimSpace(line[:open])
	if prefix == "" {
		return 0, "", 0, fmt.Errorf("malformed proc stat line: missing tid")
	}

	tid, err = strconv.Atoi(prefix)
	if err != nil {
		return 0, "", 0, fmt.Errorf("malformed proc stat line: invalid tid %q: %w", prefix, err)
	}

	comm = line[open+1 : close]
	rest := strings.TrimSpace(line[close+1:])
	if rest == "" {
		return 0, "", 0, fmt.Errorf("malformed proc stat line: missing state")
	}

	return tid, comm, rest[0], nil
}
