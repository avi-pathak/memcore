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
)

// ErrInvalid is the sentinel that every validation failure wraps, so callers
// can branch with errors.Is without depending on message text.
var ErrInvalid = errors.New("config: invalid")

// Config is the complete runtime configuration. It grows as features land;
// each field is owned by the subsystem named after its group.
type Config struct {
	Network Network
	Log     Log
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

// Default returns a valid configuration with conservative defaults. The bind
// host is loopback on purpose: exposing the listener is an explicit operator
// decision, not a default. Port 6380 avoids colliding with a local Redis.
func Default() Config {
	return Config{
		Network: Network{
			Host:      "127.0.0.1",
			Port:      6380,
			Databases: 16,
			Shards:    runtime.GOMAXPROCS(0),
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
