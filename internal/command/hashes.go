package command

import (
	"github.com/avinashpathak/memcore/internal/resp"
	"github.com/avinashpathak/memcore/internal/value"
)

func hashCommands() []Command {
	return []Command{
		writeKey("hset", -4, cmdHSet),
		readKey("hget", 3, cmdHGet),
		writeKey("hdel", -3, cmdHDel),
		readKey("hgetall", 2, cmdHGetAll),
	}
}

func cmdHSet(ctx *Context, args [][]byte) resp.Reply {
	if len(args)%2 != 0 { // HSET key (field value)+ has an even argument count
		return resp.Error("ERR wrong number of arguments for 'hset' command")
	}
	key := string(args[1])
	e, ok := ctx.Keyspace.Lookup(key)
	var h *value.Hash
	if ok {
		if e.Value.Kind() != value.KindHash {
			return resp.Error(msgWrongType)
		}
		h = e.Value.Hash()
	} else {
		h = value.NewHash(ctx.Limits.Hash)
	}
	var added int64
	for i := 2; i+1 < len(args); i += 2 {
		if h.Set(string(args[i]), args[i+1]) {
			added++
		}
	}
	if !ok {
		ctx.Keyspace.Set(key, value.MakeHash(h))
	}
	return resp.Int(added)
}

func cmdHGet(ctx *Context, args [][]byte) resp.Reply {
	e, ok := ctx.Keyspace.Lookup(string(args[1]))
	if !ok {
		return resp.Nil()
	}
	if e.Value.Kind() != value.KindHash {
		return resp.Error(msgWrongType)
	}
	v, ok := e.Value.Hash().Get(string(args[2]))
	if !ok {
		return resp.Nil()
	}
	return resp.Bulk(v)
}

func cmdHDel(ctx *Context, args [][]byte) resp.Reply {
	key := string(args[1])
	e, ok := ctx.Keyspace.Lookup(key)
	if !ok {
		return resp.Int(0)
	}
	if e.Value.Kind() != value.KindHash {
		return resp.Error(msgWrongType)
	}
	h := e.Value.Hash()
	var removed int64
	for _, f := range args[2:] {
		if h.Delete(string(f)) {
			removed++
		}
	}
	if h.Len() == 0 {
		ctx.Keyspace.Delete(key)
	}
	return resp.Int(removed)
}

func cmdHGetAll(ctx *Context, args [][]byte) resp.Reply {
	e, ok := ctx.Keyspace.Lookup(string(args[1]))
	if !ok {
		return resp.Array(nil)
	}
	if e.Value.Kind() != value.KindHash {
		return resp.Error(msgWrongType)
	}
	pairs := e.Value.Hash().Pairs()
	out := make([]resp.Reply, len(pairs))
	for i, p := range pairs {
		out[i] = resp.Bulk(p)
	}
	return resp.Array(out)
}
