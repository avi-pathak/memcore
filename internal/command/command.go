// Package command implements Memcore's command set as a registry of handlers.
// Each command is pure and synchronous: it reads and writes the keyspace and
// clock carried by its Context and returns a resp.Reply. Commands never touch a
// socket and never reach for globals, which is what makes them testable with a
// ManualClock and no network.
package command

import (
	"errors"

	"github.com/avinashpathak/memcore/internal/clock"
	"github.com/avinashpathak/memcore/internal/config"
	"github.com/avinashpathak/memcore/internal/resp"
	"github.com/avinashpathak/memcore/internal/shard"
	"github.com/avinashpathak/memcore/internal/value"
)

var errDBOutOfRange = errors.New("database index out of range")

// Context is a connection's session: the clock, the set of logical databases,
// and which one is selected. One Context is created per connection and threaded
// through every command on it, so SELECT can change the active database for the
// commands that follow.
//
// A Context is owned by a single connection goroutine. The server holds the
// selected database's lock for the duration of each command, so handlers read
// and write Keyspace without locking. Keyspace always refers to databases[index]
// and is kept in step by Select.
type Context struct {
	Keyspace *shard.DB // the selected database
	Clock    clock.Clock
	Limits   value.Limits     // compact-encoding thresholds for new collections
	Settings *config.Settings // live, reloadable configuration

	databases []*shard.DB
	index     int
}

// NewContext returns a session over databases positioned at database 0.
func NewContext(clk clock.Clock, databases []*shard.DB, limits value.Limits, settings *config.Settings) *Context {
	c := &Context{Clock: clk, Limits: limits, Settings: settings, databases: databases}
	if len(databases) > 0 {
		c.Keyspace = databases[0]
	}
	return c
}

// DB returns the index of the selected database.
func (c *Context) DB() int { return c.index }

// Select switches the active database. It reports an error and leaves the
// selection unchanged when index is out of range.
func (c *Context) Select(index int) error {
	if index < 0 || index >= len(c.databases) {
		return errDBOutOfRange
	}
	c.index = index
	c.Keyspace = c.databases[index]
	return nil
}

// Handler executes a command. args is the whole RESP request array, including
// the command name at args[0], so handler argument indices line up with the
// Redis documentation.
type Handler func(ctx *Context, args [][]byte) resp.Reply

// Command is one entry in the command table: its name, arity rule, handler, and
// the metadata the executor uses to lock the right shards before running it.
type Command struct {
	name     string
	arity    int
	run      Handler
	readOnly bool
	wholeDB  bool
	firstKey int // index of the first key argument; 0 means the command takes no keys
	lastKey  int // index of the last key argument; negative counts back from the end
	keyStep  int // stride between key arguments
}

// Name returns the command's lowercase name.
func (c Command) Name() string { return c.name }

// Arity is the command's argument-count rule, counting the command name. A
// non-negative arity requires exactly that many elements; a negative arity n
// requires at least -n. This mirrors the Redis command table.
func (c Command) Arity() int { return c.arity }

// ReadOnly reports whether the command only reads, so the executor can take a
// shared lock on its shards.
func (c Command) ReadOnly() bool { return c.readOnly }

// WholeDB reports whether the command operates on the entire database, so the
// executor locks every shard.
func (c Command) WholeDB() bool { return c.wholeDB }

// Run executes the command against ctx.
func (c Command) Run(ctx *Context, args [][]byte) resp.Reply { return c.run(ctx, args) }

// SingleKey reports that the command touches exactly one key, at args[1]. This
// is the common case, and it lets the executor lock one shard without building
// the slices that the multi-key path needs.
func (c Command) SingleKey() bool {
	return c.firstKey == 1 && c.lastKey == 1 && c.keyStep == 1 && !c.wholeDB
}

// Keys returns the key arguments the command touches, which the executor hashes
// to decide which shards to lock. It returns nil for commands that take no keys
// or that span the whole database.
func (c Command) Keys(args [][]byte) [][]byte {
	if c.firstKey == 0 || c.wholeDB {
		return nil
	}
	last := c.lastKey
	if last < 0 {
		last = len(args) + last
	}
	if last >= len(args) {
		last = len(args) - 1
	}
	if c.firstKey > last {
		return nil
	}
	keys := make([][]byte, 0, (last-c.firstKey)/c.keyStep+1)
	for i := c.firstKey; i <= last; i += c.keyStep {
		keys = append(keys, args[i])
	}
	return keys
}

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

// Command constructors. The name encodes the lock scope so a command table reads
// as a specification: plain commands touch no keys; readKey and writeKey touch a
// single key at argument 1; readKeys and writeKeys touch every argument from 1
// onward; wholeDB commands lock the entire database.

func plain(name string, arity int, run Handler) Command {
	return Command{name: name, arity: arity, run: run}
}

func readKey(name string, arity int, run Handler) Command {
	return Command{name: name, arity: arity, run: run, readOnly: true, firstKey: 1, lastKey: 1, keyStep: 1}
}

func writeKey(name string, arity int, run Handler) Command {
	return Command{name: name, arity: arity, run: run, firstKey: 1, lastKey: 1, keyStep: 1}
}

func readKeys(name string, arity int, run Handler) Command {
	return Command{name: name, arity: arity, run: run, readOnly: true, firstKey: 1, lastKey: -1, keyStep: 1}
}

func writeKeys(name string, arity int, run Handler) Command {
	return Command{name: name, arity: arity, run: run, firstKey: 1, lastKey: -1, keyStep: 1}
}

func wholeDB(name string, arity int, run Handler) Command {
	return Command{name: name, arity: arity, run: run, wholeDB: true}
}
