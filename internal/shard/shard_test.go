package shard

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/avinashpathak/memcore/internal/clock"
	"github.com/avinashpathak/memcore/internal/value"
)

func newTestDB(shards int) *DB {
	return New(shards, clock.NewManualClock(time.Unix(0, 0)))
}

func TestStringAndByteRoutingAgree(t *testing.T) {
	db := newTestDB(8)
	for _, k := range []string{"foo", "bar", "baz", "qux", "a longer key"} {
		if got, want := db.indexBytes([]byte(k)), db.indexString(k); got != want {
			t.Fatalf("routing for %q disagrees: bytes=%d, string=%d", k, got, want)
		}
	}
}

func TestKeysDistributeAcrossShards(t *testing.T) {
	db := newTestDB(8)
	seen := make(map[int]bool)
	for i := 0; i < 1000; i++ {
		seen[db.indexString(fmt.Sprintf("key-%d", i))] = true
	}
	if len(seen) < 2 {
		t.Fatalf("keys landed on only %d shard(s); routing is not distributing", len(seen))
	}
}

func TestGetReturnsWhatSetStored(t *testing.T) {
	db := newTestDB(4)
	unlock := db.LockKeys([][]byte{[]byte("k")})
	db.Set("k", value.MakeString([]byte("v")))
	unlock()

	rUnlock := db.RLockKeys([][]byte{[]byte("k")})
	defer rUnlock()
	v, ok := db.Get("k")
	if !ok || string(v.Str()) != "v" {
		t.Fatalf("Get = (%q, %v), want (\"v\", true)", v.Str(), ok)
	}
}

func TestShardSetIsSortedAndDeduplicated(t *testing.T) {
	db := newTestDB(4)
	// Two copies of the same key plus another key: the result must be sorted and
	// hold each shard once.
	idx := db.shardSet([][]byte{[]byte("a"), []byte("a"), []byte("b")})
	for i := 1; i < len(idx); i++ {
		if idx[i] <= idx[i-1] {
			t.Fatalf("shard set not strictly ascending: %v", idx)
		}
	}
}

func TestConcurrentWritesToDistinctKeysAreSafe(t *testing.T) {
	db := newTestDB(8)
	const workers, perWorker = 16, 500
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				key := fmt.Sprintf("w%d-k%d", w, i)
				unlock := db.LockKeys([][]byte{[]byte(key)})
				db.Set(key, value.MakeString([]byte("v")))
				unlock()
			}
		}(w)
	}
	wg.Wait()

	got := allShardsLen(db)
	if want := workers * perWorker; got != want {
		t.Fatalf("stored %d keys, want %d", got, want)
	}
}

func TestMultiKeyLockingDoesNotDeadlock(t *testing.T) {
	db := newTestDB(4)
	keys := [][]byte{[]byte("a"), []byte("b"), []byte("c"), []byte("d")}
	var wg sync.WaitGroup
	for g := 0; g < 32; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 500; i++ {
				unlock := db.LockKeys(keys)
				for _, k := range keys {
					db.Set(string(k), value.MakeString([]byte("x")))
				}
				unlock()
			}
		}()
	}
	wg.Wait() // reaching here means the fixed lock order avoided deadlock
}

func allShardsLen(db *DB) int {
	unlock := db.LockAll()
	defer unlock()
	return db.Len()
}
