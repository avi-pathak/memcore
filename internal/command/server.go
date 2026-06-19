package command

import "github.com/avinashpathak/memcore/internal/resp"

func serverCommands() []Command {
	return []Command{
		newCommand("ping", -1, cmdPing),
		newCommand("flushdb", -1, cmdFlushDB),
	}
}

func cmdPing(_ *Context, args [][]byte) resp.Reply {
	switch len(args) {
	case 1:
		return resp.Simple("PONG")
	case 2:
		return resp.Bulk(args[1])
	default:
		return resp.Error("ERR wrong number of arguments for 'ping' command")
	}
}

func cmdFlushDB(ctx *Context, _ [][]byte) resp.Reply {
	// FLUSHDB [ASYNC|SYNC]: the modifier is accepted and ignored; the flush is
	// synchronous for now.
	ctx.Keyspace.Flush()
	return resp.OK()
}
