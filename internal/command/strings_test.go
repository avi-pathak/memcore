package command

import (
	"testing"

	"github.com/avinashpathak/memcore/internal/resp"
)

func TestGetOnAMissingKeyRepliesNil(t *testing.T) {
	r, ctx, _ := newTestEnv()
	if got := run(r, ctx, "GET", "absent"); !got.Equal(resp.Nil()) {
		t.Fatalf("GET = %v, want nil", got)
	}
}

func TestSetThenGetReturnsTheStoredValue(t *testing.T) {
	r, ctx, _ := newTestEnv()
	if got := run(r, ctx, "SET", "k", "v"); !got.Equal(resp.OK()) {
		t.Fatalf("SET = %v, want OK", got)
	}
	if got := run(r, ctx, "GET", "k"); !got.Equal(resp.BulkString("v")) {
		t.Fatalf("GET = %v, want \"v\"", got)
	}
}

func TestIncrCreatesACounterAtOne(t *testing.T) {
	r, ctx, _ := newTestEnv()
	if got := run(r, ctx, "INCR", "n"); !got.Equal(resp.Int(1)) {
		t.Fatalf("INCR = %v, want 1", got)
	}
}

func TestIncrAndDecrMoveTheCounter(t *testing.T) {
	r, ctx, _ := newTestEnv()
	run(r, ctx, "SET", "n", "10")
	if got := run(r, ctx, "INCR", "n"); !got.Equal(resp.Int(11)) {
		t.Fatalf("INCR = %v, want 11", got)
	}
	if got := run(r, ctx, "DECR", "n"); !got.Equal(resp.Int(10)) {
		t.Fatalf("DECR = %v, want 10", got)
	}
}

func TestIncrOnANonNumericValueErrors(t *testing.T) {
	r, ctx, _ := newTestEnv()
	run(r, ctx, "SET", "n", "abc")
	if got := run(r, ctx, "INCR", "n"); !got.IsError() {
		t.Fatalf("INCR = %v, want an error", got)
	}
}

func TestIncrRejectsOverflow(t *testing.T) {
	r, ctx, _ := newTestEnv()
	run(r, ctx, "SET", "n", "9223372036854775807") // math.MaxInt64
	if got := run(r, ctx, "INCR", "n"); !got.IsError() {
		t.Fatalf("INCR at MaxInt64 = %v, want an overflow error", got)
	}
}

func TestIncrPreservesAnExistingTTL(t *testing.T) {
	r, ctx, _ := newTestEnv()
	run(r, ctx, "SET", "n", "1")
	run(r, ctx, "EXPIRE", "n", "100")
	run(r, ctx, "INCR", "n")
	if got := run(r, ctx, "TTL", "n"); !got.Equal(resp.Int(100)) {
		t.Fatalf("TTL after INCR = %v, want 100", got)
	}
}
