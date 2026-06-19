package server

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/avinashpathak/memcore/internal/clock"
	"github.com/avinashpathak/memcore/internal/command"
	"github.com/avinashpathak/memcore/internal/config"
)

func startTestServer(t *testing.T) (addr string, stop func()) {
	t.Helper()
	cfg := config.Default()
	cfg.Network.Host = "127.0.0.1"
	cfg.Network.Port = 0 // let the OS choose a free port
	cfg.Network.Databases = 4

	srv := New(cfg, clock.SystemClock{}, slog.New(slog.NewTextHandler(io.Discard, nil)), command.NewRegistry())
	if err := srv.Listen(); err != nil {
		t.Fatalf("Listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- srv.Serve(ctx) }()

	stop = func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("Serve returned error: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("Serve did not return after shutdown")
		}
	}
	return srv.Addr().String(), stop
}

type testClient struct {
	t    *testing.T
	conn net.Conn
	r    *bufio.Reader
}

func dialTestClient(t *testing.T, addr string) *testClient {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return &testClient{t: t, conn: conn, r: bufio.NewReader(conn)}
}

func encodeReq(parts ...string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "*%d\r\n", len(parts))
	for _, p := range parts {
		fmt.Fprintf(&b, "$%d\r\n%s\r\n", len(p), p)
	}
	return b.String()
}

func (c *testClient) send(parts ...string) {
	c.t.Helper()
	c.writeRaw(encodeReq(parts...))
}

func (c *testClient) writeRaw(s string) {
	c.t.Helper()
	if _, err := c.conn.Write([]byte(s)); err != nil {
		c.t.Fatalf("write: %v", err)
	}
}

func (c *testClient) line() string {
	c.t.Helper()
	s, err := c.r.ReadString('\n')
	if err != nil {
		c.t.Fatalf("read line: %v", err)
	}
	return strings.TrimRight(s, "\r\n")
}

func (c *testClient) bulk() string {
	c.t.Helper()
	header := c.line()
	if header == "$-1" {
		return "<nil>"
	}
	if len(header) == 0 || header[0] != '$' {
		c.t.Fatalf("expected a bulk header, got %q", header)
	}
	n, err := strconv.Atoi(header[1:])
	if err != nil {
		c.t.Fatalf("bad bulk length %q", header)
	}
	buf := make([]byte, n+2)
	if _, err := io.ReadFull(c.r, buf); err != nil {
		c.t.Fatalf("read bulk body: %v", err)
	}
	return string(buf[:n])
}

func TestServerServesCommandsOverTCP(t *testing.T) {
	addr, stop := startTestServer(t)
	defer stop()
	c := dialTestClient(t, addr)

	c.send("PING")
	if got := c.line(); got != "+PONG" {
		t.Fatalf("PING = %q, want +PONG", got)
	}
	c.send("SET", "k", "hello")
	if got := c.line(); got != "+OK" {
		t.Fatalf("SET = %q, want +OK", got)
	}
	c.send("GET", "k")
	if got := c.bulk(); got != "hello" {
		t.Fatalf("GET = %q, want hello", got)
	}
	c.send("GET", "absent")
	if got := c.bulk(); got != "<nil>" {
		t.Fatalf("GET absent = %q, want a nil reply", got)
	}
	c.send("INCR", "n")
	if got := c.line(); got != ":1" {
		t.Fatalf("INCR = %q, want :1", got)
	}
	c.send("NOSUCHCOMMAND")
	if got := c.line(); !strings.HasPrefix(got, "-ERR") {
		t.Fatalf("unknown command = %q, want an -ERR reply", got)
	}
}

func TestServerKeepsDatabasesIsolatedAcrossSelect(t *testing.T) {
	addr, stop := startTestServer(t)
	defer stop()
	c := dialTestClient(t, addr)

	c.send("SET", "k", "zero")
	c.line()
	c.send("SELECT", "1")
	if got := c.line(); got != "+OK" {
		t.Fatalf("SELECT 1 = %q, want +OK", got)
	}
	c.send("GET", "k")
	if got := c.bulk(); got != "<nil>" {
		t.Fatalf("GET in db1 = %q, want a nil reply", got)
	}
}

func TestServerHandlesPipelinedRequests(t *testing.T) {
	addr, stop := startTestServer(t)
	defer stop()
	c := dialTestClient(t, addr)

	c.writeRaw(encodeReq("SET", "a", "1") + encodeReq("INCR", "a") + encodeReq("GET", "a"))
	if got := c.line(); got != "+OK" {
		t.Fatalf("pipelined SET = %q, want +OK", got)
	}
	if got := c.line(); got != ":2" {
		t.Fatalf("pipelined INCR = %q, want :2", got)
	}
	if got := c.bulk(); got != "2" {
		t.Fatalf("pipelined GET = %q, want 2", got)
	}
}

func TestServerRecoversFromAProtocolError(t *testing.T) {
	addr, stop := startTestServer(t)
	defer stop()
	c := dialTestClient(t, addr)

	c.writeRaw("*1\r\n$x\r\n") // bulk length is not a number
	if got := c.line(); !strings.HasPrefix(got, "-ERR Protocol error") {
		t.Fatalf("malformed input = %q, want a protocol error reply", got)
	}
}

func TestServerShutsDownWithAnOpenConnection(t *testing.T) {
	cfg := config.Default()
	cfg.Network.Host = "127.0.0.1"
	cfg.Network.Port = 0
	srv := New(cfg, clock.SystemClock{}, slog.New(slog.NewTextHandler(io.Discard, nil)), command.NewRegistry())
	if err := srv.Listen(); err != nil {
		t.Fatalf("Listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- srv.Serve(ctx) }()

	conn, err := net.Dial("tcp", srv.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	cancel() // an idle open connection must not block shutdown
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Serve: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not shut down with an open connection")
	}
}
