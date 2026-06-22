package command

import (
	"github.com/avinashpathak/memcore/internal/resp"
)

// maxCommandNameLen bounds the stack buffer used for case-insensitive command
// lookup. The longest built-in name is well under this, so a longer token is
// simply not a known command.
const maxCommandNameLen = 32

// Registry is the command table: the single source of truth for which commands
// exist. It is built once at boot and is read-only afterward, so it needs no
// synchronization.
type Registry struct {
	commands map[string]Command
}

// NewRegistry builds the registry with every built-in command.
func NewRegistry() *Registry {
	r := &Registry{commands: make(map[string]Command)}
	r.register(stringCommands())
	r.register(keyCommands())
	r.register(listCommands())
	r.register(hashCommands())
	r.register(setCommands())
	r.register(zsetCommands())
	r.register(serverCommands())
	return r
}

// register adds cmds to the table. A duplicate name is a programming error
// caught at boot, not a runtime condition, so it panics.
func (r *Registry) register(cmds []Command) {
	for _, c := range cmds {
		if _, dup := r.commands[c.name]; dup {
			panic("command: duplicate registration of " + c.name)
		}
		r.commands[c.name] = c
	}
}

// Lookup returns the command registered under name, which must be lowercase.
func (r *Registry) Lookup(name string) (Command, bool) {
	c, ok := r.commands[name]
	return c, ok
}

// lookupFold finds a command by name case-insensitively without allocating.
// Command names are short ASCII, so the lowercased form fits in a stack buffer,
// and indexing the map with string(buf[:n]) is special-cased by the compiler to
// avoid a heap allocation. This runs on every command, so the allocation it
// saves matters.
func (r *Registry) lookupFold(name []byte) (Command, bool) {
	if len(name) == 0 || len(name) > maxCommandNameLen {
		return Command{}, false
	}
	var buf [maxCommandNameLen]byte
	for i, b := range name {
		if b >= 'A' && b <= 'Z' {
			b += 'a' - 'A'
		}
		buf[i] = b
	}
	c, ok := r.commands[string(buf[:len(name)])]
	return c, ok
}

// Resolve looks up the command named by args[0] and validates its arity. On
// success it returns the command; otherwise it returns the RESP error reply to
// send back. The server uses Resolve so it can lock the command's shards between
// resolution and execution.
func (r *Registry) Resolve(args [][]byte) (Command, resp.Reply, bool) {
	if len(args) == 0 {
		return Command{}, resp.Error("ERR empty command"), false
	}
	cmd, ok := r.lookupFold(args[0])
	if !ok {
		return Command{}, resp.Errorf("ERR unknown command '%s'", args[0]), false
	}
	if !cmd.accepts(len(args)) {
		return Command{}, resp.Errorf("ERR wrong number of arguments for '%s' command", cmd.name), false
	}
	return cmd, resp.Reply{}, true
}

// Dispatch resolves and runs a command without external locking. The server
// wraps Run in shard locking; tests use Dispatch directly.
func (r *Registry) Dispatch(ctx *Context, args [][]byte) resp.Reply {
	cmd, errReply, ok := r.Resolve(args)
	if !ok {
		return errReply
	}
	return cmd.run(ctx, args)
}
