package keyspace

import (
	"fmt"
	"testing"
	"time"

	"github.com/avinashpathak/memcore/internal/clock"
	"github.com/avinashpathak/memcore/internal/value"
)

func newTestKeyspace() (*Keyspace, *clock.ManualClock) {
	clk := clock.NewManualClock(time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC))
	return New(clk), clk
}

func TestGetReturnsAStoredValue(t *testing.T) {
	ks, _ := newTestKeyspace()
	ks.Set("k", value.MakeString([]byte("v")))
	got, ok := ks.Get("k")
	if !ok {
		t.Fatal("Get reported the key absent")
	}
	if string(got.Str()) != "v" {
		t.Fatalf("Get = %q, want %q", got.Str(), "v")
	}
}

func TestGetReportsAMissingKeyAsAbsent(t *testing.T) {
	ks, _ := newTestKeyspace()
	if _, ok := ks.Get("absent"); ok {
		t.Fatal("Get reported a missing key as present")
	}
}

func TestSetDiscardsAnExistingExpiry(t *testing.T) {
	ks, clk := newTestKeyspace()
	ks.Set("k", value.MakeString([]byte("v")))
	ks.SetExpire("k", clk.Now().Add(time.Minute))
	ks.Set("k", value.MakeString([]byte("v2")))
	e, ok := ks.Lookup("k")
	if !ok {
		t.Fatal("key vanished after Set")
	}
	if e.HasExpiry() {
		t.Fatal("Set did not clear the existing TTL")
	}
}

func TestAnExpiredKeyIsReportedAbsent(t *testing.T) {
	ks, clk := newTestKeyspace()
	ks.Set("k", value.MakeString([]byte("v")))
	ks.SetExpire("k", clk.Now().Add(10*time.Second))
	clk.Advance(11 * time.Second)
	if _, ok := ks.Get("k"); ok {
		t.Fatal("expired key still readable")
	}
}

func TestReadsDoNotReclaimExpiredEntries(t *testing.T) {
	ks, clk := newTestKeyspace()
	ks.Set("k", value.MakeString([]byte("v")))
	ks.SetExpire("k", clk.Now().Add(10*time.Second))
	clk.Advance(11 * time.Second)
	if _, ok := ks.Get("k"); ok {
		t.Fatal("expired key was visible to a read")
	}
	// Reads hide an expired entry but leave reclamation to writes and active
	// expiry, so the entry is still counted until then.
	if n := ks.Len(); n != 1 {
		t.Fatalf("Len = %d, want 1; a read must not reclaim", n)
	}
}

func TestDeletingAnExpiredKeyReportsItAbsentAndReclaimsIt(t *testing.T) {
	ks, clk := newTestKeyspace()
	ks.Set("k", value.MakeString([]byte("v")))
	ks.SetExpire("k", clk.Now().Add(10*time.Second))
	clk.Advance(11 * time.Second)
	if ks.Delete("k") {
		t.Fatal("Delete reported an expired key as live")
	}
	if n := ks.Len(); n != 0 {
		t.Fatalf("Len = %d, want 0; Delete must reclaim the entry", n)
	}
}

func TestExpiryIsInclusiveOfTheDeadline(t *testing.T) {
	ks, clk := newTestKeyspace()
	ks.Set("k", value.MakeString([]byte("v")))
	deadline := clk.Now().Add(5 * time.Second)
	ks.SetExpire("k", deadline)
	clk.Set(deadline)
	if _, ok := ks.Get("k"); ok {
		t.Fatal("key still live at its exact deadline")
	}
}

func TestSetExpireReturnsFalseForAMissingKey(t *testing.T) {
	ks, clk := newTestKeyspace()
	if ks.SetExpire("absent", clk.Now().Add(time.Minute)) {
		t.Fatal("SetExpire on a missing key reported success")
	}
}

func TestPersistRemovesATTLAndReportsIt(t *testing.T) {
	ks, clk := newTestKeyspace()
	ks.Set("k", value.MakeString([]byte("v")))
	ks.SetExpire("k", clk.Now().Add(time.Minute))
	if !ks.Persist("k") {
		t.Fatal("Persist did not report clearing the TTL")
	}
	e, _ := ks.Lookup("k")
	if e.HasExpiry() {
		t.Fatal("Persist left the TTL in place")
	}
}

func TestPersistReturnsFalseWithoutATTL(t *testing.T) {
	ks, _ := newTestKeyspace()
	ks.Set("k", value.MakeString([]byte("v")))
	if ks.Persist("k") {
		t.Fatal("Persist reported clearing a TTL that did not exist")
	}
}

func TestDeleteReportsPriorPresence(t *testing.T) {
	ks, _ := newTestKeyspace()
	ks.Set("k", value.MakeString([]byte("v")))
	if !ks.Delete("k") {
		t.Fatal("Delete reported an existing key as absent")
	}
	if ks.Delete("k") {
		t.Fatal("Delete reported a removed key as present")
	}
}

func TestFlushEmptiesTheKeyspace(t *testing.T) {
	ks, _ := newTestKeyspace()
	ks.Set("a", value.MakeString([]byte("1")))
	ks.Set("b", value.MakeString([]byte("2")))
	ks.Flush()
	if n := ks.Len(); n != 0 {
		t.Fatalf("Len = %d, want 0 after Flush", n)
	}
}

func TestSampleExpiredEvictsOnlyExpiredVolatileKeys(t *testing.T) {
	ks, clk := newTestKeyspace()
	ks.Set("live", value.MakeString([]byte("v")))
	ks.SetExpire("live", clk.Now().Add(time.Hour))
	ks.Set("dead", value.MakeString([]byte("v")))
	ks.SetExpire("dead", clk.Now().Add(time.Second))
	clk.Advance(2 * time.Second)

	_, evicted := ks.SampleExpired(clk.Now(), 100)
	if evicted != 1 {
		t.Fatalf("evicted = %d, want 1", evicted)
	}
	if ks.Exists("dead") {
		t.Fatal("expired key survived active expiry")
	}
	if !ks.Exists("live") {
		t.Fatal("a live key was evicted")
	}
}

func TestSampleExpiredRespectsItsLimit(t *testing.T) {
	ks, clk := newTestKeyspace()
	for i := 0; i < 10; i++ {
		key := fmt.Sprintf("k%d", i)
		ks.Set(key, value.MakeString([]byte("v")))
		ks.SetExpire(key, clk.Now().Add(time.Second))
	}
	clk.Advance(2 * time.Second)
	if examined, _ := ks.SampleExpired(clk.Now(), 3); examined != 3 {
		t.Fatalf("examined = %d, want exactly the limit of 3", examined)
	}
}

func TestSampleExpiredIgnoresKeysWithoutATTL(t *testing.T) {
	ks, clk := newTestKeyspace()
	ks.Set("permanent", value.MakeString([]byte("v")))
	examined, evicted := ks.SampleExpired(clk.Now(), 100)
	if examined != 0 || evicted != 0 {
		t.Fatalf("examined=%d evicted=%d, want 0 and 0 with no volatile keys", examined, evicted)
	}
}

func TestTakeRemovesAndReportsLiveness(t *testing.T) {
	ks, _ := newTestKeyspace()
	ks.Set("k", value.MakeString([]byte("v")))
	e, live := ks.Take("k")
	if !live {
		t.Fatal("Take reported a live key as not live")
	}
	if string(e.Value.Str()) != "v" {
		t.Fatalf("Take value = %q, want v", e.Value.Str())
	}
	if ks.Exists("k") {
		t.Fatal("Take did not remove the key")
	}
}
