package keyspace

import (
	"time"

	"github.com/avinashpathak/memcore/internal/value"
)

// Entry is a stored value together with its optional expiration. The zero
// ExpireAt means the entry does not expire.
type Entry struct {
	Value    value.Value
	ExpireAt time.Time
}

// HasExpiry reports whether the entry carries an expiration deadline.
func (e Entry) HasExpiry() bool { return !e.ExpireAt.IsZero() }

// expired reports whether the entry's deadline is at or before now. Expiry is
// inclusive of the deadline, matching Redis.
func (e Entry) expired(now time.Time) bool {
	return e.HasExpiry() && !now.Before(e.ExpireAt)
}
