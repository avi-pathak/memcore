// Package resp implements RESP, the Redis serialization protocol. It is a pure
// transformation layer: Reply models a wire reply as a tagged union, the reader
// parses bytes into requests, and the writer serializes replies. Nothing here
// touches a socket or knows about commands.
package resp

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

type replyKind uint8

const (
	replyNil replyKind = iota
	replySimple
	replyError
	replyInt
	replyBulk
	replyArray
)

// Reply is a RESP reply as a tagged union. Exactly one payload field is live,
// selected by kind. Construct replies with the helpers below and serialize them
// with a Writer.
//
// The shape is RESP2 today. RESP3-only kinds (map, set, double, boolean, push)
// are additive: a new kind plus a Writer case, with no change to callers.
type Reply struct {
	kind  replyKind
	str   string
	num   int64
	bulk  []byte
	array []Reply
}

// Nil returns the null reply. A RESP2 Writer renders it as a null bulk string.
func Nil() Reply { return Reply{kind: replyNil} }

// Simple returns a simple-string reply such as +OK.
func Simple(s string) Reply { return Reply{kind: replySimple, str: s} }

// OK is the conventional +OK reply.
func OK() Reply { return Reply{kind: replySimple, str: "OK"} }

// Error returns an error reply. s carries the message, conventionally prefixed
// with an uppercase code such as ERR or WRONGTYPE.
func Error(s string) Reply { return Reply{kind: replyError, str: s} }

// Errorf returns an error reply formatted from a template.
func Errorf(format string, a ...any) Reply {
	return Reply{kind: replyError, str: fmt.Sprintf(format, a...)}
}

// Int returns an integer reply.
func Int(n int64) Reply { return Reply{kind: replyInt, num: n} }

// Bulk returns a bulk-string reply over b without copying it. The caller must
// not mutate b afterward; stored values are immutable, so passing their bytes
// directly is safe.
func Bulk(b []byte) Reply { return Reply{kind: replyBulk, bulk: b} }

// BulkString returns a bulk-string reply holding s.
func BulkString(s string) Reply { return Reply{kind: replyBulk, bulk: []byte(s)} }

// Array returns an array reply over elems. A nil or empty slice serializes as
// an empty array, not a null array.
func Array(elems []Reply) Reply { return Reply{kind: replyArray, array: elems} }

// IsError reports whether the reply is an error reply.
func (r Reply) IsError() bool { return r.kind == replyError }

// Equal reports whether two replies are structurally equal. Empty and nil byte
// payloads compare equal.
func (r Reply) Equal(o Reply) bool {
	if r.kind != o.kind {
		return false
	}
	switch r.kind {
	case replySimple, replyError:
		return r.str == o.str
	case replyInt:
		return r.num == o.num
	case replyBulk:
		return bytes.Equal(r.bulk, o.bulk)
	case replyArray:
		if len(r.array) != len(o.array) {
			return false
		}
		for i := range r.array {
			if !r.array[i].Equal(o.array[i]) {
				return false
			}
		}
		return true
	case replyNil:
		return true
	default:
		return false
	}
}

// String renders the reply in a redis-cli-like form, for logs and test output.
func (r Reply) String() string {
	switch r.kind {
	case replyNil:
		return "(nil)"
	case replySimple:
		return r.str
	case replyError:
		return "(error) " + r.str
	case replyInt:
		return strconv.FormatInt(r.num, 10)
	case replyBulk:
		return strconv.Quote(string(r.bulk))
	case replyArray:
		var sb strings.Builder
		sb.WriteByte('[')
		for i, e := range r.array {
			if i > 0 {
				sb.WriteByte(' ')
			}
			sb.WriteString(e.String())
		}
		sb.WriteByte(']')
		return sb.String()
	default:
		return "(unknown)"
	}
}
