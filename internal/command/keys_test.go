package command

import (
	"testing"
	"time"

	"github.com/avinashpathak/memcore/internal/resp"
)

func TestDelReportsTheNumberOfKeysRemoved(t *testing.T) {
	r, ctx, _ := newTestEnv()
	run(r, ctx, "SET", "a", "1")
	run(r, ctx, "SET", "b", "2")
	if got := run(r, ctx, "DEL", "a", "b", "missing"); !got.Equal(resp.Int(2)) {
		t.Fatalf("DEL = %v, want 2", got)
	}
}

func TestUnlinkRemovesKeysLikeDel(t *testing.T) {
	r, ctx, _ := newTestEnv()
	run(r, ctx, "SET", "a", "1")
	if got := run(r, ctx, "UNLINK", "a"); !got.Equal(resp.Int(1)) {
		t.Fatalf("UNLINK = %v, want 1", got)
	}
}

func TestExistsCountsPresentKeysIncludingDuplicates(t *testing.T) {
	r, ctx, _ := newTestEnv()
	run(r, ctx, "SET", "a", "1")
	if got := run(r, ctx, "EXISTS", "a", "a", "missing"); !got.Equal(resp.Int(2)) {
		t.Fatalf("EXISTS = %v, want 2", got)
	}
}

func TestExpireSetsATTLThatTTLReports(t *testing.T) {
	r, ctx, _ := newTestEnv()
	run(r, ctx, "SET", "k", "v")
	if got := run(r, ctx, "EXPIRE", "k", "100"); !got.Equal(resp.Int(1)) {
		t.Fatalf("EXPIRE = %v, want 1", got)
	}
	if got := run(r, ctx, "TTL", "k"); !got.Equal(resp.Int(100)) {
		t.Fatalf("TTL = %v, want 100", got)
	}
}

func TestExpireOnAMissingKeyReportsZero(t *testing.T) {
	r, ctx, _ := newTestEnv()
	if got := run(r, ctx, "EXPIRE", "absent", "100"); !got.Equal(resp.Int(0)) {
		t.Fatalf("EXPIRE = %v, want 0", got)
	}
}

func TestTTLDistinguishesMissingKeyFromNoExpiry(t *testing.T) {
	r, ctx, _ := newTestEnv()
	if got := run(r, ctx, "TTL", "absent"); !got.Equal(resp.Int(-2)) {
		t.Fatalf("TTL absent = %v, want -2", got)
	}
	run(r, ctx, "SET", "k", "v")
	if got := run(r, ctx, "TTL", "k"); !got.Equal(resp.Int(-1)) {
		t.Fatalf("TTL without expiry = %v, want -1", got)
	}
}

func TestPersistClearsATTL(t *testing.T) {
	r, ctx, _ := newTestEnv()
	run(r, ctx, "SET", "k", "v")
	run(r, ctx, "EXPIRE", "k", "100")
	if got := run(r, ctx, "PERSIST", "k"); !got.Equal(resp.Int(1)) {
		t.Fatalf("PERSIST = %v, want 1", got)
	}
	if got := run(r, ctx, "TTL", "k"); !got.Equal(resp.Int(-1)) {
		t.Fatalf("TTL after PERSIST = %v, want -1", got)
	}
}

func TestExpiredKeyIsGoneAfterTheClockAdvances(t *testing.T) {
	r, ctx, clk := newTestEnv()
	run(r, ctx, "SET", "k", "v")
	run(r, ctx, "EXPIRE", "k", "10")
	clk.Advance(11 * time.Second)
	if got := run(r, ctx, "GET", "k"); !got.Equal(resp.Nil()) {
		t.Fatalf("GET after expiry = %v, want nil", got)
	}
}

func TestTypeReportsStringOrNone(t *testing.T) {
	r, ctx, _ := newTestEnv()
	if got := run(r, ctx, "TYPE", "absent"); !got.Equal(resp.Simple("none")) {
		t.Fatalf("TYPE absent = %v, want none", got)
	}
	run(r, ctx, "SET", "k", "v")
	if got := run(r, ctx, "TYPE", "k"); !got.Equal(resp.Simple("string")) {
		t.Fatalf("TYPE string = %v, want string", got)
	}
}
