// Package value defines Memcore's data values as a tagged union. A Value holds
// exactly one payload, selected by its Kind; callers switch on Kind rather than
// type-asserting an interface, so the hot path stays free of boxing.
package value

// Kind identifies the payload a Value carries.
type Kind uint8

const (
	// None is the kind of the zero Value: it carries no payload.
	None Kind = iota
	// String is a binary-safe byte string.
	String
)

// String returns the lowercase type name, matching what the TYPE command
// reports to clients.
func (k Kind) String() string {
	switch k {
	case None:
		return "none"
	case String:
		return "string"
	default:
		return "unknown"
	}
}

// Value is an immutable tagged union over Memcore's data types. Exactly one
// payload field is live, selected by kind; the zero Value has kind None.
//
// Stored payload bytes are treated as immutable once a Value is constructed.
// That invariant lets a reader hold a reference to the payload and serialize a
// reply after releasing the shard lock, without risking a torn read.
type Value struct {
	kind Kind
	str  []byte
}

// Kind returns the value's discriminator.
func (v Value) Kind() Kind { return v.kind }
