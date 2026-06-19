package value

import "bytes"

// Hash is the full representation of a Redis hash: a map from field to a
// binary-safe value. It owns the bytes stored in it. The compact encoding for
// small hashes is a separate type that promotes to this one.
type Hash struct {
	fields map[string][]byte
}

// NewHash returns an empty hash.
func NewHash() *Hash { return &Hash{fields: make(map[string][]byte)} }

// MakeHash returns a Value wrapping h.
func MakeHash(h *Hash) Value { return Value{kind: KindHash, hash: h} }

// Hash returns the hash payload. It must only be called when Kind() == KindHash.
func (v Value) Hash() *Hash {
	if v.kind != KindHash {
		panic("value: Hash on kind " + v.kind.String())
	}
	return v.hash
}

// Len reports the number of fields.
func (h *Hash) Len() int { return len(h.fields) }

// Get returns the value stored under field. The returned slice is immutable.
func (h *Hash) Get(field string) ([]byte, bool) {
	b, ok := h.fields[field]
	return b, ok
}

// Set stores a copy of value under field and reports whether the field is new.
func (h *Hash) Set(field string, value []byte) bool {
	_, existed := h.fields[field]
	h.fields[field] = bytes.Clone(value)
	return !existed
}

// Delete removes field and reports whether it was present.
func (h *Hash) Delete(field string) bool {
	_, ok := h.fields[field]
	delete(h.fields, field)
	return ok
}

// Pairs returns the field/value pairs in unspecified order as a flat slice
// [field0, value0, field1, value1, ...]. The value slices alias the stored
// bytes, which are immutable; the field slices are fresh copies.
func (h *Hash) Pairs() [][]byte {
	out := make([][]byte, 0, len(h.fields)*2)
	for f, v := range h.fields {
		out = append(out, []byte(f), v)
	}
	return out
}
