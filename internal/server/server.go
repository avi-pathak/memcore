// Package server runs the TCP listener and owns every other layer. It is the
// only package that touches sockets, and the only one that turns errors into
// RESP error replies.
package server

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/avinashpathak/memcore/internal/clock"
	"github.com/avinashpathak/memcore/internal/command"
	"github.com/avinashpathak/memcore/internal/config"
	"github.com/avinashpathak/memcore/internal/expiry"
	"github.com/avinashpathak/memcore/internal/keyspace"
	"github.com/avinashpathak/memcore/internal/persistence"
	"github.com/avinashpathak/memcore/internal/resp"
	"github.com/avinashpathak/memcore/internal/shard"
	"github.com/avinashpathak/memcore/internal/value"
)

// Server accepts client connections and serves RESP commands. It is safe for
// concurrent use: each connection runs on its own goroutine, and the only
// shared mutable state is the databases, each of which locks its own shards.
type Server struct {
	cfg      config.Network
	clock    clock.Clock
	log      *slog.Logger
	registry *command.Registry

	databases  []*shard.DB
	limits     value.Limits
	expiry     *expiry.Runner
	reaper     chan value.Value
	persist    *persistence.Store
	persistCfg config.Persistence

	mu       sync.Mutex
	listener net.Listener
	conns    map[*conn]struct{}
	closing  bool
	wg       sync.WaitGroup
}

// New builds a server with cfg.Network.Databases independent databases, each
// split into cfg.Network.Shards shards.
func New(cfg config.Config, clk clock.Clock, log *slog.Logger, registry *command.Registry) *Server {
	dbs := make([]*shard.DB, cfg.Network.Databases)
	for i := range dbs {
		dbs[i] = shard.New(cfg.Network.Shards, clk)
	}
	s := &Server{
		cfg:        cfg.Network,
		clock:      clk,
		log:        log,
		registry:   registry,
		databases:  dbs,
		limits:     cfg.Limits,
		persistCfg: cfg.Persistence,
		conns:      make(map[*conn]struct{}),
		reaper:     make(chan value.Value, reaperBuffer),
	}
	s.expiry = expiry.New(dbs, clk, expiry.Config{
		Interval:       cfg.Expiry.Interval,
		SamplePerShard: cfg.Expiry.SamplePerShard,
	}, log)
	for _, db := range dbs {
		db.SetReaper(s.reapAsync)
	}
	return s
}

// Listen binds the configured address. Splitting it from Serve lets a caller
// learn the bound address (useful when the OS chooses the port) before any
// connection is accepted.
func (s *Server) Listen() error {
	ln, err := net.Listen("tcp", s.cfg.Addr())
	if err != nil {
		return fmt.Errorf("listen on %s: %w", s.cfg.Addr(), err)
	}
	s.mu.Lock()
	s.listener = ln
	s.mu.Unlock()
	return nil
}

// Addr returns the bound address, or nil before Listen succeeds.
func (s *Server) Addr() net.Addr {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener == nil {
		return nil
	}
	return s.listener.Addr()
}

// Serve accepts connections until ctx is canceled, then stops accepting, drains
// in-flight connections, and returns. Listen must have run first.
func (s *Server) Serve(ctx context.Context) error {
	s.mu.Lock()
	ln := s.listener
	s.mu.Unlock()
	if ln == nil {
		return errors.New("server: Serve called before Listen")
	}
	s.log.Info("listening", "addr", ln.Addr().String())

	go func() {
		<-ctx.Done()
		s.shutdown()
	}()

	s.wg.Add(2)
	go func() { defer s.wg.Done(); s.runReaper(ctx) }()
	go func() { defer s.wg.Done(); s.expiry.Run(ctx) }()

	if s.persist != nil {
		s.wg.Add(2)
		go func() { defer s.wg.Done(); s.runAppendLogFlush(ctx) }()
		go func() { defer s.wg.Done(); s.runCompaction(ctx) }()
	}

	for {
		nc, err := ln.Accept()
		if err != nil {
			if s.isClosing() {
				break
			}
			s.log.Warn("accept failed", "error", err)
			continue
		}
		s.startConn(nc)
	}

	s.wg.Wait()
	if s.persist != nil {
		if err := s.finalizePersistence(); err != nil {
			s.log.Error("final snapshot failed", "error", err)
		}
	}
	s.log.Info("server stopped")
	return nil
}

// Run binds and serves in one call. main uses it; tests use Listen and Serve
// separately so they can read the OS-chosen port.
func (s *Server) Run(ctx context.Context) error {
	if err := s.openPersistence(); err != nil {
		return err
	}
	if err := s.Listen(); err != nil {
		return err
	}
	return s.Serve(ctx)
}

func (s *Server) startConn(nc net.Conn) {
	c := &conn{
		nc:      nc,
		reader:  resp.NewReader(nc),
		writer:  resp.NewWriter(nc),
		session: command.NewContext(s.clock, s.databases, s.limits),
	}
	s.mu.Lock()
	if s.closing {
		s.mu.Unlock()
		_ = nc.Close()
		return
	}
	s.conns[c] = struct{}{}
	s.wg.Add(1)
	s.mu.Unlock()

	go func() {
		defer s.wg.Done()
		s.serve(c)
	}()
}

func (s *Server) closeConn(c *conn) {
	s.mu.Lock()
	delete(s.conns, c)
	s.mu.Unlock()
	_ = c.nc.Close()
}

func (s *Server) isClosing() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closing
}

// shutdown stops accepting and winds down open connections by closing the
// listener and tripping each connection's read deadline. A command already in
// flight finishes and writes its reply before the connection's next read
// returns the deadline error.
func (s *Server) shutdown() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closing {
		return
	}
	s.closing = true
	if s.listener != nil {
		_ = s.listener.Close()
	}
	for c := range s.conns {
		_ = c.nc.SetReadDeadline(time.Now())
	}
}

// reaperBuffer bounds how many freed values may queue for the reaper before
// UNLINK falls back to dropping them inline.
const reaperBuffer = 1024

// reapAsync hands v to the background reaper without blocking. If the reaper is
// busy, v is dropped here and reclaimed by the garbage collector like any other
// value; correctness never depends on the reaper draining.
func (s *Server) reapAsync(v value.Value) {
	select {
	case s.reaper <- v:
	default:
	}
}

// runReaper drains freed collection values until ctx is canceled. Receiving a
// value drops the last reference to it, so the garbage collector reclaims it off
// the command path.
func (s *Server) runReaper(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.reaper:
		}
	}
}

// openPersistence opens the data directory and recovers prior state. It is a
// no-op when persistence is disabled.
func (s *Server) openPersistence() error {
	if !s.persistCfg.Enabled {
		return nil
	}
	store, err := persistence.Open(s.persistCfg.Dir, s.persistCfg.FSync, s.limits)
	if err != nil {
		return fmt.Errorf("open persistence: %w", err)
	}
	s.persist = store
	return s.recover()
}

func (s *Server) recover() error {
	restore := func(rec persistence.Record) error {
		if rec.DB < 0 || rec.DB >= len(s.databases) {
			return fmt.Errorf("snapshot names database %d out of range", rec.DB)
		}
		e := keyspace.Entry{Value: rec.Value}
		if rec.ExpireAt != 0 {
			e.ExpireAt = time.Unix(0, rec.ExpireAt)
		}
		s.databases[rec.DB].Restore(rec.Key, e)
		return nil
	}
	session := command.NewContext(s.clock, s.databases, s.limits)
	replay := func(db int, args [][]byte) error {
		if err := session.Select(db); err != nil {
			return err
		}
		s.registry.Dispatch(session, args) // a replayed write is expected to succeed
		return nil
	}
	loaded, replayed, err := persistence.Recover(s.persistCfg.Dir, s.limits, restore, replay)
	if err != nil {
		return fmt.Errorf("recover: %w", err)
	}
	s.log.Info("recovered state", "snapshot_records", loaded, "replayed_commands", replayed)
	return nil
}

// runAppendLogFlush fsyncs the append log once a second under the everysec
// policy; the other policies fsync inline or leave it to the OS.
func (s *Server) runAppendLogFlush(ctx context.Context) {
	if s.persistCfg.FSync != persistence.FSyncEverySec {
		<-ctx.Done()
		return
	}
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := s.persist.Flush(); err != nil {
				s.log.Error("append-log flush failed", "error", err)
			}
		}
	}
}

func (s *Server) runCompaction(ctx context.Context) {
	if s.persistCfg.CompactEvery <= 0 {
		<-ctx.Done()
		return
	}
	t := time.NewTicker(s.persistCfg.CompactEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := s.compact(); err != nil {
				s.log.Error("compaction failed", "error", err)
			}
		}
	}
}

// compact captures the live state into an in-memory snapshot under a brief lock
// over every database, then writes it to disk and trims the append log off the
// lock. There is no process fork; the only pause is the in-memory capture.
func (s *Server) compact() error {
	baseGen := s.persist.NextGen()
	var buf bytes.Buffer

	unlock := s.lockAllDatabases()
	werr := persistence.WriteSnapshot(&buf, baseGen, s.emitRecords)
	_, rerr := s.persist.Rotate()
	unlock()

	if werr != nil {
		return werr
	}
	if rerr != nil {
		return rerr
	}
	return s.persist.InstallSnapshot(buf.Bytes(), baseGen)
}

func (s *Server) finalizePersistence() error {
	if err := s.persist.Flush(); err != nil {
		return err
	}
	if err := s.compact(); err != nil {
		return err
	}
	return s.persist.Close()
}

// lockAllDatabases write-locks every shard of every database, returning a
// function that releases them. Only compaction takes locks across databases, and
// always in this order, so it cannot deadlock against single-database commands.
func (s *Server) lockAllDatabases() func() {
	unlocks := make([]func(), len(s.databases))
	for i, db := range s.databases {
		unlocks[i] = db.LockAll()
	}
	return func() {
		for i := len(unlocks) - 1; i >= 0; i-- {
			unlocks[i]()
		}
	}
}

func (s *Server) emitRecords(write func(persistence.Record) error) error {
	for db := range s.databases {
		var err error
		s.databases[db].EachEntry(func(key string, e keyspace.Entry) {
			if err != nil {
				return
			}
			var expireAt int64
			if e.HasExpiry() {
				expireAt = e.ExpireAt.UnixNano()
			}
			err = write(persistence.Record{DB: db, Key: key, ExpireAt: expireAt, Value: e.Value})
		})
		if err != nil {
			return err
		}
	}
	return nil
}
