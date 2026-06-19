package command

import (
	"strconv"

	"github.com/avinashpathak/memcore/internal/resp"
)

func serverCommands() []Command {
	return []Command{
		newCommand("ping", -1, cmdPing),
		newCommand("select", 2, cmdSelect),
		newCommand("flushdb", -1, cmdFlushDB),
	}
}

func cmdSelect(ctx *Context, args [][]byte) resp.Reply {
	idx, err := strconv.Atoi(string(args[1]))
	if err != nil {
		return resp.Error("ERR value is not an integer or out of range")
	}
	if err := ctx.Select(idx); err != nil {
		return resp.Error("ERR DB index is out of range")
	}
	return resp.OK()
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
