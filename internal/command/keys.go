package command

import (
	"strconv"
	"time"

	"github.com/avinashpathak/memcore/internal/resp"
)

func keyCommands() []Command {
	return []Command{
		writeKeys("del", -2, cmdDel),
		// UNLINK matches DEL's reply but frees collection values on a background
		// reaper, so removing a large key does not stall command execution.
		writeKeys("unlink", -2, cmdUnlink),
		readKeys("exists", -2, cmdExists),
		writeKey("expire", 3, cmdExpire),
		writeKey("pexpireat", 3, cmdPExpireAt),
		readKey("ttl", 2, cmdTTL),
		writeKey("persist", 2, cmdPersist),
		readKey("type", 2, cmdType),
	}
}

func cmdDel(ctx *Context, args [][]byte) resp.Reply {
	var n int64
	for _, raw := range args[1:] {
		if ctx.Keyspace.Delete(string(raw)) {
			n++
		}
	}
	return resp.Int(n)
}

func cmdUnlink(ctx *Context, args [][]byte) resp.Reply {
	var n int64
	for _, raw := range args[1:] {
		if ctx.Keyspace.Unlink(string(raw)) {
			n++
		}
	}
	return resp.Int(n)
}

func cmdExists(ctx *Context, args [][]byte) resp.Reply {
	var n int64
	for _, raw := range args[1:] {
		if ctx.Keyspace.Exists(string(raw)) {
			n++
		}
	}
	return resp.Int(n)
}

func cmdExpire(ctx *Context, args [][]byte) resp.Reply {
	secs, err := strconv.ParseInt(string(args[2]), 10, 64)
	if err != nil {
		return resp.Error(msgNotInteger)
	}
	at := ctx.Clock.Now().Add(time.Duration(secs) * time.Second)
	if ctx.Keyspace.SetExpire(string(args[1]), at) {
		return resp.Int(1)
	}
	return resp.Int(0)
}

// cmdPExpireAt sets an absolute expiry from a Unix-millisecond deadline. The
// append log rewrites the relative EXPIRE to this form so replay is independent
// of when it runs.
func cmdPExpireAt(ctx *Context, args [][]byte) resp.Reply {
	ms, err := strconv.ParseInt(string(args[2]), 10, 64)
	if err != nil {
		return resp.Error(msgNotInteger)
	}
	if ctx.Keyspace.SetExpire(string(args[1]), time.UnixMilli(ms)) {
		return resp.Int(1)
	}
	return resp.Int(0)
}

func cmdTTL(ctx *Context, args [][]byte) resp.Reply {
	e, ok := ctx.Keyspace.Lookup(string(args[1]))
	if !ok {
		return resp.Int(-2)
	}
	if !e.HasExpiry() {
		return resp.Int(-1)
	}
	ms := e.ExpireAt.Sub(ctx.Clock.Now()).Milliseconds()
	if ms < 0 {
		ms = 0
	}
	return resp.Int((ms + 500) / 1000) // round to the nearest second, as Redis does
}

func cmdPersist(ctx *Context, args [][]byte) resp.Reply {
	if ctx.Keyspace.Persist(string(args[1])) {
		return resp.Int(1)
	}
	return resp.Int(0)
}

func cmdType(ctx *Context, args [][]byte) resp.Reply {
	e, ok := ctx.Keyspace.Lookup(string(args[1]))
	if !ok {
		return resp.Simple("none")
	}
	return resp.Simple(e.Value.Kind().String())
}
