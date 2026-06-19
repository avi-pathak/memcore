package resp

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"testing/iotest"
)

func encodeRequest(parts ...string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "*%d\r\n", len(parts))
	for _, p := range parts {
		fmt.Fprintf(&sb, "$%d\r\n%s\r\n", len(p), p)
	}
	return sb.String()
}

func wantArgs(t *testing.T, got [][]byte, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d args %q, want %d %v", len(got), got, len(want), want)
	}
	for i := range want {
		if string(got[i]) != want[i] {
			t.Fatalf("arg %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestReadsAMultiBulkRequest(t *testing.T) {
	r := NewReader(strings.NewReader(encodeRequest("GET", "foo")))
	args, err := r.ReadRequest()
	if err != nil {
		t.Fatalf("ReadRequest: %v", err)
	}
	wantArgs(t, args, "GET", "foo")
}

func TestReadsPipelinedRequests(t *testing.T) {
	data := encodeRequest("PING") + encodeRequest("GET", "k")
	r := NewReader(strings.NewReader(data))
	first, err := r.ReadRequest()
	if err != nil {
		t.Fatalf("first ReadRequest: %v", err)
	}
	wantArgs(t, first, "PING")
	second, err := r.ReadRequest()
	if err != nil {
		t.Fatalf("second ReadRequest: %v", err)
	}
	wantArgs(t, second, "GET", "k")
}

func TestReadsAnInlineRequest(t *testing.T) {
	r := NewReader(strings.NewReader("SET k v\r\n"))
	args, err := r.ReadRequest()
	if err != nil {
		t.Fatalf("ReadRequest: %v", err)
	}
	wantArgs(t, args, "SET", "k", "v")
}

func TestAnEmptyArrayCarriesNoRequest(t *testing.T) {
	r := NewReader(strings.NewReader("*0\r\n"))
	args, err := r.ReadRequest()
	if err != nil {
		t.Fatalf("ReadRequest: %v", err)
	}
	if len(args) != 0 {
		t.Fatalf("got %q, want no args", args)
	}
}

func TestAnEmptyStreamReturnsEOF(t *testing.T) {
	r := NewReader(strings.NewReader(""))
	if _, err := r.ReadRequest(); !errors.Is(err, io.EOF) {
		t.Fatalf("err = %v, want io.EOF", err)
	}
}

func TestARequestSplitOneBytePerReadStillParses(t *testing.T) {
	data := encodeRequest("SET", "key", "a longer value with spaces")
	r := NewReader(iotest.OneByteReader(strings.NewReader(data)))
	args, err := r.ReadRequest()
	if err != nil {
		t.Fatalf("ReadRequest: %v", err)
	}
	wantArgs(t, args, "SET", "key", "a longer value with spaces")
}

func TestALargeBulkUsesTheChunkedPath(t *testing.T) {
	payload := strings.Repeat("x", 200*1024) // exceeds directReadLimit
	r := NewReader(strings.NewReader(encodeRequest("SET", "k", payload)))
	args, err := r.ReadRequest()
	if err != nil {
		t.Fatalf("ReadRequest: %v", err)
	}
	if string(args[2]) != payload {
		t.Fatalf("payload round-trip lost data: got %d bytes, want %d", len(args[2]), len(payload))
	}
}

func TestMalformedRequestsAreProtocolErrors(t *testing.T) {
	tests := map[string]string{
		"non-numeric multibulk count": "*x\r\n",
		"non-numeric bulk length":     "*1\r\n$x\r\n",
		"missing element prefix":      "*1\r\n+OK\r\n",
		"bulk not CRLF terminated":    "*1\r\n$3\r\nabcde",
		"multibulk count too large":   "*2000000\r\n",
		"bulk length too large":       "*1\r\n$600000000\r\n",
		"line not CRLF terminated":    "*1\n",
	}
	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			r := NewReader(strings.NewReader(data))
			if _, err := r.ReadRequest(); !errors.Is(err, ErrProtocol) {
				t.Fatalf("err = %v, want a protocol error", err)
			}
		})
	}
}

func TestATruncatedRequestReportsUnexpectedEOF(t *testing.T) {
	r := NewReader(strings.NewReader("*2\r\n$3\r\nGET\r\n")) // promises 2 args, sends 1
	if _, err := r.ReadRequest(); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("err = %v, want io.ErrUnexpectedEOF", err)
	}
}
