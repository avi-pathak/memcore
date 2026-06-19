// Package shard splits one logical database across a fixed number of
// independently locked shards, so commands on different keys run in parallel. A
// key is routed to a shard by hash; each shard owns a keyspace and an RWMutex.
package shard

import (
	"hash/maphash"
	"sort"
	"sync"
	"time"

	"github.com/avinashpathak/memcore/internal/clock"
	"github.com/avinashpathak/memcore/internal/keyspace"
	"github.com/avinashpathak/memcore/internal/value"
)

// seed fixes key-to-shard routing for the life of the process. Routing need not
// be stable across restarts: a snapshot stores keys and values, and a reload
// re-routes them.
var seed = maphash.MakeSeed()

// DB is a logical database split across shards. Its data methods (Get, Set, and
// the rest) route by key and assume the caller already holds the matching shard
// lock. The lock helpers (LockKeys, RLockKeys, LockAll) acquire shards in a
// fixed global order, so multi-key commands cannot deadlock against each other.
//
// DB is safe for concurrent use provided every command wraps its data-method
// calls in the lock the helpers return for that command's keys. A handler must
// touch only the keys it declared, so the executor locks the right shards.
type DB struct {
	shards []*keyspace.Keyspace
	mus    []sync.RWMutex
}

// New returns a database with the given number of shards (at least one), each
// reading time from clk.
func New(shards int, clk clock.Clock) *DB {
	if shards < 1 {
		shards = 1
	}
	db := &DB{
		shards: make([]*keyspace.Keyspace, shards),
		mus:    make([]sync.RWMutex, shards),
	}
	for i := range db.shards {
		db.shards[i] = keyspace.New(clk)
	}
	return db
}

// Shards reports the number of shards.
func (db *DB) Shards() int { return len(db.shards) }

func (db *DB) indexString(key string) int {
	return int(maphash.String(seed, key) % uint64(len(db.shards)))
}

func (db *DB) indexBytes(key []byte) int {
	return int(maphash.Bytes(seed, key) % uint64(len(db.shards)))
}

func (db *DB) keyspaceFor(key string) *keyspace.Keyspace {
	return db.shards[db.indexString(key)]
}

// The data methods mirror keyspace.Keyspace, routing each call to the shard that
// owns key. They do no locking of their own.

// Get returns the live value for key.
func (db *DB) Get(key string) (value.Value, bool) { return db.keyspaceFor(key).Get(key) }

// Lookup returns the live entry for key.
func (db *DB) Lookup(key string) (keyspace.Entry, bool) { return db.keyspaceFor(key).Lookup(key) }

// Exists reports whether key is present and unexpired.
func (db *DB) Exists(key string) bool { return db.keyspaceFor(key).Exists(key) }

// Set stores v under key with no expiration.
func (db *DB) Set(key string, v value.Value) { db.keyspaceFor(key).Set(key, v) }

// SetEntry stores a fully formed entry under key.
func (db *DB) SetEntry(key string, e keyspace.Entry) { db.keyspaceFor(key).SetEntry(key, e) }

// SetExpire attaches the deadline at to an existing key.
func (db *DB) SetExpire(key string, at time.Time) bool { return db.keyspaceFor(key).SetExpire(key, at) }

// Persist removes any TTL from key.
func (db *DB) Persist(key string) bool { return db.keyspaceFor(key).Persist(key) }

// Delete removes key and reports whether it was live.
func (db *DB) Delete(key string) bool { return db.keyspaceFor(key).Delete(key) }

// Flush empties every shard. The caller must hold every shard's write lock,
// which LockAll provides.
func (db *DB) Flush() {
	for _, ks := range db.shards {
		ks.Flush()
	}
}

// Len sums the entry counts of every shard. The caller must hold the relevant
// shard locks.
func (db *DB) Len() int {
	n := 0
	for _, ks := range db.shards {
		n += ks.Len()
	}
	return n
}

// LockKeys write-locks the shards covering keys, in ascending shard order, and
// returns a function that releases them.
func (db *DB) LockKeys(keys [][]byte) func() {
	idx := db.shardSet(keys)
	for _, i := range idx {
		db.mus[i].Lock()
	}
	return func() {
		for i := len(idx) - 1; i >= 0; i-- {
			db.mus[idx[i]].Unlock()
		}
	}
}

// RLockKeys read-locks the shards covering keys, in ascending shard order, and
// returns a function that releases them. Because reads do not mutate a keyspace,
// several read commands may share a shard.
func (db *DB) RLockKeys(keys [][]byte) func() {
	idx := db.shardSet(keys)
	for _, i := range idx {
		db.mus[i].RLock()
	}
	return func() {
		for i := len(idx) - 1; i >= 0; i-- {
			db.mus[idx[i]].RUnlock()
		}
	}
}

// LockAll write-locks every shard in order, for whole-database commands such as
// FLUSHDB.
func (db *DB) LockAll() func() {
	for i := range db.mus {
		db.mus[i].Lock()
	}
	return func() {
		for i := len(db.mus) - 1; i >= 0; i-- {
			db.mus[i].Unlock()
		}
	}
}

// shardSet returns the distinct shard indices for keys, sorted ascending so that
// every caller acquires locks in the same global order. The common single-key
// case skips the map and the sort.
func (db *DB) shardSet(keys [][]byte) []int {
	switch len(keys) {
	case 0:
		return nil
	case 1:
		return []int{db.indexBytes(keys[0])}
	}
	seen := make(map[int]struct{}, len(keys))
	idx := make([]int, 0, len(keys))
	for _, k := range keys {
		i := db.indexBytes(k)
		if _, dup := seen[i]; !dup {
			seen[i] = struct{}{}
			idx = append(idx, i)
		}
	}
	sort.Ints(idx)
	return idx
}
