package persistence

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/avinashpathak/memcore/internal/value"
)

func testLimits() value.Limits {
	t := value.Thresholds{MaxEntries: 128, MaxBytes: 64}
	return value.Limits{List: t, Hash: t, Set: t, ZSet: t}
}

func cmd(parts ...string) [][]byte {
	args := make([][]byte, len(parts))
	for i, p := range parts {
		args[i] = []byte(p)
	}
	return args
}

func TestValueCodecRoundTrip(t *testing.T) {
	limits := testLimits()
	list := value.NewList(limits.List)
	list.PushBack([]byte("a"))
	list.PushBack([]byte("b"))
	hash := value.NewHash(limits.Hash)
	hash.Set("f1", []byte("v1"))
	hash.Set("f2", []byte("v2"))
	set := value.NewSet(limits.Set)
	set.Add([]byte("m1"))
	set.Add([]byte("m2"))
	zset := value.NewZSet(limits.ZSet)
	zset.Add([]byte("x"), 1.5)
	zset.Add([]byte("y"), -2)

	cases := []value.Value{
		value.MakeString([]byte("hello")),
		value.MakeList(list),
		value.MakeHash(hash),
		value.MakeSet(set),
		value.MakeZSet(zset),
	}
	for _, want := range cases {
		var buf bytes.Buffer
		bw := bufio.NewWriter(&buf)
		if err := writeValue(bw, want); err != nil {
			t.Fatalf("writeValue(%v): %v", want.Kind(), err)
		}
		if err := bw.Flush(); err != nil {
			t.Fatal(err)
		}
		got, err := readValue(bufio.NewReader(&buf), limits)
		if err != nil {
			t.Fatalf("readValue(%v): %v", want.Kind(), err)
		}
		assertSameValue(t, got, want)
	}
}

func TestSnapshotAppendLogRecovers(t *testing.T) {
	dir := t.TempDir()
	limits := testLimits()
	st, err := Open(dir, FSyncNo, limits)
	if err != nil {
		t.Fatal(err)
	}
	mustAppend(t, st, 0, cmd("SET", "k", "v"))
	mustAppend(t, st, 1, cmd("RPUSH", "l", "a"))
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	var names []string
	loaded, replayed, err := Recover(dir, limits,
		func(Record) error { return nil },
		func(db int, args [][]byte) error { names = append(names, string(args[0])); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if loaded != 0 {
		t.Fatalf("loaded %d snapshot records, want 0", loaded)
	}
	if replayed != 2 || names[0] != "SET" || names[1] != "RPUSH" {
		t.Fatalf("replayed %v, want [SET RPUSH]", names)
	}
}

func TestCompactionSnapshotsStateAndDropsCoveredSegments(t *testing.T) {
	dir := t.TempDir()
	limits := testLimits()
	st, err := Open(dir, FSyncNo, limits)
	if err != nil {
		t.Fatal(err)
	}
	mustAppend(t, st, 0, cmd("SET", "k", "v"))

	baseGen := st.NextGen()
	var buf bytes.Buffer
	err = WriteSnapshot(&buf, baseGen, func(write func(Record) error) error {
		return write(Record{DB: 0, Key: "k", Value: value.MakeString([]byte("v"))})
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Rotate(); err != nil {
		t.Fatal(err)
	}
	if err := st.InstallSnapshot(buf.Bytes(), baseGen); err != nil {
		t.Fatal(err)
	}
	mustAppend(t, st, 0, cmd("INCR", "n")) // lands in the new segment
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	var loadedKeys, replayedNames []string
	loaded, replayed, err := Recover(dir, limits,
		func(rec Record) error { loadedKeys = append(loadedKeys, rec.Key); return nil },
		func(db int, args [][]byte) error { replayedNames = append(replayedNames, string(args[0])); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if loaded != 1 || loadedKeys[0] != "k" {
		t.Fatalf("loaded %v, want [k] from the snapshot", loadedKeys)
	}
	if replayed != 1 || replayedNames[0] != "INCR" {
		t.Fatalf("replayed %v, want [INCR] from the new segment", replayedNames)
	}
	if _, err := os.Stat(filepath.Join(dir, appendName+".0")); !os.IsNotExist(err) {
		t.Fatal("segment 0 should have been removed by compaction")
	}
}

func TestRecoverToleratesATruncatedTrailingRecord(t *testing.T) {
	dir := t.TempDir()
	limits := testLimits()
	st, err := Open(dir, FSyncNo, limits)
	if err != nil {
		t.Fatal(err)
	}
	mustAppend(t, st, 0, cmd("SET", "k", "v"))
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	// Append a record header that promises five arguments but supplies none.
	f, err := os.OpenFile(filepath.Join(dir, appendName+".0"), os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte{0x00, 0x05}); err != nil {
		t.Fatal(err)
	}
	f.Close()

	var names []string
	_, replayed, err := Recover(dir, limits,
		func(Record) error { return nil },
		func(db int, args [][]byte) error { names = append(names, string(args[0])); return nil })
	if err != nil {
		t.Fatalf("recover errored on a truncated trailing record: %v", err)
	}
	if replayed != 1 || names[0] != "SET" {
		t.Fatalf("replayed %v, want only the intact [SET]", names)
	}
}

func mustAppend(t *testing.T, st *Store, db int, args [][]byte) {
	t.Helper()
	if err := st.Append(db, args); err != nil {
		t.Fatalf("Append: %v", err)
	}
}

func assertSameValue(t *testing.T, got, want value.Value) {
	t.Helper()
	if got.Kind() != want.Kind() {
		t.Fatalf("kind = %v, want %v", got.Kind(), want.Kind())
	}
	switch want.Kind() {
	case value.KindString:
		if !bytes.Equal(got.Str(), want.Str()) {
			t.Fatalf("string = %q, want %q", got.Str(), want.Str())
		}
	case value.KindList:
		assertByteSeq(t, got.List().Range(0, -1), want.List().Range(0, -1))
	case value.KindHash:
		assertSameStringMap(t, hashMap(got), hashMap(want))
	case value.KindSet:
		assertSameStringSet(t, setMembers(got), setMembers(want))
	case value.KindZSet:
		assertSameScoreMap(t, zsetMap(got), zsetMap(want))
	}
}

func assertByteSeq(t *testing.T, got, want [][]byte) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("length %d, want %d", len(got), len(want))
	}
	for i := range want {
		if !bytes.Equal(got[i], want[i]) {
			t.Fatalf("element %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func hashMap(v value.Value) map[string]string {
	m := make(map[string]string)
	pairs := v.Hash().Pairs()
	for i := 0; i < len(pairs); i += 2 {
		m[string(pairs[i])] = string(pairs[i+1])
	}
	return m
}

func setMembers(v value.Value) []string {
	var out []string
	for _, m := range v.Set().Members() {
		out = append(out, string(m))
	}
	sort.Strings(out)
	return out
}

func zsetMap(v value.Value) map[string]float64 {
	m := make(map[string]float64)
	for _, zm := range v.ZSet().Range(0, -1) {
		m[string(zm.Member)] = zm.Score
	}
	return m
}

func assertSameStringMap(t *testing.T, got, want map[string]string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("map size %d, want %d", len(got), len(want))
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("key %q = %q, want %q", k, got[k], v)
		}
	}
}

func assertSameStringSet(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("set size %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("member %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func assertSameScoreMap(t *testing.T, got, want map[string]float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("zset size %d, want %d", len(got), len(want))
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("member %q score = %v, want %v", k, got[k], v)
		}
	}
}
