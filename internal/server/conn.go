package server

import (
	"errors"
	"io"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/avinashpathak/memcore/internal/clock"
	"github.com/avinashpathak/memcore/internal/command"
	"github.com/avinashpathak/memcore/internal/resp"
	"github.com/avinashpathak/memcore/internal/shard"
)

// conn is one client connection. It runs on a single goroutine and is not
// shared, so it holds no locks of its own; its session carries the selected
// database for the commands that flow over it.
type conn struct {
	nc      net.Conn
	reader  *resp.Reader
	writer  *resp.Writer
	session *command.Context
}

func (s *Server) serve(c *conn) {
	defer s.closeConn(c)
	if s.metrics != nil {
		s.metrics.ConnectionOpened()
		defer s.metrics.ConnectionClosed()
	}
	remote := c.nc.RemoteAddr().String()
	s.log.Debug("connection opened", "remote", remote)

	for {
		args, err := c.reader.ReadRequest()
		if err != nil {
			s.handleReadError(c, err, remote)
			return
		}
		if len(args) == 0 {
			continue // empty inline line
		}

		reply := s.dispatch(c, args)
		if err := c.writer.WriteReply(reply); err != nil {
			return
		}
		// Hold replies back while more pipelined requests are already buffered,
		// then flush the batch in one write.
		if c.reader.Buffered() == 0 {
			if err := c.writer.Flush(); err != nil {
				return
			}
		}
	}
}

// dispatch times one command, records its metrics, and logs it when it crosses
// the slow-command threshold. The measurement spans lock acquisition and
// execution, which is what an operator cares about.
func (s *Server) dispatch(c *conn, args [][]byte) resp.Reply {
	start := s.clock.Now()
	reply := s.execute(c, args)
	elapsed := s.clock.Now().Sub(start)

	name := commandName(args)
	if s.metrics != nil {
		s.metrics.ObserveCommand(name, elapsed, reply.IsError())
	}
	s.maybeLogSlow(c, name, args, elapsed)
	return reply
}

func (s *Server) maybeLogSlow(c *conn, name string, args [][]byte, elapsed time.Duration) {
	r := c.session.Settings.Load()
	if !r.SlowLogEnabled || r.SlowThreshold <= 0 || elapsed < r.SlowThreshold {
		return
	}
	s.log.Warn("slow command",
		"command", name,
		"args", len(args)-1,
		"duration", elapsed,
		"db", c.session.DB(),
	)
}

func commandName(args [][]byte) string {
	if len(args) == 0 {
		return ""
	}
	return strings.ToLower(string(args[0]))
}

// execute runs one command under the shard locks it needs and recovers from a
// panic, so a single misbehaving command cannot take down the server. This
// recovery is the connection boundary the design relies on.
func (s *Server) execute(c *conn, args [][]byte) (reply resp.Reply) {
	defer func() {
		if r := recover(); r != nil {
			s.log.Error("recovered from panic during command execution",
				"command", string(args[0]), "panic", r)
			reply = resp.Error("ERR internal server error")
		}
	}()

	cmd, errReply, ok := s.registry.Resolve(args)
	if !ok {
		return errReply
	}
	db := s.databases[c.session.DB()]
	unlock := lockShards(db, cmd, args)
	defer unlock()
	reply = cmd.Run(c.session, args)
	// Log successful writes under the shard lock, so same-key writes reach the
	// log in the order they were applied.
	if s.persist != nil && !cmd.ReadOnly() && !reply.IsError() {
		if err := s.persist.Append(c.session.DB(), persistArgs(s.clock, cmd, args)); err != nil {
			s.log.Error("append-log write failed", "command", cmd.Name(), "error", err)
		}
	}
	return reply
}

// persistArgs returns the form of a write command to record. EXPIRE is rewritten
// to PEXPIREAT with an absolute deadline so replay does not depend on when it
// runs; other commands are logged verbatim.
func persistArgs(clk clock.Clock, cmd command.Command, args [][]byte) [][]byte {
	if cmd.Name() == "expire" && len(args) == 3 {
		if secs, err := strconv.ParseInt(string(args[2]), 10, 64); err == nil {
			ms := clk.Now().Add(time.Duration(secs) * time.Second).UnixMilli()
			return [][]byte{[]byte("PEXPIREAT"), args[1], []byte(strconv.FormatInt(ms, 10))}
		}
	}
	return args
}

// lockShards takes the locks a command needs before it runs: every shard for a
// whole-database command, a shared lock on the touched shards for a read-only
// command, or an exclusive lock for a write. The returned function releases
// them.
func lockShards(db *shard.DB, cmd command.Command, args [][]byte) func() {
	switch {
	case cmd.WholeDB():
		return db.LockAll()
	case cmd.ReadOnly():
		return db.RLockKeys(cmd.Keys(args))
	default:
		return db.LockKeys(cmd.Keys(args))
	}
}

func (s *Server) handleReadError(c *conn, err error, remote string) {
	switch {
	case errors.Is(err, io.EOF):
		s.log.Debug("connection closed by client", "remote", remote)
	case errors.Is(err, resp.ErrProtocol):
		// Report the violation, then let the caller close the connection.
		_ = c.writer.WriteReply(resp.Error("ERR Protocol error"))
		_ = c.writer.Flush()
		s.log.Debug("protocol error", "remote", remote, "error", err)
	default:
		// Abrupt disconnects and shutdown read-deadline timeouts land here.
		s.log.Debug("connection ended", "remote", remote, "error", err)
	}
}
