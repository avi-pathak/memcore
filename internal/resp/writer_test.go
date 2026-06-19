package resp

import (
	"bytes"
	"errors"
	"testing"
)

func TestWriteReplyProducesRESP2Bytes(t *testing.T) {
	tests := []struct {
		name  string
		reply Reply
		want  string
	}{
		{"ok", OK(), "+OK\r\n"},
		{"simple", Simple("PONG"), "+PONG\r\n"},
		{"error", Error("ERR bad"), "-ERR bad\r\n"},
		{"zero", Int(0), ":0\r\n"},
		{"negative", Int(-5), ":-5\r\n"},
		{"nil", Nil(), "$-1\r\n"},
		{"bulk", BulkString("foo"), "$3\r\nfoo\r\n"},
		{"empty bulk", Bulk([]byte{}), "$0\r\n\r\n"},
		{"array", Array([]Reply{BulkString("a"), Int(1)}), "*2\r\n$1\r\na\r\n:1\r\n"},
		{"empty array", Array(nil), "*0\r\n"},
		{"nested", Array([]Reply{Array([]Reply{Int(1)}), Nil()}), "*2\r\n*1\r\n:1\r\n$-1\r\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			w := NewWriter(&buf)
			if err := w.WriteReply(tt.reply); err != nil {
				t.Fatalf("WriteReply: %v", err)
			}
			if err := w.Flush(); err != nil {
				t.Fatalf("Flush: %v", err)
			}
			if got := buf.String(); got != tt.want {
				t.Fatalf("wire = %q, want %q", got, tt.want)
			}
		})
	}
}

type stubbornWriter struct{ err error }

func (s stubbornWriter) Write([]byte) (int, error) { return 0, s.err }

func TestWriteErrorsAreSticky(t *testing.T) {
	boom := errors.New("connection reset")
	w := NewWriter(stubbornWriter{err: boom})

	// Output is buffered, so the underlying failure surfaces at Flush.
	if err := w.WriteReply(OK()); err != nil {
		t.Fatalf("buffered WriteReply returned an error early: %v", err)
	}
	if err := w.Flush(); !errors.Is(err, boom) {
		t.Fatalf("Flush err = %v, want %v", err, boom)
	}
	// The error is now retained: further writes return it without reaching the
	// underlying writer again.
	if err := w.WriteReply(OK()); !errors.Is(err, boom) {
		t.Fatalf("WriteReply after error = %v, want %v", err, boom)
	}
}
