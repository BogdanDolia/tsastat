package app

import (
	"io"
	"strings"
	"testing"
	"time"
)

func TestParseConfigUsesDefaultSampleInterval(t *testing.T) {
	cfg, err := parseConfig([]string{"-p", "123", "-i", "1s"}, io.Discard)
	if err != nil {
		t.Fatalf("parseConfig returned error: %v", err)
	}
	if cfg.Interval != time.Second {
		t.Fatalf("interval = %s, want 1s", cfg.Interval)
	}
	if cfg.SampleInterval != 10*time.Millisecond {
		t.Fatalf("sample interval = %s, want 10ms", cfg.SampleInterval)
	}
}

func TestParseConfigAcceptsCustomSampleInterval(t *testing.T) {
	cfg, err := parseConfig([]string{"-p", "123", "--interval", "2s", "--sample", "25ms"}, io.Discard)
	if err != nil {
		t.Fatalf("parseConfig returned error: %v", err)
	}
	if cfg.SampleInterval != 25*time.Millisecond {
		t.Fatalf("sample interval = %s, want 25ms", cfg.SampleInterval)
	}
}

func TestParseConfigRejectsSampleIntervalNotShorterThanReport(t *testing.T) {
	_, err := parseConfig([]string{"-p", "123", "--interval", "1s", "--sample", "1s"}, io.Discard)
	if err == nil {
		t.Fatal("parseConfig returned nil error")
	}
	if !strings.Contains(err.Error(), "must be shorter") {
		t.Fatalf("error = %q, want shorter-than validation", err)
	}
}
