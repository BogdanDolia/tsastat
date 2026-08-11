package procfs

import (
	"fmt"
	"testing"
)

func TestParseProcStatLine(t *testing.T) {
	tests := []struct {
		name      string
		line      string
		wantTID   int
		wantComm  string
		wantState byte
	}{
		{
			name:      "simple",
			line:      validStatLine(123, "bash", 'S', 4242),
			wantTID:   123,
			wantComm:  "bash",
			wantState: 'S',
		},
		{
			name:      "comm with space",
			line:      validStatLine(123, "worker thread", 'R', 4242),
			wantTID:   123,
			wantComm:  "worker thread",
			wantState: 'R',
		},
		{
			name:      "comm with close bracket",
			line:      validStatLine(123, "name with ) bracket", 'S', 4242),
			wantTID:   123,
			wantComm:  "name with ) bracket",
			wantState: 'S',
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseProcStatLine(tt.line)
			if err != nil {
				t.Fatalf("ParseProcStatLine returned error: %v", err)
			}
			if got.TID != tt.wantTID || got.Comm != tt.wantComm || got.State != tt.wantState || got.StartTimeTicks != 4242 {
				t.Fatalf("ParseProcStatLine() = %#v, want tid=%d comm=%q state=%q starttime=4242",
					got, tt.wantTID, tt.wantComm, tt.wantState)
			}
		})
	}
}

func TestParseProcStatLineMalformed(t *testing.T) {
	tests := []string{
		"",
		"123 bash S 1 2 3",
		"abc (bash) S 1 2 3",
		"123 (bash)",
		"(bash) S 1 2 3",
		"123 (bash) S 1 2 3",
		validStatLine(123, "bash", 'S', 4242)[:len(validStatLine(123, "bash", 'S', 4242))-4] + "nope",
	}

	for _, line := range tests {
		if _, err := ParseProcStatLine(line); err == nil {
			t.Fatalf("ParseProcStatLine(%q) returned nil error", line)
		}
	}
}

func TestParseSchedstatLine(t *testing.T) {
	got, err := ParseSchedstatLine("123456789 987654321 42\n")
	if err != nil {
		t.Fatalf("ParseSchedstatLine returned error: %v", err)
	}
	if got.OnCPUNanoseconds != 123456789 || got.RunqueueNanoseconds != 987654321 || got.Timeslices != 42 {
		t.Fatalf("ParseSchedstatLine() = %#v", got)
	}
}

func TestParseSchedstatLineMalformed(t *testing.T) {
	for _, line := range []string{"", "1 2", "one 2 3", "1 two 3", "1 2 three"} {
		if _, err := ParseSchedstatLine(line); err == nil {
			t.Fatalf("ParseSchedstatLine(%q) returned nil error", line)
		}
	}
}

func validStatLine(tid int, comm string, state byte, startTime uint64) string {
	return fmt.Sprintf("%d (%s) %c 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 %d", tid, comm, state, startTime)
}
