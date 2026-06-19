package command

import (
	"testing"

	"github.com/avinashpathak/memcore/internal/resp"
)

func assertBulkSet(t *testing.T, got resp.Reply, want ...string) {
	t.Helper()
	elems := got.Elements()
	if len(elems) != len(want) {
		t.Fatalf("got %d members, want %d: %v", len(elems), len(want), got)
	}
	for _, w := range want {
		found := false
		for _, e := range elems {
			if e.Equal(resp.BulkString(w)) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("member %q missing from %v", w, got)
		}
	}
}

func TestSAddCountsNewMembers(t *testing.T) {
	r, ctx, _ := newTestEnv()
	if got := run(r, ctx, "SADD", "s", "a", "b", "a"); !got.Equal(resp.Int(2)) {
		t.Fatalf("SADD = %v, want 2", got)
	}
}

func TestSRemDropsMembersAndEmptySet(t *testing.T) {
	r, ctx, _ := newTestEnv()
	run(r, ctx, "SADD", "s", "a", "b")
	if got := run(r, ctx, "SREM", "s", "a", "missing"); !got.Equal(resp.Int(1)) {
		t.Fatalf("SREM = %v, want 1", got)
	}
	run(r, ctx, "SREM", "s", "b")
	if got := run(r, ctx, "EXISTS", "s"); !got.Equal(resp.Int(0)) {
		t.Fatalf("EXISTS after draining set = %v, want 0", got)
	}
}

func TestSMembersReturnsEveryMember(t *testing.T) {
	r, ctx, _ := newTestEnv()
	run(r, ctx, "SADD", "s", "a", "b", "c")
	assertBulkSet(t, run(r, ctx, "SMEMBERS", "s"), "a", "b", "c")
}

func TestSInterIntersectsAcrossKeys(t *testing.T) {
	r, ctx, _ := newTestEnv()
	run(r, ctx, "SADD", "s1", "a", "b", "c", "d")
	run(r, ctx, "SADD", "s2", "b", "c", "e")
	run(r, ctx, "SADD", "s3", "c", "b", "z")
	assertBulkSet(t, run(r, ctx, "SINTER", "s1", "s2", "s3"), "b", "c")
}

func TestSInterWithAMissingKeyIsEmpty(t *testing.T) {
	r, ctx, _ := newTestEnv()
	run(r, ctx, "SADD", "s1", "a")
	if got := run(r, ctx, "SINTER", "s1", "absent"); !got.Equal(resp.Array(nil)) {
		t.Fatalf("SINTER with a missing key = %v, want empty array", got)
	}
}
