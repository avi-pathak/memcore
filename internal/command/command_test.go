package command

import (
	"testing"
	"time"

	"github.com/avinashpathak/memcore/internal/clock"
	"github.com/avinashpathak/memcore/internal/config"
	"github.com/avinashpathak/memcore/internal/resp"
	"github.com/avinashpathak/memcore/internal/shard"
	"github.com/avinashpathak/memcore/internal/value"
)

func newTestEnv() (*Registry, *Context, *clock.ManualClock) {
	return newTestEnvN(1)
}

func newTestEnvN(databases int) (*Registry, *Context, *clock.ManualClock) {
	clk := clock.NewManualClock(time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC))
	dbs := make([]*shard.DB, databases)
	for i := range dbs {
		dbs[i] = shard.New(4, clk)
	}
	compact := value.Thresholds{MaxEntries: 128, MaxBytes: 64}
	limits := value.Limits{List: compact, Hash: compact, Set: compact, ZSet: compact}
	settings := config.NewSettings(config.Reloadable{SlowThreshold: 10 * time.Millisecond, ExpirySample: 20, SlowLogEnabled: true})
	return NewRegistry(), NewContext(clk, dbs, limits, settings), clk
}

// run dispatches a command from string arguments and returns its reply.
func run(r *Registry, ctx *Context, parts ...string) resp.Reply {
	args := make([][]byte, len(parts))
	for i, p := range parts {
		args[i] = []byte(p)
	}
	return r.Dispatch(ctx, args)
}

func TestRegistryRejectsUnknownCommands(t *testing.T) {
	r, ctx, _ := newTestEnv()
	if got := run(r, ctx, "NOSUCHCOMMAND"); !got.IsError() {
		t.Fatalf("unknown command = %v, want an error", got)
	}
}

func TestRegistryEnforcesArity(t *testing.T) {
	r, ctx, _ := newTestEnv()
	tests := [][]string{
		{"GET"},
		{"GET", "a", "b"},
		{"SET", "k"},
		{"DEL"},
	}
	for _, args := range tests {
		if got := run(r, ctx, args...); !got.IsError() {
			t.Fatalf("%v = %v, want an arity error", args, got)
		}
	}
}

func TestLookupIsCaseInsensitive(t *testing.T) {
	r, ctx, _ := newTestEnv()
	if got := run(r, ctx, "pInG"); !got.Equal(resp.Simple("PONG")) {
		t.Fatalf("pInG = %v, want PONG", got)
	}
}
