package procfs

import "testing"

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
			line:      "123 (bash) S 1 2 3",
			wantTID:   123,
			wantComm:  "bash",
			wantState: 'S',
		},
		{
			name:      "comm with space",
			line:      "123 (worker thread) R 1 2 3",
			wantTID:   123,
			wantComm:  "worker thread",
			wantState: 'R',
		},
		{
			name:      "comm with close bracket",
			line:      "123 (name with ) bracket) S 1 2 3",
			wantTID:   123,
			wantComm:  "name with ) bracket",
			wantState: 'S',
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotTID, gotComm, gotState, err := ParseProcStatLine(tt.line)
			if err != nil {
				t.Fatalf("ParseProcStatLine returned error: %v", err)
			}
			if gotTID != tt.wantTID || gotComm != tt.wantComm || gotState != tt.wantState {
				t.Fatalf("ParseProcStatLine() = (%d, %q, %q), want (%d, %q, %q)",
					gotTID, gotComm, gotState, tt.wantTID, tt.wantComm, tt.wantState)
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
	}

	for _, line := range tests {
		if _, _, _, err := ParseProcStatLine(line); err == nil {
			t.Fatalf("ParseProcStatLine(%q) returned nil error", line)
		}
	}
}
