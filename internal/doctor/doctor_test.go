package doctor

import "testing"

func TestParseKernelVersion(t *testing.T) {
	tests := []struct {
		release string
		major   int
		minor   int
		ok      bool
	}{
		{release: "6.1.0-27-amd64", major: 6, minor: 1, ok: true},
		{release: "6.12.8", major: 6, minor: 12, ok: true},
		{release: "5.15.0", major: 5, minor: 15, ok: true},
		{release: "invalid", ok: false},
	}
	for _, test := range tests {
		major, minor, ok := parseKernelVersion(test.release)
		if major != test.major || minor != test.minor || ok != test.ok {
			t.Fatalf("parseKernelVersion(%q) = %d,%d,%t, want %d,%d,%t",
				test.release, major, minor, ok, test.major, test.minor, test.ok)
		}
	}
}
