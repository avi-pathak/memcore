package command

import (
	"math"
	"strconv"

	"github.com/avinashpathak/memcore/internal/resp"
	"github.com/avinashpathak/memcore/internal/value"
)

func stringCommands() []Command {
	return []Command{
		readKey("get", 2, cmdGet),
		writeKey("set", 3, cmdSet),
		writeKey("incr", 2, cmdIncr),
		writeKey("decr", 2, cmdDecr),
	}
}

func cmdGet(ctx *Context, args [][]byte) resp.Reply {
	v, ok := ctx.Keyspace.Get(string(args[1]))
	if !ok {
		return resp.Nil()
	}
	if v.Kind() != value.KindString {
		return resp.Error(msgWrongType)
	}
	return resp.Bulk(v.Str())
}

func cmdSet(ctx *Context, args [][]byte) resp.Reply {
	ctx.Keyspace.Set(string(args[1]), value.MakeString(args[2]))
	return resp.OK()
}

func cmdIncr(ctx *Context, args [][]byte) resp.Reply { return incrBy(ctx, string(args[1]), 1) }
func cmdDecr(ctx *Context, args [][]byte) resp.Reply { return incrBy(ctx, string(args[1]), -1) }

// incrBy applies delta to the integer stored at key, creating it at zero when
// absent. An existing TTL is preserved, matching Redis.
func incrBy(ctx *Context, key string, delta int64) resp.Reply {
	e, ok := ctx.Keyspace.Lookup(key)
	var cur int64
	if ok {
		if e.Value.Kind() != value.KindString {
			return resp.Error(msgWrongType)
		}
		n, err := strconv.ParseInt(string(e.Value.Str()), 10, 64)
		if err != nil {
			return resp.Error(msgNotInteger)
		}
		cur = n
	}
	if (delta > 0 && cur > math.MaxInt64-delta) || (delta < 0 && cur < math.MinInt64-delta) {
		return resp.Error("ERR increment or decrement would overflow")
	}
	cur += delta

	next := value.MakeString(strconv.AppendInt(nil, cur, 10))
	if ok {
		e.Value = next
		ctx.Keyspace.SetEntry(key, e)
	} else {
		ctx.Keyspace.Set(key, next)
	}
	return resp.Int(cur)
}
