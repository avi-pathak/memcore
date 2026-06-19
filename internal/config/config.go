// Package config defines Memcore's typed configuration. A Config is built once
// at boot from defaults and operator overrides, validated, and then treated as
// read-only by the rest of the system. A documented subset becomes reloadable
// in a later stage; until then callers may read it without synchronization.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"runtime"
	"strconv"
	"time"

	"github.com/avinashpathak/memcore/internal/value"
)

// ErrInvalid is the sentinel that every validation failure wraps, so callers
// can branch with errors.Is without depending on message text.
var ErrInvalid = errors.New("config: invalid")

// Config is the complete runtime configuration. It grows as features land;
// each field is owned by the subsystem named after its group.
type Config struct {
	Network     Network
	Limits      value.Limits
	Expiry      Expiry
	Persistence Persistence
	Log         Log
}

// Network holds the listener and logical-database settings.
type Network struct {
	Host      string
	Port      int
	Databases int
	Shards    int // shards per database; each shard is locked independently
}

// Log holds structured-logging settings.
type Log struct {
	Level  slog.Level
	Format string // "text" or "json"
}

// Expiry tunes the background active-expiry loop.
type Expiry struct {
	Interval       time.Duration // time between cycles; 0 disables active expiry
	SamplePerShard int           // keys examined per shard per cycle
}

// Persistence configures durability. It is off by default: writing to disk is
// an explicit operator decision.
type Persistence struct {
	Enabled      bool
	Dir          string        // directory holding the snapshot and append log
	FSync        string        // "always", "everysec", or "no"
	CompactEvery time.Duration // append-log compaction interval; 0 disables it
}

// Default returns a valid configuration with conservative defaults. The bind
// host is loopback on purpose: exposing the listener is an explicit operator
// decision, not a default. Port 6380 avoids colliding with a local Redis.
func Default() Config {
	compact := value.Thresholds{MaxEntries: 128, MaxBytes: 64}
	return Config{
		Network: Network{
			Host:      "127.0.0.1",
			Port:      6380,
			Databases: 16,
			Shards:    runtime.GOMAXPROCS(0),
		},
		Limits: value.Limits{
			List: compact,
			Hash: compact,
			Set:  compact,
			ZSet: compact,
		},
		Expiry: Expiry{
			Interval:       100 * time.Millisecond,
			SamplePerShard: 20,
		},
		Persistence: Persistence{
			Enabled:      false,
			Dir:          "data",
			FSync:        "everysec",
			CompactEvery: 5 * time.Minute,
		},
		Log: Log{
			Level:  slog.LevelInfo,
			Format: "text",
		},
	}
}

// Validate reports the first invariant the configuration violates. Every
// returned error wraps ErrInvalid.
func (c Config) Validate() error {
	if c.Network.Host == "" {
		return fmt.Errorf("%w: network host must not be empty", ErrInvalid)
	}
	if c.Network.Port < 1 || c.Network.Port > 65535 {
		return fmt.Errorf("%w: network port %d out of range 1-65535", ErrInvalid, c.Network.Port)
	}
	if c.Network.Databases < 1 {
		return fmt.Errorf("%w: databases must be at least 1, got %d", ErrInvalid, c.Network.Databases)
	}
	if c.Network.Shards < 1 {
		return fmt.Errorf("%w: shards must be at least 1, got %d", ErrInvalid, c.Network.Shards)
	}
	for _, th := range []value.Thresholds{c.Limits.List, c.Limits.Hash, c.Limits.Set, c.Limits.ZSet} {
		if th.MaxEntries < 0 || th.MaxBytes < 0 {
			return fmt.Errorf("%w: compact-encoding thresholds must not be negative", ErrInvalid)
		}
	}
	if c.Expiry.Interval < 0 {
		return fmt.Errorf("%w: expiry interval must not be negative", ErrInvalid)
	}
	if c.Expiry.SamplePerShard < 0 {
		return fmt.Errorf("%w: expiry sample size must not be negative", ErrInvalid)
	}
	if c.Persistence.Enabled {
		if c.Persistence.Dir == "" {
			return fmt.Errorf("%w: persistence directory must be set when persistence is enabled", ErrInvalid)
		}
		switch c.Persistence.FSync {
		case "always", "everysec", "no":
		default:
			return fmt.Errorf("%w: fsync policy %q must be \"always\", \"everysec\", or \"no\"", ErrInvalid, c.Persistence.FSync)
		}
	}
	switch c.Log.Format {
	case "text", "json":
	default:
		return fmt.Errorf("%w: log format %q must be \"text\" or \"json\"", ErrInvalid, c.Log.Format)
	}
	return nil
}

// Addr returns the host:port the listener binds, formatted so IPv6 literals are
// bracketed correctly.
func (n Network) Addr() string {
	return net.JoinHostPort(n.Host, strconv.Itoa(n.Port))
}
