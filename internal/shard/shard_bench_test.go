package shard

import (
	"fmt"
	"testing"
	"time"

	"github.com/avinashpathak/memcore/internal/clock"
	"github.com/avinashpathak/memcore/internal/value"
)

// BenchmarkParallelReads measures aggregate read throughput across goroutines,
// each taking the real per-shard read lock. It is the mechanism behind the one
// throughput advantage a sharded store has over single-threaded command
// execution: reads on different shards proceed at the same time. Run it across
// core counts to see the scaling:
//
//	go test ./internal/shard/ -run='^$' -bench=BenchmarkParallelReads -cpu=1,2,4,8
func BenchmarkParallelReads(b *testing.B) {
	db := New(256, clock.NewManualClock(time.Unix(0, 0)))

	const n = 65536
	keys := make([][]byte, n)
	for i := range keys {
		k := []byte(fmt.Sprintf("key-%05d", i))
		keys[i] = k
		sh := db.ShardOf(k)
		db.LockAt(sh)
		db.Set(string(k), value.MakeString([]byte("value")))
		db.UnlockAt(sh)
	}

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			k := keys[i&(n-1)]
			i++
			sh := db.ShardOf(k)
			db.RLockAt(sh)
			db.GetBytes(k)
			db.RUnlockAt(sh)
		}
	})
}

// BenchmarkParallelMixed runs an 80/20 read/write mix across goroutines, a more
// realistic shape than reads alone, still exercising the per-shard locks.
func BenchmarkParallelMixed(b *testing.B) {
	db := New(256, clock.NewManualClock(time.Unix(0, 0)))

	const n = 65536
	keys := make([][]byte, n)
	for i := range keys {
		k := []byte(fmt.Sprintf("key-%05d", i))
		keys[i] = k
		sh := db.ShardOf(k)
		db.LockAt(sh)
		db.Set(string(k), value.MakeString([]byte("value")))
		db.UnlockAt(sh)
	}

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			k := keys[i&(n-1)]
			sh := db.ShardOf(k)
			if i%5 == 0 {
				db.LockAt(sh)
				db.Set(string(k), value.MakeString([]byte("value")))
				db.UnlockAt(sh)
			} else {
				db.RLockAt(sh)
				db.GetBytes(k)
				db.RUnlockAt(sh)
			}
			i++
		}
	})
}
