package value

import "testing"

func TestMakeStringStoresADefensiveCopy(t *testing.T) {
	b := []byte("hello")
	v := MakeString(b)
	b[0] = 'j'
	if got := string(v.Str()); got != "hello" {
		t.Fatalf("Str = %q, want %q: MakeString aliased its input", got, "hello")
	}
}

func TestKindReportsStringForAStringValue(t *testing.T) {
	v := MakeString([]byte("x"))
	if v.Kind() != String {
		t.Fatalf("Kind = %v, want String", v.Kind())
	}
}

func TestZeroValueHasKindNone(t *testing.T) {
	var v Value
	if v.Kind() != None {
		t.Fatalf("Kind = %v, want None", v.Kind())
	}
}

func TestStrPanicsOnANonStringValue(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("Str did not panic on a non-string value")
		}
	}()
	var v Value // kind None
	_ = v.Str()
}

func TestKindNamesMatchTheWireProtocol(t *testing.T) {
	tests := []struct {
		kind Kind
		want string
	}{
		{None, "none"},
		{String, "string"},
		{Kind(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.kind.String(); got != tt.want {
			t.Fatalf("Kind(%d).String() = %q, want %q", tt.kind, got, tt.want)
		}
	}
}
