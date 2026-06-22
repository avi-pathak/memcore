// Package memcore is a minimal, dependency-free client for a Memcore server
// speaking RESP2 over TCP. Memcore is wire-compatible with Redis, so this client
// also works against a Redis server, and a full-featured client such as
// github.com/redis/go-redis works against Memcore. Use this when a small,
// dependency-free client is preferable.
//
// A Client serializes one request at a time and is not safe for concurrent use;
// give each goroutine its own Client, or guard one with a mutex.
package memcore

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"
)

// Error is a server error reply (a RESP "-ERR ..." line), as opposed to a
// transport error. Callers can distinguish it with errors.As.
type Error struct{ Msg string }

func (e *Error) Error() string { return e.Msg }

// Client is a connection to a Memcore server.
type Client struct {
	conn net.Conn
	r    *bufio.Reader
	w    *bufio.Writer
}

// Dial connects to addr (host:port) with a default timeout.
func Dial(addr string) (*Client, error) {
	return DialTimeout(addr, 5*time.Second)
}

// DialTimeout connects to addr, failing if the connection is not established
// within timeout.
func DialTimeout(addr string, timeout time.Duration) (*Client, error) {
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return nil, err
	}
	if tcp, ok := conn.(*net.TCPConn); ok {
		_ = tcp.SetNoDelay(true)
	}
	return &Client{conn: conn, r: bufio.NewReader(conn), w: bufio.NewWriter(conn)}, nil
}

// Close closes the connection.
func (c *Client) Close() error { return c.conn.Close() }

// Do sends a command and returns the decoded reply. The reply is one of: string
// (simple or bulk), int64, nil (a null bulk or array), or []any (an array). A
// server error reply is returned as a *Error. The any return is a deliberate
// convenience for a generic client API; typed helpers cover the common cases.
func (c *Client) Do(args ...string) (any, error) {
	if err := c.writeCommand(args); err != nil {
		return nil, err
	}
	if err := c.w.Flush(); err != nil {
		return nil, err
	}
	return readReply(c.r)
}

// Get returns the value at key and whether it was present.
func (c *Client) Get(key string) (string, bool, error) {
	v, err := c.Do("GET", key)
	if err != nil {
		return "", false, err
	}
	if v == nil {
		return "", false, nil
	}
	s, ok := v.(string)
	if !ok {
		return "", false, fmt.Errorf("memcore: GET returned %T, want string", v)
	}
	return s, true, nil
}

// Set stores value at key.
func (c *Client) Set(key, value string) error {
	_, err := c.Do("SET", key, value)
	return err
}

// Del removes the given keys and returns how many existed.
func (c *Client) Del(keys ...string) (int64, error) {
	v, err := c.Do(append([]string{"DEL"}, keys...)...)
	if err != nil {
		return 0, err
	}
	n, _ := v.(int64)
	return n, nil
}

// Ping verifies the server answers.
func (c *Client) Ping() error {
	v, err := c.Do("PING")
	if err != nil {
		return err
	}
	if v != "PONG" {
		return fmt.Errorf("memcore: unexpected PING reply %v", v)
	}
	return nil
}

func (c *Client) writeCommand(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("memcore: empty command")
	}
	if _, err := fmt.Fprintf(c.w, "*%d\r\n", len(args)); err != nil {
		return err
	}
	for _, a := range args {
		if _, err := fmt.Fprintf(c.w, "$%d\r\n", len(a)); err != nil {
			return err
		}
		if _, err := c.w.WriteString(a); err != nil {
			return err
		}
		if _, err := c.w.WriteString("\r\n"); err != nil {
			return err
		}
	}
	return nil
}

func readReply(r *bufio.Reader) (any, error) {
	line, err := readLine(r)
	if err != nil {
		return nil, err
	}
	if len(line) == 0 {
		return nil, fmt.Errorf("memcore: empty reply")
	}
	body := string(line[1:])
	switch line[0] {
	case '+':
		return body, nil
	case '-':
		return nil, &Error{Msg: body}
	case ':':
		return strconv.ParseInt(body, 10, 64)
	case '$':
		n, err := strconv.Atoi(body)
		if err != nil {
			return nil, err
		}
		if n < 0 {
			return nil, nil
		}
		buf := make([]byte, n+2) // value plus its trailing CRLF
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, err
		}
		return string(buf[:n]), nil
	case '*':
		n, err := strconv.Atoi(body)
		if err != nil {
			return nil, err
		}
		if n < 0 {
			return nil, nil
		}
		items := make([]any, n)
		for i := range items {
			v, err := readReply(r)
			if err != nil {
				return nil, err
			}
			items[i] = v
		}
		return items, nil
	default:
		return nil, fmt.Errorf("memcore: unknown reply type %q", line[0])
	}
}

func readLine(r *bufio.Reader) ([]byte, error) {
	line, err := r.ReadBytes('\n')
	if err != nil {
		return nil, err
	}
	n := len(line)
	if n >= 2 && line[n-2] == '\r' {
		return line[:n-2], nil
	}
	return line[:n-1], nil
}
