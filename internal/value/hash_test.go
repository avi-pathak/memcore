package value

import "testing"

func TestHashSetReportsNewFields(t *testing.T) {
	h := NewHash()
	if !h.Set("f", []byte("1")) {
		t.Fatal("Set of a new field reported it as existing")
	}
	if h.Set("f", []byte("2")) {
		t.Fatal("Set of an existing field reported it as new")
	}
	if v, _ := h.Get("f"); string(v) != "2" {
		t.Fatalf("Get = %q, want 2 after overwrite", v)
	}
}

func TestHashDeleteReportsPresence(t *testing.T) {
	h := NewHash()
	h.Set("f", []byte("v"))
	if !h.Delete("f") {
		t.Fatal("Delete of an existing field reported it absent")
	}
	if h.Delete("f") {
		t.Fatal("Delete of a removed field reported it present")
	}
}

func TestHashStoresCopiesOfValues(t *testing.T) {
	h := NewHash()
	val := []byte("v")
	h.Set("f", val)
	val[0] = 'x'
	if v, _ := h.Get("f"); string(v) != "v" {
		t.Fatalf("value = %q, want v; Set aliased its input", v)
	}
}

func TestHashPairsReturnsEveryField(t *testing.T) {
	h := NewHash()
	h.Set("a", []byte("1"))
	h.Set("b", []byte("2"))
	pairs := h.Pairs()
	if len(pairs) != 4 {
		t.Fatalf("Pairs returned %d items, want 4", len(pairs))
	}
	got := make(map[string]string)
	for i := 0; i < len(pairs); i += 2 {
		got[string(pairs[i])] = string(pairs[i+1])
	}
	if got["a"] != "1" || got["b"] != "2" {
		t.Fatalf("Pairs = %v, want a=1 and b=2", got)
	}
}
