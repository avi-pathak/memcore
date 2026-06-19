package value

import (
	"sort"
	"testing"
)

func TestSetAddReportsNewMembers(t *testing.T) {
	s := NewSet()
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
	s := NewSet()
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
	s := NewSet()
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
