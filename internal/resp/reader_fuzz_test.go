package resp

import (
	"bytes"
	"testing"
	"testing/iotest"
)

// FuzzReadRequest checks the property that matters most for a streaming parser:
// the result must not depend on how the bytes are chunked. For every input, the
// parser must produce the same sequence of requests and errors whether the data
// arrives all at once or one byte per read, and it must never panic.
func FuzzReadRequest(f *testing.F) {
	seeds := []string{
		encodeRequest("PING"),
		encodeRequest("SET", "k", "v"),
		encodeRequest("GET", "longer key with spaces"),
		"PING\r\n",
		"SET k v\r\n",
		"*0\r\n",
		"*-1\r\n",
		"\r\n",
		"*1\r\n$3\r\nabc\r\n*1\r\n$1\r\nx\r\n",
		"*x\r\n",
		"*1\r\n$3\r\nabcde",
		"$5\r\nhello\r\n",
		"",
	}
	for _, s := range seeds {
		f.Add([]byte(s))
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		whole := NewReader(bytes.NewReader(data))
		dripped := NewReader(iotest.OneByteReader(bytes.NewReader(data)))
		for {
			a, errA := whole.ReadRequest()
			b, errB := dripped.ReadRequest()
			if (errA == nil) != (errB == nil) {
				t.Fatalf("chunking changed success: whole err=%v, dripped err=%v", errA, errB)
			}
			if !equalArgs(a, b) {
				t.Fatalf("chunking changed the parse: whole=%q, dripped=%q", a, b)
			}
			if errA != nil {
				return
			}
		}
	})
}

func equalArgs(a, b [][]byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !bytes.Equal(a[i], b[i]) {
			return false
		}
	}
	return true
}
