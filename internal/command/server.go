package command

import (
	"strconv"
	"strings"

	"github.com/avinashpathak/memcore/internal/resp"
)

func serverCommands() []Command {
	return []Command{
		plain("ping", -1, cmdPing),
		plain("select", 2, cmdSelect),
		plain("config", -2, cmdConfig),
		wholeDB("flushdb", -1, cmdFlushDB),
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

// cmdConfig implements CONFIG GET and CONFIG SET over the reloadable subset of
// configuration. Parameters outside that subset are not visible here; changing
// them requires a restart.
func cmdConfig(ctx *Context, args [][]byte) resp.Reply {
	if ctx.Settings == nil {
		return resp.Error("ERR CONFIG is unavailable")
	}
	switch strings.ToUpper(string(args[1])) {
	case "GET":
		if len(args) != 3 {
			return resp.Error("ERR wrong number of arguments for 'config|get' command")
		}
		return configGet(ctx, string(args[2]))
	case "SET":
		if len(args) != 4 {
			return resp.Error("ERR wrong number of arguments for 'config|set' command")
		}
		if err := ctx.Settings.Set(strings.ToLower(string(args[2])), string(args[3])); err != nil {
			return resp.Errorf("ERR %s", err)
		}
		return resp.OK()
	default:
		return resp.Errorf("ERR unknown CONFIG subcommand '%s'", args[1])
	}
}

func configGet(ctx *Context, pattern string) resp.Reply {
	// A bare "*" lists every reloadable parameter; otherwise pattern is an exact
	// parameter name. Glob matching beyond "*" is intentionally not supported.
	if pattern == "*" {
		all := ctx.Settings.All()
		out := make([]resp.Reply, 0, len(all)*2)
		for _, kv := range all {
			out = append(out, resp.BulkString(kv[0]), resp.BulkString(kv[1]))
		}
		return resp.Array(out)
	}
	if v, ok := ctx.Settings.Get(strings.ToLower(pattern)); ok {
		return resp.Array([]resp.Reply{resp.BulkString(strings.ToLower(pattern)), resp.BulkString(v)})
	}
	return resp.Array(nil)
}
