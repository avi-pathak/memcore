// Package clock provides an injectable time source. Expiry and any other
// time-dependent logic depends on a Clock rather than calling time.Now
// directly, so tests can advance time deterministically without sleeping.
package clock

import (
	"sync"
	"time"
)

// Clock reports the current time. Production code uses SystemClock; tests use
// ManualClock.
type Clock interface {
	Now() time.Time
}

// SystemClock reads the host wall clock. It is the only place in the codebase
// that calls time.Now. The zero value is ready to use and is safe for
// concurrent use.
type SystemClock struct{}

// Now returns the current wall-clock time.
func (SystemClock) Now() time.Time { return time.Now() }

// ManualClock is a Clock whose value advances only when explicitly told to. It
// is safe for concurrent use.
type ManualClock struct {
	mu  sync.Mutex
	now time.Time
}

// NewManualClock returns a ManualClock positioned at t.
func NewManualClock(t time.Time) *ManualClock {
	return &ManualClock{now: t}
}

// Now returns the clock's current instant.
func (c *ManualClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// Advance moves the clock forward by d.
func (c *ManualClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// Set positions the clock at t.
func (c *ManualClock) Set(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = t
}
