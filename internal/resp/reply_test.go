package resp

import "testing"

func TestEqualDistinguishesKinds(t *testing.T) {
	if Int(1).Equal(BulkString("1")) {
		t.Fatal("an integer reply compared equal to a bulk string")
	}
	if OK().Equal(Simple("PONG")) {
		t.Fatal("distinct simple strings compared equal")
	}
}

func TestEqualTreatsNilAndEmptyBulkAsEqual(t *testing.T) {
	if !Bulk(nil).Equal(Bulk([]byte{})) {
		t.Fatal("nil and empty bulk replies compared unequal")
	}
}

func TestEqualRecursesIntoArrays(t *testing.T) {
	a := Array([]Reply{Int(1), BulkString("x")})
	b := Array([]Reply{Int(1), BulkString("x")})
	c := Array([]Reply{Int(1), BulkString("y")})
	if !a.Equal(b) {
		t.Fatal("identical arrays compared unequal")
	}
	if a.Equal(c) {
		t.Fatal("arrays differing in an element compared equal")
	}
}

func TestStringRendersRepliesReadably(t *testing.T) {
	tests := []struct {
		reply Reply
		want  string
	}{
		{Nil(), "(nil)"},
		{OK(), "OK"},
		{Error("ERR boom"), "(error) ERR boom"},
		{Int(42), "42"},
		{BulkString("hi"), `"hi"`},
		{Array([]Reply{Int(1), Int(2)}), "[1 2]"},
	}
	for _, tt := range tests {
		if got := tt.reply.String(); got != tt.want {
			t.Fatalf("String() = %q, want %q", got, tt.want)
		}
	}
}
