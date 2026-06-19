package command

import (
	"testing"

	"github.com/avinashpathak/memcore/internal/resp"
)

func TestConfigGetReturnsAReloadableParameter(t *testing.T) {
	r, ctx, _ := newTestEnv()
	got := run(r, ctx, "CONFIG", "GET", "expiry-sample-per-shard")
	want := resp.Array([]resp.Reply{resp.BulkString("expiry-sample-per-shard"), resp.BulkString("20")})
	if !got.Equal(want) {
		t.Fatalf("CONFIG GET = %v, want %v", got, want)
	}
}

func TestConfigSetChangesAParameterVisibleToConfigGet(t *testing.T) {
	r, ctx, _ := newTestEnv()
	if got := run(r, ctx, "CONFIG", "SET", "slowlog-threshold-ms", "50"); !got.Equal(resp.OK()) {
		t.Fatalf("CONFIG SET = %v, want OK", got)
	}
	got := run(r, ctx, "CONFIG", "GET", "slowlog-threshold-ms")
	want := resp.Array([]resp.Reply{resp.BulkString("slowlog-threshold-ms"), resp.BulkString("50")})
	if !got.Equal(want) {
		t.Fatalf("CONFIG GET after SET = %v, want %v", got, want)
	}
}

func TestConfigSetRejectsAnInvalidValue(t *testing.T) {
	r, ctx, _ := newTestEnv()
	if got := run(r, ctx, "CONFIG", "SET", "expiry-sample-per-shard", "-5"); !got.IsError() {
		t.Fatalf("CONFIG SET with a bad value = %v, want an error", got)
	}
}

func TestConfigGetWildcardListsEveryParameter(t *testing.T) {
	r, ctx, _ := newTestEnv()
	got := run(r, ctx, "CONFIG", "GET", "*")
	if n := len(got.Elements()); n != 6 { // three name/value pairs
		t.Fatalf("CONFIG GET * returned %d elements, want 6", n)
	}
}

func TestConfigRejectsUnknownSubcommands(t *testing.T) {
	r, ctx, _ := newTestEnv()
	if got := run(r, ctx, "CONFIG", "REWRITE"); !got.IsError() {
		t.Fatalf("CONFIG REWRITE = %v, want an error", got)
	}
}
