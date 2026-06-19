package value

import "testing"

func assertElems(t *testing.T, got [][]byte, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d elements %q, want %d %v", len(got), got, len(want), want)
	}
	for i := range want {
		if string(got[i]) != want[i] {
			t.Fatalf("element %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestListPushesAndPopsAtBothEnds(t *testing.T) {
	l := NewList()
	l.PushBack([]byte("b"))
	l.PushFront([]byte("a"))
	l.PushBack([]byte("c")) // a, b, c
	if l.Len() != 3 {
		t.Fatalf("Len = %d, want 3", l.Len())
	}
	if v, _ := l.PopFront(); string(v) != "a" {
		t.Fatalf("PopFront = %q, want a", v)
	}
	if v, _ := l.PopBack(); string(v) != "c" {
		t.Fatalf("PopBack = %q, want c", v)
	}
	if l.Len() != 1 {
		t.Fatalf("Len = %d, want 1", l.Len())
	}
}

func TestListPopFromEmptyReportsAbsent(t *testing.T) {
	l := NewList()
	if _, ok := l.PopFront(); ok {
		t.Fatal("PopFront on an empty list reported a value")
	}
	if _, ok := l.PopBack(); ok {
		t.Fatal("PopBack on an empty list reported a value")
	}
}

func TestListRangeUsesRedisIndexSemantics(t *testing.T) {
	l := NewList()
	for _, s := range []string{"a", "b", "c", "d", "e"} {
		l.PushBack([]byte(s))
	}
	assertElems(t, l.Range(1, 3), []string{"b", "c", "d"})
	assertElems(t, l.Range(-2, -1), []string{"d", "e"})
	assertElems(t, l.Range(0, 100), []string{"a", "b", "c", "d", "e"})
	if r := l.Range(3, 1); len(r) != 0 {
		t.Fatalf("Range(3,1) = %q, want empty", r)
	}
}

func TestListStoresCopiesOfItsInput(t *testing.T) {
	l := NewList()
	b := []byte("x")
	l.PushBack(b)
	b[0] = 'y'
	if v := l.Range(0, 0); string(v[0]) != "x" {
		t.Fatalf("element = %q, want x; PushBack aliased its input", v[0])
	}
}
