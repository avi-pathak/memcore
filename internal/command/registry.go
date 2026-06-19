package command

import (
	"strings"

	"github.com/avinashpathak/memcore/internal/resp"
)

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

// Resolve looks up the command named by args[0] and validates its arity. On
// success it returns the command; otherwise it returns the RESP error reply to
// send back. The server uses Resolve so it can lock the command's shards between
// resolution and execution.
func (r *Registry) Resolve(args [][]byte) (Command, resp.Reply, bool) {
	if len(args) == 0 {
		return Command{}, resp.Error("ERR empty command"), false
	}
	name := strings.ToLower(string(args[0]))
	cmd, ok := r.commands[name]
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
