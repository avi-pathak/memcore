package command

import (
	"strconv"

	"github.com/avinashpathak/memcore/internal/resp"
	"github.com/avinashpathak/memcore/internal/value"
)

func listCommands() []Command {
	return []Command{
		writeKey("lpush", -3, cmdLPush),
		writeKey("rpush", -3, cmdRPush),
		writeKey("lpop", -2, cmdLPop),
		writeKey("rpop", -2, cmdRPop),
		readKey("lrange", 4, cmdLRange),
		readKey("llen", 2, cmdLLen),
	}
}

func cmdLPush(ctx *Context, args [][]byte) resp.Reply { return listPush(ctx, args, true) }
func cmdRPush(ctx *Context, args [][]byte) resp.Reply { return listPush(ctx, args, false) }

func listPush(ctx *Context, args [][]byte, front bool) resp.Reply {
	key := string(args[1])
	e, ok := ctx.Keyspace.Lookup(key)
	var l *value.List
	if ok {
		if e.Value.Kind() != value.KindList {
			return resp.Error(msgWrongType)
		}
		l = e.Value.List()
	} else {
		l = value.NewList(ctx.Limits.List)
	}
	for _, v := range args[2:] {
		if front {
			l.PushFront(v)
		} else {
			l.PushBack(v)
		}
	}
	if !ok {
		ctx.Keyspace.Set(key, value.MakeList(l))
	}
	return resp.Int(int64(l.Len()))
}

func cmdLPop(ctx *Context, args [][]byte) resp.Reply { return listPop(ctx, args, true) }
func cmdRPop(ctx *Context, args [][]byte) resp.Reply { return listPop(ctx, args, false) }

func listPop(ctx *Context, args [][]byte, front bool) resp.Reply {
	if len(args) > 3 {
		return resp.Errorf("ERR wrong number of arguments for '%s' command", args[0])
	}
	count, counted := 1, len(args) == 3
	if counted {
		n, err := strconv.Atoi(string(args[2]))
		if err != nil {
			return resp.Error(msgNotInteger)
		}
		if n < 0 {
			return resp.Error("ERR value is out of range, must be positive")
		}
		count = n
	}

	key := string(args[1])
	e, ok := ctx.Keyspace.Lookup(key)
	if !ok {
		return resp.Nil()
	}
	if e.Value.Kind() != value.KindList {
		return resp.Error(msgWrongType)
	}
	l := e.Value.List()

	pop := l.PopFront
	if !front {
		pop = l.PopBack
	}
	if !counted {
		v, _ := pop()
		if l.Len() == 0 {
			ctx.Keyspace.Delete(key)
		}
		return resp.Bulk(v)
	}
	out := make([]resp.Reply, 0, count)
	for i := 0; i < count; i++ {
		v, ok := pop()
		if !ok {
			break
		}
		out = append(out, resp.Bulk(v))
	}
	if l.Len() == 0 {
		ctx.Keyspace.Delete(key)
	}
	return resp.Array(out)
}

func cmdLRange(ctx *Context, args [][]byte) resp.Reply {
	start, err1 := strconv.Atoi(string(args[2]))
	stop, err2 := strconv.Atoi(string(args[3]))
	if err1 != nil || err2 != nil {
		return resp.Error(msgNotInteger)
	}
	e, ok := ctx.Keyspace.Lookup(string(args[1]))
	if !ok {
		return resp.Array(nil)
	}
	if e.Value.Kind() != value.KindList {
		return resp.Error(msgWrongType)
	}
	elems := e.Value.List().Range(start, stop)
	out := make([]resp.Reply, len(elems))
	for i, el := range elems {
		out[i] = resp.Bulk(el)
	}
	return resp.Array(out)
}

func cmdLLen(ctx *Context, args [][]byte) resp.Reply {
	e, ok := ctx.Keyspace.Lookup(string(args[1]))
	if !ok {
		return resp.Int(0)
	}
	if e.Value.Kind() != value.KindList {
		return resp.Error(msgWrongType)
	}
	return resp.Int(int64(e.Value.List().Len()))
}
