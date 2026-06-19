package server

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/avinashpathak/memcore/internal/clock"
	"github.com/avinashpathak/memcore/internal/command"
	"github.com/avinashpathak/memcore/internal/config"
)

// recordingClock advances by a fixed step on each Now call, so a single command
// appears to take that long without any real delay.
type recordingClock struct {
	mu   sync.Mutex
	now  time.Time
	step time.Duration
}

func (c *recordingClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := c.now
	c.now = c.now.Add(c.step)
	return t
}

type fakeRecorder struct {
	mu       sync.Mutex
	commands map[string]int
	errors   map[string]int
}

func newFakeRecorder() *fakeRecorder {
	return &fakeRecorder{commands: map[string]int{}, errors: map[string]int{}}
}

func (r *fakeRecorder) ObserveCommand(name string, _ time.Duration, isError bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.commands[name]++
	if isError {
		r.errors[name]++
	}
}
func (r *fakeRecorder) ConnectionOpened() {}
func (r *fakeRecorder) ConnectionClosed() {}

func (r *fakeRecorder) count(name string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.commands[name]
}

func TestServerRecordsCommandMetrics(t *testing.T) {
	cfg := config.Default()
	cfg.Network.Host = "127.0.0.1"
	cfg.Network.Port = 0
	srv := New(cfg, clock.SystemClock{}, discardLogger(), command.NewRegistry())
	rec := newFakeRecorder()
	srv.SetRecorder(rec)

	addr, stop := serveOn(t, srv)
	defer stop()

	c := dialTestClient(t, addr)
	c.send("PING")
	c.line()
	c.send("SET", "k", "v")
	c.line()

	waitFor(t, func() bool { return rec.count("ping") == 1 && rec.count("set") == 1 })
}

func TestServerLogsSlowCommands(t *testing.T) {
	var logBuf syncBuffer
	logger := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelWarn}))

	cfg := config.Default()
	cfg.Network.Host = "127.0.0.1"
	cfg.Network.Port = 0
	cfg.Metrics.SlowThreshold = time.Millisecond

	// A clock that jumps 5ms between the two reads bracketing each command makes
	// every command cross the 1ms slow threshold deterministically.
	clk := &recordingClock{now: time.Unix(0, 0), step: 5 * time.Millisecond}
	srv := New(cfg, clk, logger, command.NewRegistry())

	addr, stop := serveOn(t, srv)
	defer stop()

	c := dialTestClient(t, addr)
	c.send("SET", "k", "v")
	c.line()

	waitFor(t, func() bool { return strings.Contains(logBuf.String(), "slow command") })
}

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func serveOn(t *testing.T, srv *Server) (addr string, stop func()) {
	t.Helper()
	if err := srv.Listen(); err != nil {
		t.Fatalf("Listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- srv.Serve(ctx) }()
	return srv.Addr().String(), func() {
		cancel()
		<-done
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition not met within the deadline")
}
