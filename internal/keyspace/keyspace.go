// Package keyspace holds the per-database key space and applies lazy expiry.
//
// A Keyspace is a plain data structure with no locks of its own. The shard that
// owns it serializes every call, so a read that evicts an expired entry is
// safe.
package keyspace

import (
	"time"

	"github.com/avinashpathak/memcore/internal/clock"
	"github.com/avinashpathak/memcore/internal/value"
)

// Keyspace owns the key-to-entry map for a single logical database. It is not
// safe for concurrent use; the owning shard provides synchronization.
//
// Expiry is lazy: a read that observes an entry past its deadline removes it and
// reports the key as absent. Because such a read mutates the map, the shard must
// hold its exclusive lock even for read commands.
type Keyspace struct {
	clock   clock.Clock
	entries map[string]Entry
}

// New returns an empty Keyspace that reads time from clk.
func New(clk clock.Clock) *Keyspace {
	return &Keyspace{clock: clk, entries: make(map[string]Entry)}
}

// live returns the entry for key when it is present and unexpired, evicting it
// lazily otherwise.
func (k *Keyspace) live(key string) (Entry, bool) {
	e, ok := k.entries[key]
	if !ok {
		return Entry{}, false
	}
	if e.expired(k.clock.Now()) {
		delete(k.entries, key)
		return Entry{}, false
	}
	return e, true
}

// Lookup returns the live entry for key.
func (k *Keyspace) Lookup(key string) (Entry, bool) { return k.live(key) }

// Get returns the live value for key.
func (k *Keyspace) Get(key string) (value.Value, bool) {
	e, ok := k.live(key)
	if !ok {
		return value.Value{}, false
	}
	return e.Value, true
}

// Exists reports whether key is present and unexpired.
func (k *Keyspace) Exists(key string) bool {
	_, ok := k.live(key)
	return ok
}

// Set stores v under key with no expiration, discarding any previous TTL. This
// is the SET command's semantics.
func (k *Keyspace) Set(key string, v value.Value) {
	k.entries[key] = Entry{Value: v}
}

// SetEntry stores a fully formed entry, including its expiration. A caller that
// must preserve an existing TTL reads the entry, replaces its Value, and stores
// it back through SetEntry.
func (k *Keyspace) SetEntry(key string, e Entry) {
	k.entries[key] = e
}

// SetExpire attaches the deadline at to an existing key and reports whether the
// key was present.
func (k *Keyspace) SetExpire(key string, at time.Time) bool {
	e, ok := k.live(key)
	if !ok {
		return false
	}
	e.ExpireAt = at
	k.entries[key] = e
	return true
}

// Persist removes any TTL from key and reports whether a TTL was cleared.
func (k *Keyspace) Persist(key string) bool {
	e, ok := k.live(key)
	if !ok || !e.HasExpiry() {
		return false
	}
	e.ExpireAt = time.Time{}
	k.entries[key] = e
	return true
}

// Delete removes key and reports whether it was present beforehand.
func (k *Keyspace) Delete(key string) bool {
	if _, ok := k.live(key); !ok {
		return false
	}
	delete(k.entries, key)
	return true
}

// Len reports the number of stored entries, including any that have expired but
// have not yet been reclaimed.
func (k *Keyspace) Len() int { return len(k.entries) }

// Flush removes every entry.
func (k *Keyspace) Flush() { clear(k.entries) }
