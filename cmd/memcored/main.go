// Command memcored is the Memcore server. This file is the composition root:
// it parses configuration, builds the object graph by hand, runs it, and shuts
// it down. No dependency-injection framework is involved.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/avinashpathak/memcore/internal/clock"
	"github.com/avinashpathak/memcore/internal/command"
	"github.com/avinashpathak/memcore/internal/config"
	"github.com/avinashpathak/memcore/internal/metrics"
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

	var healthcheck bool
	logLevel := cfg.Log.Level.String()
	flag.StringVar(&cfg.Network.Host, "host", cfg.Network.Host, "address to bind the listener")
	flag.IntVar(&cfg.Network.Port, "port", cfg.Network.Port, "port to listen on")
	flag.IntVar(&cfg.Network.Databases, "databases", cfg.Network.Databases, "number of logical databases")
	flag.IntVar(&cfg.Network.Shards, "shards", cfg.Network.Shards, "shards per database (default GOMAXPROCS)")
	flag.BoolVar(&cfg.Persistence.Enabled, "persistence", cfg.Persistence.Enabled, "enable on-disk persistence")
	flag.StringVar(&cfg.Persistence.Dir, "data-dir", cfg.Persistence.Dir, "directory for the snapshot and append log")
	flag.StringVar(&cfg.Persistence.FSync, "fsync", cfg.Persistence.FSync, "append-log fsync policy: always, everysec, or no")
	flag.BoolVar(&cfg.Metrics.Enabled, "metrics", cfg.Metrics.Enabled, "expose Prometheus metrics over HTTP")
	flag.IntVar(&cfg.Metrics.Port, "metrics-port", cfg.Metrics.Port, "port for the Prometheus metrics endpoint")
	flag.StringVar(&cfg.Log.Format, "log-format", cfg.Log.Format, "log format: text or json")
	flag.StringVar(&logLevel, "log-level", logLevel, "log level: debug, info, warn, error")
	flag.BoolVar(&healthcheck, "healthcheck", false, "PING the configured port and exit; used by the container healthcheck")
	flag.Parse()

	if healthcheck {
		return ping(cfg.Network.Addr())
	}

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

	var wg sync.WaitGroup
	if cfg.Metrics.Enabled {
		if err := startMetrics(ctx, &wg, cfg.Metrics, srv, logger); err != nil {
			return err
		}
	}

	if err := srv.Run(ctx); err != nil {
		return err
	}
	wg.Wait()
	logger.Info("shutdown complete")
	return nil
}

// startMetrics binds the metrics listener before serving begins, so a bind
// failure aborts startup rather than surfacing on a background goroutine, and
// wires the collectors into the server.
func startMetrics(ctx context.Context, wg *sync.WaitGroup, cfg config.Metrics, srv *server.Server, logger *slog.Logger) error {
	collectors := metrics.New()
	srv.SetRecorder(collectors)

	ln, err := net.Listen("tcp", cfg.Addr())
	if err != nil {
		return fmt.Errorf("metrics listen on %s: %w", cfg.Addr(), err)
	}
	ms := metrics.NewServer(cfg.Addr(), collectors)
	logger.Info("metrics endpoint listening", "addr", cfg.Addr(), "path", "/metrics")

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := ms.Serve(ln); err != nil {
			logger.Error("metrics server failed", "error", err)
		}
	}()
	go func() {
		<-ctx.Done()
		if err := ms.Shutdown(); err != nil {
			logger.Error("metrics server shutdown failed", "error", err)
		}
	}()
	return nil
}

func newLogger(c config.Log) *slog.Logger {
	opts := &slog.HandlerOptions{Level: c.Level}
	if c.Format == "json" {
		return slog.New(slog.NewJSONHandler(os.Stderr, opts))
	}
	return slog.New(slog.NewTextHandler(os.Stderr, opts))
}

// ping connects to addr, issues a RESP PING, and verifies the +PONG reply. It
// backs the container healthcheck, so it depends on nothing outside the standard
// library and the running server.
func ping(addr string) error {
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err := conn.Write([]byte("*1\r\n$4\r\nPING\r\n")); err != nil {
		return err
	}
	buf := make([]byte, 7) // "+PONG\r\n"
	if _, err := io.ReadFull(conn, buf); err != nil {
		return err
	}
	if string(buf) != "+PONG\r\n" {
		return fmt.Errorf("unexpected ping reply %q", buf)
	}
	return nil
}
