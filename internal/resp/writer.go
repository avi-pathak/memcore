package resp

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
)

const writeBufferSize = 16 << 10

// Writer serializes replies to a byte stream in RESP2. It buffers output, so a
// caller flushes once after writing a batch of replies. A Writer belongs to a
// single connection goroutine and is not safe for concurrent use.
//
// A write error is sticky: after the first failure the Writer drops further
// output and returns that error from every method, so a caller can issue a run
// of writes and check once. A connection discards its Writer after an error.
type Writer struct {
	w       *bufio.Writer
	scratch []byte
	err     error
}

// NewWriter returns a Writer over w.
func NewWriter(w io.Writer) *Writer {
	return &Writer{w: bufio.NewWriterSize(w, writeBufferSize)}
}

// WriteReply serializes r into the buffer. The first error is retained and
// returned; see the type comment.
func (w *Writer) WriteReply(r Reply) error {
	w.writeReply(r)
	return w.err
}

// Flush writes any buffered bytes to the underlying stream.
func (w *Writer) Flush() error {
	if w.err != nil {
		return w.err
	}
	w.err = w.w.Flush()
	return w.err
}

func (w *Writer) writeReply(r Reply) {
	switch r.kind {
	case replyNil:
		w.writeString("$-1\r\n")
	case replySimple:
		w.writeFrame('+', r.str)
	case replyError:
		w.writeFrame('-', r.str)
	case replyInt:
		w.writeByte(':')
		w.writeInt(r.num)
		w.writeString("\r\n")
	case replyBulk:
		w.writeBulk(r.bulk)
	case replyArray:
		w.writeByte('*')
		w.writeInt(int64(len(r.array)))
		w.writeString("\r\n")
		for i := range r.array {
			w.writeReply(r.array[i])
		}
	default:
		w.err = fmt.Errorf("resp: unserializable reply kind %d", r.kind)
	}
}

func (w *Writer) writeFrame(prefix byte, s string) {
	w.writeByte(prefix)
	w.writeString(s)
	w.writeString("\r\n")
}

func (w *Writer) writeBulk(b []byte) {
	w.writeByte('$')
	w.writeInt(int64(len(b)))
	w.writeString("\r\n")
	w.write(b)
	w.writeString("\r\n")
}

func (w *Writer) writeInt(n int64) {
	if w.err != nil {
		return
	}
	w.scratch = strconv.AppendInt(w.scratch[:0], n, 10)
	_, w.err = w.w.Write(w.scratch)
}

func (w *Writer) write(p []byte) {
	if w.err != nil {
		return
	}
	_, w.err = w.w.Write(p)
}

func (w *Writer) writeString(s string) {
	if w.err != nil {
		return
	}
	_, w.err = w.w.WriteString(s)
}

func (w *Writer) writeByte(b byte) {
	if w.err != nil {
		return
	}
	w.err = w.w.WriteByte(b)
}
