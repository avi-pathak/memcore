// Package command implements Memcore's command set as a registry of handlers.
// Each command is pure and synchronous: it reads and writes the keyspace and
// clock carried by its Context and returns a resp.Reply. Commands never touch a
// socket and never reach for globals, which is what makes them testable with a
// ManualClock and no network.
package command

import (
	"github.com/avinashpathak/memcore/internal/clock"
	"github.com/avinashpathak/memcore/internal/keyspace"
	"github.com/avinashpathak/memcore/internal/resp"
)

// Context is the environment a command runs against: the selected database's
// keyspace and the clock. The server builds one per command invocation.
type Context struct {
	Keyspace *keyspace.Keyspace
	Clock    clock.Clock
}

// Handler executes a command. args is the whole RESP request array, including
// the command name at args[0], so handler argument indices line up with the
// Redis documentation.
type Handler func(ctx *Context, args [][]byte) resp.Reply

// Command is one entry in the command table: a name, an arity rule, and the
// handler that runs it.
type Command struct {
	name  string
	arity int
	run   Handler
}

// Name returns the command's lowercase name.
func (c Command) Name() string { return c.name }

// Arity is the command's argument-count rule, counting the command name. A
// non-negative arity requires exactly that many elements; a negative arity n
// requires at least -n. This mirrors the Redis command table.
func (c Command) Arity() int { return c.arity }

// accepts reports whether an argument count of n satisfies the arity rule.
func (c Command) accepts(n int) bool {
	if c.arity >= 0 {
		return n == c.arity
	}
	return n >= -c.arity
}

// Error replies shared across commands. The wire text matches Redis so existing
// clients and tooling recognize the conditions.
const (
	msgWrongType  = "WRONGTYPE Operation against a key holding the wrong kind of value"
	msgNotInteger = "ERR value is not an integer or out of range"
)

func newCommand(name string, arity int, run Handler) Command {
	return Command{name: name, arity: arity, run: run}
}
