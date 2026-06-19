package config

import (
	"fmt"
	"strconv"
	"sync/atomic"
	"time"
)

// Reloadable is the subset of configuration that can change at runtime through
// CONFIG SET, without a restart. Each field maps to one documented parameter.
type Reloadable struct {
	SlowThreshold  time.Duration
	ExpirySample   int
	SlowLogEnabled bool
}

// Settings holds the live, reloadable configuration behind an atomic pointer, so
// readers on the command path never block and a CONFIG SET is visible to
// subsequent commands without locking. It is safe for concurrent use.
type Settings struct {
	v atomic.Pointer[Reloadable]
}

// NewSettings returns a Settings seeded from the boot configuration.
func NewSettings(init Reloadable) *Settings {
	s := &Settings{}
	s.v.Store(&init)
	return s
}

// Load returns the current reloadable configuration.
func (s *Settings) Load() Reloadable { return *s.v.Load() }

// names lists the CONFIG parameters in canonical form. They are lowercase with
// hyphen separators, matching what CONFIG GET reports.
const (
	paramSlowThreshold = "slowlog-threshold-ms"
	paramExpirySample  = "expiry-sample-per-shard"
	paramSlowLog       = "slowlog-enabled"
)

// Get returns the string value of a reloadable parameter, or false if the name
// is not reloadable.
func (s *Settings) Get(name string) (string, bool) {
	r := s.Load()
	switch name {
	case paramSlowThreshold:
		return strconv.FormatInt(r.SlowThreshold.Milliseconds(), 10), true
	case paramExpirySample:
		return strconv.Itoa(r.ExpirySample), true
	case paramSlowLog:
		return boolParam(r.SlowLogEnabled), true
	default:
		return "", false
	}
}

// All returns every reloadable parameter as name/value pairs, for CONFIG GET
// with a wildcard.
func (s *Settings) All() [][2]string {
	r := s.Load()
	return [][2]string{
		{paramSlowThreshold, strconv.FormatInt(r.SlowThreshold.Milliseconds(), 10)},
		{paramExpirySample, strconv.Itoa(r.ExpirySample)},
		{paramSlowLog, boolParam(r.SlowLogEnabled)},
	}
}

// Set updates one reloadable parameter, validating the value. An unknown name or
// an invalid value returns an error and leaves the settings unchanged.
func (s *Settings) Set(name, val string) error {
	next := s.Load()
	switch name {
	case paramSlowThreshold:
		ms, err := strconv.Atoi(val)
		if err != nil || ms < 0 {
			return fmt.Errorf("%w: %s must be a non-negative integer", ErrInvalid, name)
		}
		next.SlowThreshold = time.Duration(ms) * time.Millisecond
	case paramExpirySample:
		n, err := strconv.Atoi(val)
		if err != nil || n < 0 {
			return fmt.Errorf("%w: %s must be a non-negative integer", ErrInvalid, name)
		}
		next.ExpirySample = n
	case paramSlowLog:
		b, ok := parseBoolParam(val)
		if !ok {
			return fmt.Errorf("%w: %s must be yes or no", ErrInvalid, name)
		}
		next.SlowLogEnabled = b
	default:
		return fmt.Errorf("%w: unknown or non-reloadable parameter %q", ErrInvalid, name)
	}
	s.v.Store(&next)
	return nil
}

func boolParam(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func parseBoolParam(s string) (bool, bool) {
	switch s {
	case "yes", "1", "true":
		return true, true
	case "no", "0", "false":
		return false, true
	default:
		return false, false
	}
}
