package clock

import (
	"testing"
	"time"
)

func TestManualClockReportsItsInitialInstant(t *testing.T) {
	start := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	c := NewManualClock(start)
	if got := c.Now(); !got.Equal(start) {
		t.Fatalf("Now() = %v, want %v", got, start)
	}
}

func TestManualClockAdvancesByTheGivenDuration(t *testing.T) {
	start := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	c := NewManualClock(start)
	c.Advance(90 * time.Minute)
	if got, want := c.Now(), start.Add(90*time.Minute); !got.Equal(want) {
		t.Fatalf("Now() = %v, want %v", got, want)
	}
}

func TestManualClockSetReplacesTheCurrentInstant(t *testing.T) {
	c := NewManualClock(time.Unix(0, 0))
	target := time.Date(2030, time.June, 1, 12, 0, 0, 0, time.UTC)
	c.Set(target)
	if got := c.Now(); !got.Equal(target) {
		t.Fatalf("Now() = %v, want %v", got, target)
	}
}

func TestSystemClockTracksTheWallClock(t *testing.T) {
	before := time.Now()
	got := SystemClock{}.Now()
	after := time.Now()
	if got.Before(before) || got.After(after) {
		t.Fatalf("Now() = %v, want within [%v, %v]", got, before, after)
	}
}
