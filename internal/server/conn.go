package server

import (
	"errors"
	"io"
	"net"

	"github.com/avinashpathak/memcore/internal/command"
	"github.com/avinashpathak/memcore/internal/resp"
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

		reply := s.execute(c, args)
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

// execute runs one command under its database lock and recovers from a panic, so
// a single misbehaving command cannot take down the server. This recovery is the
// connection boundary the design relies on.
func (s *Server) execute(c *conn, args [][]byte) (reply resp.Reply) {
	defer func() {
		if r := recover(); r != nil {
			s.log.Error("recovered from panic during command execution",
				"command", string(args[0]), "panic", r)
			reply = resp.Error("ERR internal server error")
		}
	}()

	db := s.databases[c.session.DB()]
	db.mu.Lock()
	defer db.mu.Unlock()
	return s.registry.Dispatch(c.session, args)
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
