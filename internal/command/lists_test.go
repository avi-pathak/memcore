package command

import (
	"testing"

	"github.com/avinashpathak/memcore/internal/resp"
)

func TestRPushAndLPushBuildAListInOrder(t *testing.T) {
	r, ctx, _ := newTestEnv()
	if got := run(r, ctx, "RPUSH", "l", "a", "b", "c"); !got.Equal(resp.Int(3)) {
		t.Fatalf("RPUSH = %v, want 3", got)
	}
	if got := run(r, ctx, "LPUSH", "l", "z"); !got.Equal(resp.Int(4)) {
		t.Fatalf("LPUSH = %v, want 4", got)
	}
	want := resp.Array([]resp.Reply{
		resp.BulkString("z"), resp.BulkString("a"), resp.BulkString("b"), resp.BulkString("c"),
	})
	if got := run(r, ctx, "LRANGE", "l", "0", "-1"); !got.Equal(want) {
		t.Fatalf("LRANGE = %v, want [z a b c]", got)
	}
}

func TestLLenReportsLength(t *testing.T) {
	r, ctx, _ := newTestEnv()
	run(r, ctx, "RPUSH", "l", "a", "b")
	if got := run(r, ctx, "LLEN", "l"); !got.Equal(resp.Int(2)) {
		t.Fatalf("LLEN = %v, want 2", got)
	}
	if got := run(r, ctx, "LLEN", "absent"); !got.Equal(resp.Int(0)) {
		t.Fatalf("LLEN absent = %v, want 0", got)
	}
}

func TestPoppingTheLastElementRemovesTheKey(t *testing.T) {
	r, ctx, _ := newTestEnv()
	run(r, ctx, "RPUSH", "l", "a", "b")
	if got := run(r, ctx, "LPOP", "l"); !got.Equal(resp.BulkString("a")) {
		t.Fatalf("LPOP = %v, want a", got)
	}
	if got := run(r, ctx, "RPOP", "l"); !got.Equal(resp.BulkString("b")) {
		t.Fatalf("RPOP = %v, want b", got)
	}
	if got := run(r, ctx, "EXISTS", "l"); !got.Equal(resp.Int(0)) {
		t.Fatalf("EXISTS after draining = %v, want 0", got)
	}
}

func TestLPopWithCountReturnsAnArray(t *testing.T) {
	r, ctx, _ := newTestEnv()
	run(r, ctx, "RPUSH", "l", "a", "b", "c")
	want := resp.Array([]resp.Reply{resp.BulkString("a"), resp.BulkString("b")})
	if got := run(r, ctx, "LPOP", "l", "2"); !got.Equal(want) {
		t.Fatalf("LPOP count = %v, want [a b]", got)
	}
}

func TestLPopOnAMissingKeyIsNil(t *testing.T) {
	r, ctx, _ := newTestEnv()
	if got := run(r, ctx, "LPOP", "absent"); !got.Equal(resp.Nil()) {
		t.Fatalf("LPOP absent = %v, want nil", got)
	}
}
