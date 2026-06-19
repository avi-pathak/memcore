// Package keyspace holds the key space for one shard of a logical database.
//
// A Keyspace is a plain data structure with no locks of its own; the shard that
// owns it provides synchronization.
package keyspace

import (
	"time"

	"github.com/avinashpathak/memcore/internal/clock"
	"github.com/avinashpathak/memcore/internal/value"
)

// Keyspace owns the key-to-entry map for one shard of a logical database. It is
// not safe for concurrent use; the owning shard provides synchronization.
//
// Reads (Get, Lookup, Exists) do not modify the map, so a shard may serve them
// under a shared read lock. An expired entry is hidden from reads but reclaimed
// only by a write or by active expiry. Writes mutate the map and run under the
// shard's exclusive lock.
type Keyspace struct {
	clock   clock.Clock
	entries map[string]Entry
	// withTTL indexes the keys that carry an expiration, so active expiry can
	// sample only volatile keys rather than scanning the whole keyspace.
	withTTL map[string]struct{}
}

// New returns an empty Keyspace that reads time from clk.
func New(clk clock.Clock) *Keyspace {
	return &Keyspace{
		clock:   clk,
		entries: make(map[string]Entry),
		withTTL: make(map[string]struct{}),
	}
}

// peek returns the entry for key when present and unexpired. It does not modify
// the map: an expired entry is reported absent but left in place, to be
// reclaimed by a write or by active expiry. That is what lets reads run under a
// shared lock.
func (k *Keyspace) peek(key string) (Entry, bool) {
	e, ok := k.entries[key]
	if !ok || e.expired(k.clock.Now()) {
		return Entry{}, false
	}
	return e, true
}

// Lookup returns the live entry for key.
func (k *Keyspace) Lookup(key string) (Entry, bool) { return k.peek(key) }

// Get returns the live value for key.
func (k *Keyspace) Get(key string) (value.Value, bool) {
	e, ok := k.peek(key)
	if !ok {
		return value.Value{}, false
	}
	return e.Value, true
}

// Exists reports whether key is present and unexpired.
func (k *Keyspace) Exists(key string) bool {
	_, ok := k.peek(key)
	return ok
}

// Set stores v under key with no expiration, discarding any previous TTL. This
// is the SET command's semantics.
func (k *Keyspace) Set(key string, v value.Value) {
	k.entries[key] = Entry{Value: v}
	delete(k.withTTL, key)
}

// SetEntry stores a fully formed entry, including its expiration. A caller that
// must preserve an existing TTL reads the entry, replaces its Value, and stores
// it back through SetEntry.
func (k *Keyspace) SetEntry(key string, e Entry) {
	k.entries[key] = e
	if e.HasExpiry() {
		k.withTTL[key] = struct{}{}
	} else {
		delete(k.withTTL, key)
	}
}

// SetExpire attaches the deadline at to an existing key and reports whether the
// key was present.
func (k *Keyspace) SetExpire(key string, at time.Time) bool {
	e, ok := k.peek(key)
	if !ok {
		return false
	}
	e.ExpireAt = at
	k.entries[key] = e
	k.withTTL[key] = struct{}{}
	return true
}

// Persist removes any TTL from key and reports whether a TTL was cleared.
func (k *Keyspace) Persist(key string) bool {
	e, ok := k.peek(key)
	if !ok || !e.HasExpiry() {
		return false
	}
	e.ExpireAt = time.Time{}
	k.entries[key] = e
	delete(k.withTTL, key)
	return true
}

// Delete removes key and reports whether it was live beforehand. An expired
// entry is removed too, but reported as absent.
func (k *Keyspace) Delete(key string) bool {
	e, ok := k.entries[key]
	if !ok {
		return false
	}
	delete(k.entries, key)
	delete(k.withTTL, key)
	return !e.expired(k.clock.Now())
}

// Take removes key and returns its entry, reporting whether it was live. It is
// the basis for UNLINK, which hands the removed value to a background reaper.
func (k *Keyspace) Take(key string) (Entry, bool) {
	e, ok := k.entries[key]
	if !ok {
		return Entry{}, false
	}
	delete(k.entries, key)
	delete(k.withTTL, key)
	return e, !e.expired(k.clock.Now())
}

// SampleExpired examines up to limit keys that carry a TTL, evicting those past
// their deadline at now. It reports how many it examined and how many it
// evicted, so a caller can decide whether to keep going. It must run under the
// shard's exclusive lock.
func (k *Keyspace) SampleExpired(now time.Time, limit int) (examined, evicted int) {
	for key := range k.withTTL {
		if examined >= limit {
			break
		}
		examined++
		e, ok := k.entries[key]
		if !ok || !e.HasExpiry() {
			delete(k.withTTL, key) // stale index entry
			continue
		}
		if e.expired(now) {
			delete(k.entries, key)
			delete(k.withTTL, key)
			evicted++
		}
	}
	return examined, evicted
}

// Len reports the number of stored entries, including any that have expired but
// have not yet been reclaimed.
func (k *Keyspace) Len() int { return len(k.entries) }

// Range calls fn for every stored entry in unspecified order. It is read-only
// and used for snapshotting; the caller must hold the shard's lock.
func (k *Keyspace) Range(fn func(key string, e Entry)) {
	for key, e := range k.entries {
		fn(key, e)
	}
}

// Flush removes every entry.
func (k *Keyspace) Flush() {
	clear(k.entries)
	clear(k.withTTL)
}
