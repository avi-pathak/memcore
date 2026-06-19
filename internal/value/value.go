// Package value defines Memcore's data values as a tagged union. A Value holds
// exactly one payload, selected by its Kind; callers switch on Kind rather than
// type-asserting an interface, so the hot path stays free of boxing.
package value

// Kind identifies the payload a Value carries.
type Kind uint8

const (
	// KindNone is the kind of the zero Value: it carries no payload.
	KindNone Kind = iota
	// KindString is a binary-safe byte string.
	KindString
	// KindList is an ordered sequence of binary-safe values.
	KindList
	// KindHash is a map from field to binary-safe value.
	KindHash
	// KindSet is an unordered collection of distinct binary-safe members.
	KindSet
	// KindZSet is a set of members ordered by an associated score.
	KindZSet
)

// String returns the lowercase type name, matching what the TYPE command
// reports to clients.
func (k Kind) String() string {
	switch k {
	case KindNone:
		return "none"
	case KindString:
		return "string"
	case KindList:
		return "list"
	case KindHash:
		return "hash"
	case KindSet:
		return "set"
	case KindZSet:
		return "zset"
	default:
		return "unknown"
	}
}

// Value is an immutable tagged union over Memcore's data types. Exactly one
// payload field is live, selected by kind; the zero Value has kind None.
//
// String payload bytes are treated as immutable once a Value is constructed,
// which lets a reader hold a reference to them and serialize a reply after
// releasing the shard lock. Collection payloads are pointers mutated in place
// under the shard's exclusive lock.
type Value struct {
	kind Kind
	str  []byte
	list *List
	hash *Hash
	set  *Set
	zset *ZSet
}

// Kind returns the value's discriminator.
func (v Value) Kind() Kind { return v.kind }

// normalizeRange resolves Redis-style start and stop indices against a
// collection of the given length. Negative indices count back from the end, and
// the range is clamped to the collection's bounds. It returns the inclusive
// [lo, hi] bounds and whether the range selects anything.
func normalizeRange(start, stop, length int) (lo, hi int, ok bool) {
	if length == 0 {
		return 0, 0, false
	}
	if start < 0 {
		start = length + start
	}
	if stop < 0 {
		stop = length + stop
	}
	if start < 0 {
		start = 0
	}
	if stop >= length {
		stop = length - 1
	}
	if start > stop || start >= length {
		return 0, 0, false
	}
	return start, stop, true
}
