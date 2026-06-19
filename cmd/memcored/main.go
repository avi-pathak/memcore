// Command memcored is the Memcore server. This file is the composition root:
// it parses configuration, builds the object graph by hand, runs it, and shuts
// it down. No dependency-injection framework is involved.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/avinashpathak/memcore/internal/clock"
	"github.com/avinashpathak/memcore/internal/command"
	"github.com/avinashpathak/memcore/internal/config"
	"github.com/avinashpathak/memcore/internal/server"
)

// Build metadata, injected with -ldflags "-X main.version=... -X main.commit=...".
// The defaults are what an unstamped local build reports.
var (
	version = "dev"
	commit  = "none"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "memcored:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg := config.Default()

	logLevel := cfg.Log.Level.String()
	flag.StringVar(&cfg.Network.Host, "host", cfg.Network.Host, "address to bind the listener")
	flag.IntVar(&cfg.Network.Port, "port", cfg.Network.Port, "port to listen on")
	flag.IntVar(&cfg.Network.Databases, "databases", cfg.Network.Databases, "number of logical databases")
	flag.IntVar(&cfg.Network.Shards, "shards", cfg.Network.Shards, "shards per database (default GOMAXPROCS)")
	flag.StringVar(&cfg.Log.Format, "log-format", cfg.Log.Format, "log format: text or json")
	flag.StringVar(&logLevel, "log-level", logLevel, "log level: debug, info, warn, error")
	flag.Parse()

	if err := cfg.Log.Level.UnmarshalText([]byte(logLevel)); err != nil {
		return fmt.Errorf("log level: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return err
	}

	logger := newLogger(cfg.Log)
	logger.Info("starting memcored",
		"version", version,
		"commit", commit,
		"addr", cfg.Network.Addr(),
		"databases", cfg.Network.Databases,
	)

	srv := server.New(cfg, clock.SystemClock{}, logger, command.NewRegistry())

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := srv.Run(ctx); err != nil {
		return err
	}
	logger.Info("shutdown complete")
	return nil
}

func newLogger(c config.Log) *slog.Logger {
	opts := &slog.HandlerOptions{Level: c.Level}
	if c.Format == "json" {
		return slog.New(slog.NewJSONHandler(os.Stderr, opts))
	}
	return slog.New(slog.NewTextHandler(os.Stderr, opts))
}
