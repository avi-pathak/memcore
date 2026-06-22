package main

import (
	"net"
	"testing"
	"time"
)

// fakeServer accepts a single connection and writes reply to it, so the
// healthcheck ping can be exercised without standing up the full server. It
// returns the address to dial and a stop function.
func fakeServer(t *testing.T, reply string) (addr string, stop func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
		buf := make([]byte, 64)
		_, _ = conn.Read(buf) // drain the PING request before replying
		_, _ = conn.Write([]byte(reply))
	}()
	return ln.Addr().String(), func() {
		_ = ln.Close()
		<-done
	}
}

// freeAddr returns a loopback address with no listener, by binding a port and
// releasing it. The OS does not immediately reuse it, so a dial there is
// refused.
func freeAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	return addr
}

func TestHealthcheckSucceedsAgainstAPongingServer(t *testing.T) {
	addr, stop := fakeServer(t, "+PONG\r\n")
	defer stop()
	if err := ping(addr); err != nil {
		t.Fatalf("ping = %v, want nil against a server that replies +PONG", err)
	}
}

func TestHealthcheckFailsWhenNothingIsListening(t *testing.T) {
	if err := ping(freeAddr(t)); err == nil {
		t.Fatal("ping = nil, want an error when nothing is listening")
	}
}

func TestHealthcheckRejectsAnUnexpectedReply(t *testing.T) {
	addr, stop := fakeServer(t, "+NOPE\r\n")
	defer stop()
	if err := ping(addr); err == nil {
		t.Fatal("ping = nil, want an error on a non-PONG reply")
	}
}
