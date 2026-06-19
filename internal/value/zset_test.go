package value

import (
	"fmt"
	"testing"
)

func assertZMembers(t *testing.T, got []ZMember, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d members, want %d", len(got), len(want))
	}
	for i := range want {
		if string(got[i].Member) != want[i] {
			t.Fatalf("member %d = %q, want %q", i, got[i].Member, want[i])
		}
	}
}

func TestZSetAddReportsNewMembers(t *testing.T) {
	z := NewZSet(bigThresholds)
	if !z.Add([]byte("a"), 1) {
		t.Fatal("Add of a new member reported it as existing")
	}
	if z.Add([]byte("a"), 2) {
		t.Fatal("Add updating a member reported it as new")
	}
}

func TestZSetScoreReflectsUpdates(t *testing.T) {
	z := NewZSet(bigThresholds)
	z.Add([]byte("a"), 1)
	z.Add([]byte("a"), 5)
	if s, ok := z.Score([]byte("a")); !ok || s != 5 {
		t.Fatalf("Score = (%v, %v), want (5, true)", s, ok)
	}
}

func TestZSetRangeIsOrderedByScore(t *testing.T) {
	z := NewZSet(bigThresholds)
	z.Add([]byte("c"), 3)
	z.Add([]byte("a"), 1)
	z.Add([]byte("b"), 2)
	assertZMembers(t, z.Range(0, -1), []string{"a", "b", "c"})
}

func TestZSetBreaksScoreTiesByMember(t *testing.T) {
	z := NewZSet(bigThresholds)
	z.Add([]byte("b"), 1)
	z.Add([]byte("a"), 1)
	z.Add([]byte("c"), 1)
	assertZMembers(t, z.Range(0, -1), []string{"a", "b", "c"})
}

func TestZSetRangeReflectsAScoreUpdate(t *testing.T) {
	z := NewZSet(bigThresholds)
	z.Add([]byte("a"), 1)
	z.Add([]byte("b"), 2)
	z.Add([]byte("a"), 3) // a now outranks b
	assertZMembers(t, z.Range(0, -1), []string{"b", "a"})
}

func TestZSetPromotesPastItsThreshold(t *testing.T) {
	z := NewZSet(Thresholds{MaxEntries: 3, MaxBytes: 1 << 20})
	z.Add([]byte("a"), 1)
	z.Add([]byte("b"), 2)
	z.Add([]byte("c"), 3)
	if !z.Compact() {
		t.Fatal("a sorted set at its threshold should be compact")
	}
	z.Add([]byte("d"), 4) // crosses MaxEntries
	if z.Compact() {
		t.Fatal("a sorted set past its threshold should have promoted")
	}
	assertZMembers(t, z.Range(0, -1), []string{"a", "b", "c", "d"})
}

func TestZSetStaysSortedAcrossManyInsertions(t *testing.T) {
	// Small thresholds force promotion, so this exercises the skip list.
	z := NewZSet(smallThresholds)
	for i := 200; i > 0; i-- { // insert in descending score order
		z.Add([]byte(fmt.Sprintf("m%03d", i)), float64(i))
	}
	got := z.Range(0, -1)
	if len(got) != 200 {
		t.Fatalf("Range returned %d members, want 200", len(got))
	}
	for i := 1; i < len(got); i++ {
		if got[i-1].Score > got[i].Score {
			t.Fatalf("members out of order at index %d: %v before %v", i, got[i-1].Score, got[i].Score)
		}
	}
}
