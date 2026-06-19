package value

import "bytes"

// MakeString returns a string Value that owns a private copy of b. The copy is
// deliberate: command arguments alias the connection read buffer, which is
// reused across reads, so a stored value must not share that backing array.
func MakeString(b []byte) Value {
	return Value{kind: KindString, str: bytes.Clone(b)}
}

// Str returns the byte-string payload. It must only be called when
// Kind() == KindString; calling it on any other kind is a programming error.
func (v Value) Str() []byte {
	if v.kind != KindString {
		panic("value: Str on kind " + v.kind.String())
	}
	return v.str
}
