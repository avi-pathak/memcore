package command

import (
	"math"
	"strconv"
	"strings"

	"github.com/avinashpathak/memcore/internal/resp"
	"github.com/avinashpathak/memcore/internal/value"
)

func zsetCommands() []Command {
	return []Command{
		writeKey("zadd", -4, cmdZAdd),
		readKey("zrange", -4, cmdZRange),
		readKey("zscore", 3, cmdZScore),
	}
}

func cmdZAdd(ctx *Context, args [][]byte) resp.Reply {
	if len(args)%2 != 0 { // ZADD key (score member)+ has an even argument count
		return resp.Error("ERR wrong number of arguments for 'zadd' command")
	}
	// Parse and validate every score before touching the keyspace, so a bad
	// score cannot apply a partial update.
	type pair struct {
		score  float64
		member []byte
	}
	pairs := make([]pair, 0, (len(args)-2)/2)
	for i := 2; i+1 < len(args); i += 2 {
		score, err := strconv.ParseFloat(string(args[i]), 64)
		if err != nil || math.IsNaN(score) {
			return resp.Error("ERR value is not a valid float")
		}
		pairs = append(pairs, pair{score, args[i+1]})
	}

	key := string(args[1])
	e, ok := ctx.Keyspace.Lookup(key)
	var z *value.ZSet
	if ok {
		if e.Value.Kind() != value.KindZSet {
			return resp.Error(msgWrongType)
		}
		z = e.Value.ZSet()
	} else {
		z = value.NewZSet()
	}
	var added int64
	for _, p := range pairs {
		if z.Add(p.member, p.score) {
			added++
		}
	}
	if !ok {
		ctx.Keyspace.Set(key, value.MakeZSet(z))
	}
	return resp.Int(added)
}

func cmdZRange(ctx *Context, args [][]byte) resp.Reply {
	withScores := false
	switch {
	case len(args) == 4:
	case len(args) == 5 && strings.EqualFold(string(args[4]), "WITHSCORES"):
		withScores = true
	default:
		return resp.Error("ERR syntax error")
	}
	start, err1 := strconv.Atoi(string(args[2]))
	stop, err2 := strconv.Atoi(string(args[3]))
	if err1 != nil || err2 != nil {
		return resp.Error(msgNotInteger)
	}

	e, ok := ctx.Keyspace.Lookup(string(args[1]))
	if !ok {
		return resp.Array(nil)
	}
	if e.Value.Kind() != value.KindZSet {
		return resp.Error(msgWrongType)
	}
	members := e.Value.ZSet().Range(start, stop)

	if !withScores {
		out := make([]resp.Reply, len(members))
		for i, m := range members {
			out[i] = resp.Bulk(m.Member)
		}
		return resp.Array(out)
	}
	out := make([]resp.Reply, 0, len(members)*2)
	for _, m := range members {
		out = append(out, resp.Bulk(m.Member), resp.BulkString(formatScore(m.Score)))
	}
	return resp.Array(out)
}

func cmdZScore(ctx *Context, args [][]byte) resp.Reply {
	e, ok := ctx.Keyspace.Lookup(string(args[1]))
	if !ok {
		return resp.Nil()
	}
	if e.Value.Kind() != value.KindZSet {
		return resp.Error(msgWrongType)
	}
	score, ok := e.Value.ZSet().Score(args[2])
	if !ok {
		return resp.Nil()
	}
	return resp.BulkString(formatScore(score))
}

// formatScore renders a score the way Redis does: the shortest decimal that
// round-trips, with infinities spelled out.
func formatScore(f float64) string {
	switch {
	case math.IsInf(f, 1):
		return "inf"
	case math.IsInf(f, -1):
		return "-inf"
	default:
		return strconv.FormatFloat(f, 'g', -1, 64)
	}
}
