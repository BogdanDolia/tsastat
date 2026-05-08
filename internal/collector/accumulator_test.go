package collector

import (
	"testing"
	"time"

	"tsastat/internal/model"
)

func TestAccumulatorAttributesPreviousState(t *testing.T) {
	acc := NewAccumulator()
	base := time.Unix(0, 0)

	if got := acc.Observe([]model.ThreadSample{sample(1, "worker", model.StateRunning, base)}); len(got) != 0 {
		t.Fatalf("first Observe returned %d stats, want 0", len(got))
	}

	got := acc.Observe([]model.ThreadSample{sample(1, "worker", model.StateSleeping, base.Add(time.Second))})
	if len(got) != 1 {
		t.Fatalf("second Observe returned %d stats, want 1", len(got))
	}
	if got[0].Duration(model.StateRunning) != time.Second {
		t.Fatalf("running duration = %s, want 1s", got[0].Duration(model.StateRunning))
	}

	got = acc.Observe([]model.ThreadSample{sample(1, "worker", model.StateSleeping, base.Add(2*time.Second))})
	if len(got) != 1 {
		t.Fatalf("third Observe returned %d stats, want 1", len(got))
	}
	if got[0].Duration(model.StateSleeping) != time.Second {
		t.Fatalf("sleeping duration = %s, want 1s", got[0].Duration(model.StateSleeping))
	}
}

func TestAccumulatorNewThreadAppearing(t *testing.T) {
	acc := NewAccumulator()
	base := time.Unix(0, 0)

	acc.Observe([]model.ThreadSample{sample(1, "one", model.StateRunning, base)})
	got := acc.Observe([]model.ThreadSample{
		sample(1, "one", model.StateSleeping, base.Add(time.Second)),
		sample(2, "two", model.StateRunning, base.Add(time.Second)),
	})

	if len(got) != 1 {
		t.Fatalf("Observe returned %d stats, want only existing thread", len(got))
	}
	if got[0].TID != 1 {
		t.Fatalf("stat TID = %d, want 1", got[0].TID)
	}

	got = acc.Observe([]model.ThreadSample{
		sample(1, "one", model.StateSleeping, base.Add(2*time.Second)),
		sample(2, "two", model.StateSleeping, base.Add(2*time.Second)),
	})
	if len(got) != 2 {
		t.Fatalf("Observe returned %d stats, want 2", len(got))
	}
}

func TestAccumulatorThreadDisappearing(t *testing.T) {
	acc := NewAccumulator()
	base := time.Unix(0, 0)

	acc.Observe([]model.ThreadSample{
		sample(1, "one", model.StateRunning, base),
		sample(2, "two", model.StateSleeping, base),
	})
	got := acc.Observe([]model.ThreadSample{
		sample(1, "one", model.StateSleeping, base.Add(time.Second)),
	})

	if len(got) != 2 {
		t.Fatalf("Observe returned %d stats, want 2 including disappearing thread", len(got))
	}
	if got[1].TID != 2 {
		t.Fatalf("second stat TID = %d, want 2", got[1].TID)
	}
	if got[1].Duration(model.StateSleeping) != time.Second {
		t.Fatalf("disappearing thread sleeping duration = %s, want 1s", got[1].Duration(model.StateSleeping))
	}
}

func TestAccumulatorMultipleTIDs(t *testing.T) {
	acc := NewAccumulator()
	base := time.Unix(0, 0)

	acc.Observe([]model.ThreadSample{
		sample(2, "two", model.StateSleeping, base),
		sample(1, "one", model.StateRunning, base),
	})
	got := acc.Observe([]model.ThreadSample{
		sample(2, "two", model.StateRunning, base.Add(time.Second)),
		sample(1, "one", model.StateSleeping, base.Add(time.Second)),
	})

	if len(got) != 2 {
		t.Fatalf("Observe returned %d stats, want 2", len(got))
	}
	if got[0].TID != 1 || got[1].TID != 2 {
		t.Fatalf("stats TIDs = %d,%d, want sorted 1,2", got[0].TID, got[1].TID)
	}
}

func sample(tid int, comm string, state model.ThreadState, ts time.Time) model.ThreadSample {
	return model.ThreadSample{
		PID:       99,
		TID:       tid,
		Comm:      comm,
		State:     state,
		Timestamp: ts,
	}
}
