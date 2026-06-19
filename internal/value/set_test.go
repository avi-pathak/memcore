package value

import (
	"sort"
	"testing"
)

func TestSetAddReportsNewMembers(t *testing.T) {
	s := NewSet(bigThresholds)
	if !s.Add([]byte("m")) {
		t.Fatal("Add of a new member reported it as existing")
	}
	if s.Add([]byte("m")) {
		t.Fatal("Add of an existing member reported it as new")
	}
	if s.Len() != 1 {
		t.Fatalf("Len = %d, want 1", s.Len())
	}
}

func TestSetRemoveAndContains(t *testing.T) {
	s := NewSet(bigThresholds)
	s.Add([]byte("m"))
	if !s.Contains([]byte("m")) {
		t.Fatal("Contains reported a present member absent")
	}
	if !s.Remove([]byte("m")) {
		t.Fatal("Remove reported a present member absent")
	}
	if s.Contains([]byte("m")) {
		t.Fatal("Contains reported a removed member present")
	}
	if s.Remove([]byte("m")) {
		t.Fatal("Remove reported a removed member present")
	}
}

func TestSetMembersReturnsEveryMember(t *testing.T) {
	s := NewSet(bigThresholds)
	for _, m := range []string{"a", "b", "c"} {
		s.Add([]byte(m))
	}
	members := s.Members()
	got := make([]string, len(members))
	for i, m := range members {
		got[i] = string(m)
	}
	sort.Strings(got)
	if len(got) != 3 || got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Fatalf("Members = %v, want [a b c]", got)
	}
}

func TestSetPromotesAndStaysConsistent(t *testing.T) {
	s := NewSet(Thresholds{MaxEntries: 2, MaxBytes: 1 << 20})
	s.Add([]byte("a"))
	s.Add([]byte("b"))
	if !s.Compact() {
		t.Fatal("a set at its threshold should be compact")
	}
	s.Add([]byte("c")) // crosses MaxEntries
	if s.Compact() {
		t.Fatal("a set past its threshold should have promoted")
	}
	if !s.Contains([]byte("b")) {
		t.Fatal("Contains after promotion lost a member")
	}
	if s.Add([]byte("a")) {
		t.Fatal("Add of an existing member reported new after promotion")
	}
	if s.Len() != 3 {
		t.Fatalf("Len = %d, want 3 after promotion", s.Len())
	}
}
