package command

import (
	"testing"

	"github.com/avinashpathak/memcore/internal/resp"
)

func TestPingRepliesPong(t *testing.T) {
	r, ctx, _ := newTestEnv()
	if got := run(r, ctx, "PING"); !got.Equal(resp.Simple("PONG")) {
		t.Fatalf("PING = %v, want PONG", got)
	}
}

func TestPingEchoesItsMessage(t *testing.T) {
	r, ctx, _ := newTestEnv()
	if got := run(r, ctx, "PING", "hello"); !got.Equal(resp.BulkString("hello")) {
		t.Fatalf("PING hello = %v, want \"hello\"", got)
	}
}

func TestFlushdbEmptiesTheDatabase(t *testing.T) {
	r, ctx, _ := newTestEnv()
	run(r, ctx, "SET", "a", "1")
	run(r, ctx, "SET", "b", "2")
	if got := run(r, ctx, "FLUSHDB"); !got.Equal(resp.OK()) {
		t.Fatalf("FLUSHDB = %v, want OK", got)
	}
	if got := run(r, ctx, "EXISTS", "a", "b"); !got.Equal(resp.Int(0)) {
		t.Fatalf("EXISTS after FLUSHDB = %v, want 0", got)
	}
}

func TestSelectIsolatesDatabases(t *testing.T) {
	r, ctx, _ := newTestEnvN(2)
	run(r, ctx, "SET", "k", "db0")
	if got := run(r, ctx, "SELECT", "1"); !got.Equal(resp.OK()) {
		t.Fatalf("SELECT 1 = %v, want OK", got)
	}
	if got := run(r, ctx, "GET", "k"); !got.Equal(resp.Nil()) {
		t.Fatalf("GET in db1 = %v, want nil", got)
	}
	run(r, ctx, "SET", "k", "db1")
	run(r, ctx, "SELECT", "0")
	if got := run(r, ctx, "GET", "k"); !got.Equal(resp.BulkString("db0")) {
		t.Fatalf("GET in db0 = %v, want db0", got)
	}
}

func TestSelectRejectsAnOutOfRangeIndex(t *testing.T) {
	r, ctx, _ := newTestEnvN(2)
	if got := run(r, ctx, "SELECT", "5"); !got.IsError() {
		t.Fatalf("SELECT 5 = %v, want an error", got)
	}
}
