package value

import "testing"

// Thresholds used across the collection tests: bigThresholds keeps a collection
// in its compact encoding; smallThresholds forces promotion early.
var (
	bigThresholds   = Thresholds{MaxEntries: 1 << 20, MaxBytes: 1 << 20}
	smallThresholds = Thresholds{MaxEntries: 3, MaxBytes: 8}
)

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

func TestListBehavesTheSameCompactAndPromoted(t *testing.T) {
	cases := []struct {
		name string
		max  Thresholds
	}{
		{"compact", bigThresholds},
		{"promoted", smallThresholds},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			l := NewList(tc.max)
			for _, s := range []string{"a", "b", "c", "d", "e"} {
				l.PushBack([]byte(s))
			}
			l.PushFront([]byte("z")) // z, a, b, c, d, e
			if l.Len() != 6 {
				t.Fatalf("Len = %d, want 6", l.Len())
			}
			assertElems(t, l.Range(0, -1), []string{"z", "a", "b", "c", "d", "e"})
			assertElems(t, l.Range(1, 3), []string{"a", "b", "c"})
			assertElems(t, l.Range(-2, -1), []string{"d", "e"})
			if v, _ := l.PopFront(); string(v) != "z" {
				t.Fatalf("PopFront = %q, want z", v)
			}
			if v, _ := l.PopBack(); string(v) != "e" {
				t.Fatalf("PopBack = %q, want e", v)
			}
			if l.Len() != 4 {
				t.Fatalf("Len = %d, want 4", l.Len())
			}
		})
	}
}

func TestListPopFromEmptyReportsAbsent(t *testing.T) {
	l := NewList(bigThresholds)
	if _, ok := l.PopFront(); ok {
		t.Fatal("PopFront on an empty list reported a value")
	}
	if _, ok := l.PopBack(); ok {
		t.Fatal("PopBack on an empty list reported a value")
	}
}

func TestListStoresCopiesOfItsInput(t *testing.T) {
	l := NewList(bigThresholds)
	b := []byte("x")
	l.PushBack(b)
	b[0] = 'y'
	if v := l.Range(0, 0); string(v[0]) != "x" {
		t.Fatalf("element = %q, want x; PushBack aliased its input", v[0])
	}
}

func TestListPromotesPastItsEntryThreshold(t *testing.T) {
	l := NewList(Thresholds{MaxEntries: 3, MaxBytes: 1 << 20})
	if !l.Compact() {
		t.Fatal("a new list should be compact")
	}
	for _, s := range []string{"a", "b", "c"} {
		l.PushBack([]byte(s))
	}
	if !l.Compact() {
		t.Fatal("a list at its threshold should still be compact")
	}
	l.PushBack([]byte("d")) // crosses MaxEntries
	if l.Compact() {
		t.Fatal("a list past its threshold should have promoted")
	}
	assertElems(t, l.Range(0, -1), []string{"a", "b", "c", "d"})
}

func TestListPromotesOnAnOversizedElement(t *testing.T) {
	l := NewList(Thresholds{MaxEntries: 1 << 20, MaxBytes: 4})
	l.PushBack([]byte("ok"))
	if !l.Compact() {
		t.Fatal("a small element should keep the list compact")
	}
	l.PushBack([]byte("a value larger than four bytes"))
	if l.Compact() {
		t.Fatal("an oversized element should have promoted the list")
	}
}
