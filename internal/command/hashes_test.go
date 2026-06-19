package command

import (
	"testing"

	"github.com/avinashpathak/memcore/internal/resp"
)

func TestHSetCountsNewFields(t *testing.T) {
	r, ctx, _ := newTestEnv()
	if got := run(r, ctx, "HSET", "h", "f1", "v1", "f2", "v2"); !got.Equal(resp.Int(2)) {
		t.Fatalf("HSET = %v, want 2", got)
	}
	// Overwriting f1 and adding f3: one new field.
	if got := run(r, ctx, "HSET", "h", "f1", "x", "f3", "v3"); !got.Equal(resp.Int(1)) {
		t.Fatalf("HSET overwrite = %v, want 1", got)
	}
}

func TestHGetReturnsFieldsAndNilForMisses(t *testing.T) {
	r, ctx, _ := newTestEnv()
	run(r, ctx, "HSET", "h", "f", "v")
	if got := run(r, ctx, "HGET", "h", "f"); !got.Equal(resp.BulkString("v")) {
		t.Fatalf("HGET = %v, want v", got)
	}
	if got := run(r, ctx, "HGET", "h", "absent"); !got.Equal(resp.Nil()) {
		t.Fatalf("HGET absent field = %v, want nil", got)
	}
	if got := run(r, ctx, "HGET", "absent", "f"); !got.Equal(resp.Nil()) {
		t.Fatalf("HGET absent key = %v, want nil", got)
	}
}

func TestHDelRemovesFieldsAndDropsEmptyHash(t *testing.T) {
	r, ctx, _ := newTestEnv()
	run(r, ctx, "HSET", "h", "a", "1", "b", "2")
	if got := run(r, ctx, "HDEL", "h", "a", "missing"); !got.Equal(resp.Int(1)) {
		t.Fatalf("HDEL = %v, want 1", got)
	}
	run(r, ctx, "HDEL", "h", "b")
	if got := run(r, ctx, "EXISTS", "h"); !got.Equal(resp.Int(0)) {
		t.Fatalf("EXISTS after draining hash = %v, want 0", got)
	}
}

func TestHGetAllReturnsEveryFieldValuePair(t *testing.T) {
	r, ctx, _ := newTestEnv()
	run(r, ctx, "HSET", "h", "f1", "v1", "f2", "v2")
	got := run(r, ctx, "HGETALL", "h")
	elems := got.Elements()
	if len(elems) != 4 {
		t.Fatalf("HGETALL returned %d items, want 4", len(elems))
	}
	// Pair order is unspecified, so match each adjacent pair against the expected.
	want := map[string]string{"f1": "v1", "f2": "v2"}
	matched := 0
	for i := 0; i < len(elems); i += 2 {
		for f, v := range want {
			if elems[i].Equal(resp.BulkString(f)) && elems[i+1].Equal(resp.BulkString(v)) {
				matched++
			}
		}
	}
	if matched != 2 {
		t.Fatalf("HGETALL = %v, want fields f1=v1 and f2=v2", got)
	}
}
