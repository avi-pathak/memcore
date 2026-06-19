package command

import (
	"testing"

	"github.com/avinashpathak/memcore/internal/resp"
)

func TestZAddCountsOnlyNewMembers(t *testing.T) {
	r, ctx, _ := newTestEnv()
	if got := run(r, ctx, "ZADD", "z", "1", "a", "2", "b"); !got.Equal(resp.Int(2)) {
		t.Fatalf("ZADD = %v, want 2", got)
	}
	// Update a, add c: only c is new.
	if got := run(r, ctx, "ZADD", "z", "5", "a", "3", "c"); !got.Equal(resp.Int(1)) {
		t.Fatalf("ZADD update = %v, want 1", got)
	}
}

func TestZScoreReturnsScoresAndNilForMisses(t *testing.T) {
	r, ctx, _ := newTestEnv()
	run(r, ctx, "ZADD", "z", "1.5", "a")
	if got := run(r, ctx, "ZSCORE", "z", "a"); !got.Equal(resp.BulkString("1.5")) {
		t.Fatalf("ZSCORE = %v, want 1.5", got)
	}
	if got := run(r, ctx, "ZSCORE", "z", "absent"); !got.Equal(resp.Nil()) {
		t.Fatalf("ZSCORE absent = %v, want nil", got)
	}
}

func TestZRangeReturnsMembersInScoreOrder(t *testing.T) {
	r, ctx, _ := newTestEnv()
	run(r, ctx, "ZADD", "z", "3", "c", "1", "a", "2", "b")
	want := resp.Array([]resp.Reply{resp.BulkString("a"), resp.BulkString("b"), resp.BulkString("c")})
	if got := run(r, ctx, "ZRANGE", "z", "0", "-1"); !got.Equal(want) {
		t.Fatalf("ZRANGE = %v, want [a b c]", got)
	}
}

func TestZRangeWithScoresInterleavesScores(t *testing.T) {
	r, ctx, _ := newTestEnv()
	run(r, ctx, "ZADD", "z", "1", "a", "2", "b")
	want := resp.Array([]resp.Reply{
		resp.BulkString("a"), resp.BulkString("1"),
		resp.BulkString("b"), resp.BulkString("2"),
	})
	if got := run(r, ctx, "ZRANGE", "z", "0", "-1", "WITHSCORES"); !got.Equal(want) {
		t.Fatalf("ZRANGE WITHSCORES = %v, want [a 1 b 2]", got)
	}
}

func TestZAddRejectsNonNumericScores(t *testing.T) {
	r, ctx, _ := newTestEnv()
	if got := run(r, ctx, "ZADD", "z", "notanumber", "a"); !got.IsError() {
		t.Fatalf("ZADD bad score = %v, want an error", got)
	}
}
