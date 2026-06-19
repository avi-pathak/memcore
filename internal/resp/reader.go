package resp

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"strconv"
)

// ErrProtocol is the sentinel wrapped by every malformed-input error the reader
// reports. The server replies with the error text and closes the connection.
var ErrProtocol = errors.New("resp: protocol error")

const (
	maxBulkLen      = 512 << 20 // 512 MiB, matching Redis proto-max-bulk-len
	maxMultiBulkLen = 1 << 20   // elements per request
	directReadLimit = 64 << 10  // bulks up to this size are read in one allocation
	readBufferSize  = 16 << 10
)

// Reader parses RESP requests from a byte stream. It is built on a bufio.Reader,
// so a request delivered across several underlying reads (TCP packet boundaries)
// parses correctly. A Reader belongs to a single connection goroutine and is not
// safe for concurrent use.
type Reader struct {
	r *bufio.Reader
}

// NewReader returns a Reader over rd.
func NewReader(rd io.Reader) *Reader {
	return &Reader{r: bufio.NewReaderSize(rd, readBufferSize)}
}

// ReadRequest reads one client request: a command name followed by its
// arguments. It accepts the multi-bulk form clients use and the inline form a
// raw terminal sends. A clean end of stream between requests returns io.EOF; an
// end of stream partway through a request returns io.ErrUnexpectedEOF.
func (r *Reader) ReadRequest() ([][]byte, error) {
	prefix, err := r.r.ReadByte()
	if err != nil {
		return nil, err
	}
	if prefix == '*' {
		return r.readMultiBulk()
	}
	if err := r.r.UnreadByte(); err != nil {
		return nil, err
	}
	return r.readInline()
}

func (r *Reader) readMultiBulk() ([][]byte, error) {
	n, err := r.readCount()
	if err != nil {
		return nil, err
	}
	// A zero or null array carries no command. The server skips it.
	if n <= 0 {
		return nil, nil
	}
	if n > maxMultiBulkLen {
		return nil, fmt.Errorf("%w: multi-bulk count %d too large", ErrProtocol, n)
	}
	args := make([][]byte, n)
	for i := 0; i < n; i++ {
		marker, err := r.r.ReadByte()
		if err != nil {
			return nil, eofIsUnexpected(err)
		}
		if marker != '$' {
			return nil, fmt.Errorf("%w: expected '$', got %q", ErrProtocol, marker)
		}
		arg, err := r.readBulk()
		if err != nil {
			return nil, err
		}
		args[i] = arg
	}
	return args, nil
}

func (r *Reader) readBulk() ([]byte, error) {
	n, err := r.readCount()
	if err != nil {
		return nil, err
	}
	if n < 0 || n > maxBulkLen {
		return nil, fmt.Errorf("%w: bulk length %d out of range", ErrProtocol, n)
	}
	body, err := r.readN(n)
	if err != nil {
		return nil, err
	}
	if err := r.expectCRLF(); err != nil {
		return nil, err
	}
	return body, nil
}

func (r *Reader) readInline() ([][]byte, error) {
	line, err := r.readLine()
	if err != nil {
		return nil, err
	}
	fields := bytes.Fields(line)
	if len(fields) == 0 {
		return nil, nil
	}
	// bytes.Fields returns slices into the reader's buffer, which is reused on
	// the next read, so each field is copied before it escapes.
	args := make([][]byte, len(fields))
	for i, f := range fields {
		args[i] = bytes.Clone(f)
	}
	return args, nil
}

// readN reads exactly n payload bytes. Small bodies are read into a single
// allocation; larger ones grow as bytes arrive, so a client that declares a huge
// length but stalls cannot force a giant up-front allocation.
func (r *Reader) readN(n int) ([]byte, error) {
	if n <= directReadLimit {
		buf := make([]byte, n)
		if _, err := io.ReadFull(r.r, buf); err != nil {
			return nil, eofIsUnexpected(err)
		}
		return buf, nil
	}
	buf := make([]byte, 0, directReadLimit)
	chunk := make([]byte, directReadLimit)
	for len(buf) < n {
		want := n - len(buf)
		if want > len(chunk) {
			want = len(chunk)
		}
		m, err := io.ReadFull(r.r, chunk[:want])
		buf = append(buf, chunk[:m]...)
		if err != nil {
			return nil, eofIsUnexpected(err)
		}
	}
	return buf, nil
}

func (r *Reader) readCount() (int, error) {
	line, err := r.readLine()
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(string(line))
	if err != nil {
		return 0, fmt.Errorf("%w: invalid count %q", ErrProtocol, line)
	}
	return n, nil
}

// readLine returns the next CRLF-terminated line without its terminator. The
// returned slice points into the reader's buffer and is only valid until the
// next read.
func (r *Reader) readLine() ([]byte, error) {
	line, err := r.r.ReadSlice('\n')
	if err != nil {
		if errors.Is(err, bufio.ErrBufferFull) {
			return nil, fmt.Errorf("%w: line too long", ErrProtocol)
		}
		return nil, eofIsUnexpected(err)
	}
	n := len(line)
	if n < 2 || line[n-2] != '\r' {
		return nil, fmt.Errorf("%w: line not terminated by CRLF", ErrProtocol)
	}
	return line[:n-2], nil
}

func (r *Reader) expectCRLF() error {
	cr, err := r.r.ReadByte()
	if err != nil {
		return eofIsUnexpected(err)
	}
	lf, err := r.r.ReadByte()
	if err != nil {
		return eofIsUnexpected(err)
	}
	if cr != '\r' || lf != '\n' {
		return fmt.Errorf("%w: bulk not terminated by CRLF", ErrProtocol)
	}
	return nil
}

// eofIsUnexpected maps a stream that ends partway through a request to
// io.ErrUnexpectedEOF; a clean io.EOF only ever surfaces at a request boundary.
func eofIsUnexpected(err error) error {
	if errors.Is(err, io.EOF) {
		return io.ErrUnexpectedEOF
	}
	return err
}
