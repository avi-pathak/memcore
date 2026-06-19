package value

// Set is the full representation of a Redis set: an unordered collection of
// distinct binary-safe members. The compact encoding for small sets is a
// separate type that promotes to this one.
type Set struct {
	members map[string]struct{}
}

// NewSet returns an empty set.
func NewSet() *Set { return &Set{members: make(map[string]struct{})} }

// MakeSet returns a Value wrapping s.
func MakeSet(s *Set) Value { return Value{kind: KindSet, set: s} }

// Set returns the set payload. It must only be called when Kind() == KindSet.
func (v Value) Set() *Set {
	if v.kind != KindSet {
		panic("value: Set on kind " + v.kind.String())
	}
	return v.set
}

// Len reports the number of members.
func (s *Set) Len() int { return len(s.members) }

// Add inserts member and reports whether it was newly added.
func (s *Set) Add(member []byte) bool {
	if _, ok := s.members[string(member)]; ok {
		return false
	}
	s.members[string(member)] = struct{}{}
	return true
}

// Remove deletes member and reports whether it was present.
func (s *Set) Remove(member []byte) bool {
	if _, ok := s.members[string(member)]; !ok {
		return false
	}
	delete(s.members, string(member))
	return true
}

// Contains reports whether member is in the set.
func (s *Set) Contains(member []byte) bool {
	_, ok := s.members[string(member)]
	return ok
}

// Members returns every member in unspecified order, each as a fresh copy.
func (s *Set) Members() [][]byte {
	out := make([][]byte, 0, len(s.members))
	for m := range s.members {
		out = append(out, []byte(m))
	}
	return out
}
