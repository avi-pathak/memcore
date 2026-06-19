package value

import "bytes"

// Set is an unordered collection of distinct binary-safe members. A small set is
// held in a compact packed encoding; it promotes to a map once it crosses its
// thresholds, and never demotes.
type Set struct {
	pack  []byte
	count int
	full  *fullSet
	max   Thresholds
}

// NewSet returns an empty set bounded by max.
func NewSet(max Thresholds) *Set { return &Set{max: max} }

// MakeSet returns a Value wrapping s.
func MakeSet(s *Set) Value { return Value{kind: KindSet, set: s} }

// Set returns the set payload. It must only be called when Kind() == KindSet.
func (v Value) Set() *Set {
	if v.kind != KindSet {
		panic("value: Set on kind " + v.kind.String())
	}
	return v.set
}

// Compact reports whether the set is still in its packed encoding.
func (s *Set) Compact() bool { return s.full == nil }

// Len reports the number of members.
func (s *Set) Len() int {
	if s.full != nil {
		return len(s.full.members)
	}
	return s.count
}

// Add inserts member and reports whether it was newly added.
func (s *Set) Add(member []byte) bool {
	if s.full != nil {
		return s.full.add(member)
	}
	if s.scanContains(member) {
		return false
	}
	s.pack = packAppend(s.pack, member)
	s.count++
	if s.max.exceeded(s.count, len(member)) {
		s.promote()
	}
	return true
}

// Remove deletes member and reports whether it was present.
func (s *Set) Remove(member []byte) bool {
	if s.full != nil {
		return s.full.remove(member)
	}
	found := false
	rebuilt := make([]byte, 0, len(s.pack))
	packEach(s.pack, func(e []byte) bool {
		if bytes.Equal(e, member) {
			found = true
			return true
		}
		rebuilt = packAppend(rebuilt, e)
		return true
	})
	if !found {
		return false
	}
	s.pack = rebuilt
	s.count--
	return true
}

// Contains reports whether member is in the set.
func (s *Set) Contains(member []byte) bool {
	if s.full != nil {
		return s.full.contains(member)
	}
	return s.scanContains(member)
}

// Members returns every member in unspecified order, each as a fresh copy.
func (s *Set) Members() [][]byte {
	if s.full != nil {
		return s.full.all()
	}
	out := make([][]byte, 0, s.count)
	packEach(s.pack, func(e []byte) bool {
		out = append(out, bytes.Clone(e))
		return true
	})
	return out
}

func (s *Set) scanContains(member []byte) bool {
	found := false
	packEach(s.pack, func(e []byte) bool {
		if bytes.Equal(e, member) {
			found = true
			return false
		}
		return true
	})
	return found
}

func (s *Set) promote() {
	full := &fullSet{members: make(map[string]struct{}, s.count)}
	packEach(s.pack, func(e []byte) bool {
		full.members[string(e)] = struct{}{}
		return true
	})
	s.full = full
	s.pack = nil
	s.count = 0
}

// fullSet is the promoted representation: a hash set of members.
type fullSet struct {
	members map[string]struct{}
}

func (s *fullSet) add(member []byte) bool {
	if _, ok := s.members[string(member)]; ok {
		return false
	}
	s.members[string(member)] = struct{}{}
	return true
}

func (s *fullSet) remove(member []byte) bool {
	if _, ok := s.members[string(member)]; !ok {
		return false
	}
	delete(s.members, string(member))
	return true
}

func (s *fullSet) contains(member []byte) bool {
	_, ok := s.members[string(member)]
	return ok
}

func (s *fullSet) all() [][]byte {
	out := make([][]byte, 0, len(s.members))
	for m := range s.members {
		out = append(out, []byte(m))
	}
	return out
}
