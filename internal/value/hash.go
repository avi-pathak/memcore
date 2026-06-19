package value

import "bytes"

// Hash is a map from field to a binary-safe value. A small hash is held in a
// compact packed encoding of alternating field and value elements; it promotes
// to a map once it crosses its thresholds, and never demotes. A Hash owns the
// bytes stored in it.
type Hash struct {
	pack  []byte
	count int
	full  *fullHash
	max   Thresholds
}

// NewHash returns an empty hash bounded by max.
func NewHash(max Thresholds) *Hash { return &Hash{max: max} }

// MakeHash returns a Value wrapping h.
func MakeHash(h *Hash) Value { return Value{kind: KindHash, hash: h} }

// Hash returns the hash payload. It must only be called when Kind() == KindHash.
func (v Value) Hash() *Hash {
	if v.kind != KindHash {
		panic("value: Hash on kind " + v.kind.String())
	}
	return v.hash
}

// Compact reports whether the hash is still in its packed encoding.
func (h *Hash) Compact() bool { return h.full == nil }

// Len reports the number of fields.
func (h *Hash) Len() int {
	if h.full != nil {
		return len(h.full.fields)
	}
	return h.count
}

// Get returns the value stored under field.
func (h *Hash) Get(field string) ([]byte, bool) {
	if h.full != nil {
		b, ok := h.full.fields[field]
		return b, ok
	}
	var out []byte
	found := false
	scanPairs(h.pack, func(f, v []byte) bool {
		if string(f) == field {
			out, found = bytes.Clone(v), true
			return false
		}
		return true
	})
	return out, found
}

// Set stores a copy of value under field and reports whether the field is new.
func (h *Hash) Set(field string, val []byte) bool {
	if h.full != nil {
		_, existed := h.full.fields[field]
		h.full.fields[field] = bytes.Clone(val)
		return !existed
	}
	existed := false
	rebuilt := make([]byte, 0, len(h.pack))
	scanPairs(h.pack, func(f, v []byte) bool {
		rebuilt = packAppend(rebuilt, f)
		if string(f) == field {
			existed = true
			rebuilt = packAppend(rebuilt, val)
		} else {
			rebuilt = packAppend(rebuilt, v)
		}
		return true
	})
	if !existed {
		rebuilt = packAppend(packAppend(rebuilt, []byte(field)), val)
		h.count++
	}
	h.pack = rebuilt
	h.promoteIfNeeded(len(field), len(val))
	return !existed
}

// Delete removes field and reports whether it was present.
func (h *Hash) Delete(field string) bool {
	if h.full != nil {
		_, ok := h.full.fields[field]
		delete(h.full.fields, field)
		return ok
	}
	found := false
	rebuilt := make([]byte, 0, len(h.pack))
	scanPairs(h.pack, func(f, v []byte) bool {
		if string(f) == field {
			found = true
			return true
		}
		rebuilt = packAppend(packAppend(rebuilt, f), v)
		return true
	})
	if !found {
		return false
	}
	h.pack = rebuilt
	h.count--
	return true
}

// Pairs returns the field/value pairs in unspecified order as a flat slice
// [field0, value0, field1, value1, ...].
func (h *Hash) Pairs() [][]byte {
	if h.full != nil {
		out := make([][]byte, 0, len(h.full.fields)*2)
		for f, v := range h.full.fields {
			out = append(out, []byte(f), v)
		}
		return out
	}
	out := make([][]byte, 0, h.count*2)
	scanPairs(h.pack, func(f, v []byte) bool {
		out = append(out, bytes.Clone(f), bytes.Clone(v))
		return true
	})
	return out
}

func (h *Hash) promoteIfNeeded(fieldSize, valSize int) {
	big := fieldSize
	if valSize > big {
		big = valSize
	}
	if h.max.exceeded(h.count, big) {
		h.promote()
	}
}

func (h *Hash) promote() {
	full := &fullHash{fields: make(map[string][]byte, h.count)}
	scanPairs(h.pack, func(f, v []byte) bool {
		full.fields[string(f)] = bytes.Clone(v)
		return true
	})
	h.full = full
	h.pack = nil
	h.count = 0
}

// scanPairs walks a pack of alternating field and value elements, calling fn for
// each pair. The slices alias the pack.
func scanPairs(buf []byte, fn func(field, value []byte) bool) {
	var field []byte
	haveField := false
	packEach(buf, func(e []byte) bool {
		if !haveField {
			field, haveField = e, true
			return true
		}
		haveField = false
		return fn(field, e)
	})
}

// fullHash is the promoted representation: a map whose values are immutable once
// stored.
type fullHash struct {
	fields map[string][]byte
}
