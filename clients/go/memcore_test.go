package memcore

import (
	"bufio"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestReadReplyDecodesEachType(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want any
	}{
		{"simple string", "+OK\r\n", "OK"},
		{"integer", ":42\r\n", int64(42)},
		{"bulk string", "$3\r\nfoo\r\n", "foo"},
		{"null bulk", "$-1\r\n", nil},
		{"empty bulk", "$0\r\n\r\n", ""},
		{"array", "*2\r\n$1\r\na\r\n:7\r\n", []any{"a", int64(7)}},
		{"null array", "*-1\r\n", nil},
		{"nested array", "*1\r\n*2\r\n+x\r\n:1\r\n", []any{[]any{"x", int64(1)}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := readReply(bufio.NewReader(strings.NewReader(c.in)))
			if err != nil {
				t.Fatalf("readReply: %v", err)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("readReply = %#v, want %#v", got, c.want)
			}
		})
	}
}

func TestReadReplyReturnsServerErrorsAsError(t *testing.T) {
	_, err := readReply(bufio.NewReader(strings.NewReader("-ERR nope\r\n")))
	var rerr *Error
	if !errors.As(err, &rerr) {
		t.Fatalf("err = %v, want a *Error", err)
	}
	if rerr.Msg != "ERR nope" {
		t.Fatalf("Msg = %q, want %q", rerr.Msg, "ERR nope")
	}
}

func TestWriteCommandEncodesRESP(t *testing.T) {
	var buf strings.Builder
	c := &Client{w: bufio.NewWriter(&buf)}
	if err := c.writeCommand([]string{"SET", "k", "v"}); err != nil {
		t.Fatalf("writeCommand: %v", err)
	}
	c.w.Flush()
	want := "*3\r\n$3\r\nSET\r\n$1\r\nk\r\n$1\r\nv\r\n"
	if buf.String() != want {
		t.Fatalf("encoded = %q, want %q", buf.String(), want)
	}
}
