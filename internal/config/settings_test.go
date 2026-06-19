package config

import (
	"errors"
	"testing"
	"time"
)

func newTestSettings() *Settings {
	return NewSettings(Reloadable{
		SlowThreshold:  10 * time.Millisecond,
		ExpirySample:   20,
		SlowLogEnabled: true,
	})
}

func TestSettingsGetReportsCurrentValues(t *testing.T) {
	s := newTestSettings()
	if v, ok := s.Get("slowlog-threshold-ms"); !ok || v != "10" {
		t.Fatalf("Get(slowlog-threshold-ms) = (%q, %v), want (10, true)", v, ok)
	}
	if v, ok := s.Get("expiry-sample-per-shard"); !ok || v != "20" {
		t.Fatalf("Get(expiry-sample-per-shard) = (%q, %v), want (20, true)", v, ok)
	}
}

func TestSettingsGetReportsUnknownParameters(t *testing.T) {
	if _, ok := newTestSettings().Get("maxmemory"); ok {
		t.Fatal("Get reported an unknown parameter as known")
	}
}

func TestSettingsSetUpdatesAReloadableValue(t *testing.T) {
	s := newTestSettings()
	if err := s.Set("slowlog-threshold-ms", "50"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if got := s.Load().SlowThreshold; got != 50*time.Millisecond {
		t.Fatalf("SlowThreshold = %v, want 50ms", got)
	}
}

func TestSettingsSetRejectsInvalidValues(t *testing.T) {
	s := newTestSettings()
	for _, val := range []string{"-1", "notanumber"} {
		if err := s.Set("expiry-sample-per-shard", val); !errors.Is(err, ErrInvalid) {
			t.Fatalf("Set(expiry-sample-per-shard, %q) = %v, want ErrInvalid", val, err)
		}
	}
}

func TestSettingsSetRejectsUnknownParameters(t *testing.T) {
	if err := newTestSettings().Set("maxmemory", "100"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("Set of an unknown parameter = %v, want ErrInvalid", err)
	}
}

func TestSettingsAllListsEveryParameter(t *testing.T) {
	all := newTestSettings().All()
	if len(all) != 3 {
		t.Fatalf("All returned %d parameters, want 3", len(all))
	}
}
