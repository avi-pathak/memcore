package command

import (
	"testing"

	"github.com/avinashpathak/memcore/internal/resp"
)

func TestTypeReportsEachKind(t *testing.T) {
	r, ctx, _ := newTestEnv()
	run(r, ctx, "SET", "s", "v")
	run(r, ctx, "RPUSH", "l", "a")
	run(r, ctx, "HSET", "h", "f", "v")
	run(r, ctx, "SADD", "se", "m")
	run(r, ctx, "ZADD", "z", "1", "m")
	want := map[string]string{"s": "string", "l": "list", "h": "hash", "se": "set", "z": "zset"}
	for key, kind := range want {
		if got := run(r, ctx, "TYPE", key); !got.Equal(resp.Simple(kind)) {
			t.Fatalf("TYPE %s = %v, want %s", key, got, kind)
		}
	}
}

func TestOperatingOnTheWrongTypeRepliesWrongType(t *testing.T) {
	r, ctx, _ := newTestEnv()
	run(r, ctx, "SET", "str", "v")
	run(r, ctx, "RPUSH", "lst", "a")

	cases := [][]string{
		{"LPUSH", "str", "x"},
		{"LRANGE", "str", "0", "-1"},
		{"GET", "lst"},
		{"HSET", "str", "f", "v"},
		{"HGET", "lst", "f"},
		{"SADD", "str", "m"},
		{"SMEMBERS", "lst"},
		{"ZADD", "str", "1", "m"},
		{"ZSCORE", "lst", "m"},
	}
	for _, c := range cases {
		if got := run(r, ctx, c...); !got.IsError() {
			t.Fatalf("%v = %v, want a WRONGTYPE error", c, got)
		}
	}
}
