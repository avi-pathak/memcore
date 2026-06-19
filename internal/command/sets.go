package command

import (
	"github.com/avinashpathak/memcore/internal/resp"
	"github.com/avinashpathak/memcore/internal/value"
)

func setCommands() []Command {
	return []Command{
		writeKey("sadd", -3, cmdSAdd),
		writeKey("srem", -3, cmdSRem),
		readKey("smembers", 2, cmdSMembers),
		readKeys("sinter", -2, cmdSInter),
	}
}

func cmdSAdd(ctx *Context, args [][]byte) resp.Reply {
	key := string(args[1])
	e, ok := ctx.Keyspace.Lookup(key)
	var s *value.Set
	if ok {
		if e.Value.Kind() != value.KindSet {
			return resp.Error(msgWrongType)
		}
		s = e.Value.Set()
	} else {
		s = value.NewSet(ctx.Limits.Set)
	}
	var added int64
	for _, m := range args[2:] {
		if s.Add(m) {
			added++
		}
	}
	if !ok {
		ctx.Keyspace.Set(key, value.MakeSet(s))
	}
	return resp.Int(added)
}

func cmdSRem(ctx *Context, args [][]byte) resp.Reply {
	key := string(args[1])
	e, ok := ctx.Keyspace.Lookup(key)
	if !ok {
		return resp.Int(0)
	}
	if e.Value.Kind() != value.KindSet {
		return resp.Error(msgWrongType)
	}
	s := e.Value.Set()
	var removed int64
	for _, m := range args[2:] {
		if s.Remove(m) {
			removed++
		}
	}
	if s.Len() == 0 {
		ctx.Keyspace.Delete(key)
	}
	return resp.Int(removed)
}

func cmdSMembers(ctx *Context, args [][]byte) resp.Reply {
	e, ok := ctx.Keyspace.Lookup(string(args[1]))
	if !ok {
		return resp.Array(nil)
	}
	if e.Value.Kind() != value.KindSet {
		return resp.Error(msgWrongType)
	}
	members := e.Value.Set().Members()
	out := make([]resp.Reply, len(members))
	for i, m := range members {
		out[i] = resp.Bulk(m)
	}
	return resp.Array(out)
}

// cmdSInter intersects several sets. It is the cross-shard slow path: the
// executor read-locks every key's shard before this runs, in a fixed global
// order. The work starts from the smallest set to keep the comparisons few.
func cmdSInter(ctx *Context, args [][]byte) resp.Reply {
	sets := make([]*value.Set, 0, len(args)-1)
	for _, raw := range args[1:] {
		e, ok := ctx.Keyspace.Lookup(string(raw))
		if !ok {
			return resp.Array(nil) // intersection with an absent set is empty
		}
		if e.Value.Kind() != value.KindSet {
			return resp.Error(msgWrongType)
		}
		sets = append(sets, e.Value.Set())
	}

	smallest := 0
	for i, s := range sets {
		if s.Len() < sets[smallest].Len() {
			smallest = i
		}
	}

	var out []resp.Reply
	for _, m := range sets[smallest].Members() {
		inAll := true
		for i, s := range sets {
			if i == smallest {
				continue
			}
			if !s.Contains(m) {
				inAll = false
				break
			}
		}
		if inAll {
			out = append(out, resp.Bulk(m))
		}
	}
	return resp.Array(out)
}
