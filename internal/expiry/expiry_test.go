package expiry

import (
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/avinashpathak/memcore/internal/clock"
	"github.com/avinashpathak/memcore/internal/shard"
	"github.com/avinashpathak/memcore/internal/value"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func dbWithVolatileKeys(clk *clock.ManualClock, shards, keys int, ttl time.Duration) *shard.DB {
	db := shard.New(shards, clk)
	for i := 0; i < keys; i++ {
		key := fmt.Sprintf("k%d", i)
		unlock := db.LockKeys([][]byte{[]byte(key)})
		db.Set(key, value.MakeString([]byte("v")))
		db.SetExpire(key, clk.Now().Add(ttl))
		unlock()
	}
	return db
}

func TestCycleEvictsKeysOnlyOnceTheyExpire(t *testing.T) {
	clk := clock.NewManualClock(time.Unix(0, 0))
	db := dbWithVolatileKeys(clk, 4, 20, time.Second)
	r := New([]*shard.DB{db}, clk, Config{Interval: time.Millisecond, SamplePerShard: func() int { return 100 }}, discardLogger())

	if _, evicted := r.cycle(); evicted != 0 {
		t.Fatalf("cycle before the deadline evicted %d, want 0", evicted)
	}
	clk.Advance(2 * time.Second)
	if _, evicted := r.cycle(); evicted != 20 {
		t.Fatalf("cycle after the deadline evicted %d, want 20", evicted)
	}
}

func TestCycleRespectsThePerShardBudget(t *testing.T) {
	clk := clock.NewManualClock(time.Unix(0, 0))
	db := dbWithVolatileKeys(clk, 1, 10, time.Second) // one shard makes the budget exact
	clk.Advance(2 * time.Second)
	r := New([]*shard.DB{db}, clk, Config{Interval: time.Millisecond, SamplePerShard: func() int { return 3 }}, discardLogger())

	if examined, _ := r.cycle(); examined != 3 {
		t.Fatalf("examined = %d, want the per-shard budget of 3", examined)
	}
}

func TestCycleSkipsWhenTheBudgetIsZero(t *testing.T) {
	clk := clock.NewManualClock(time.Unix(0, 0))
	db := dbWithVolatileKeys(clk, 1, 5, time.Second)
	clk.Advance(2 * time.Second)
	r := New([]*shard.DB{db}, clk, Config{Interval: time.Millisecond, SamplePerShard: func() int { return 0 }}, discardLogger())

	if examined, evicted := r.cycle(); examined != 0 || evicted != 0 {
		t.Fatalf("examined=%d evicted=%d, want 0 and 0 when the budget is zero", examined, evicted)
	}
}
