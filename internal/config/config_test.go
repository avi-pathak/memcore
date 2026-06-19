package config

import (
	"errors"
	"testing"
)

func TestDefaultConfigIsValid(t *testing.T) {
	if err := Default().Validate(); err != nil {
		t.Fatalf("Default().Validate() = %v, want nil", err)
	}
}

func TestValidateRejectsInvalidConfigurations(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
	}{
		{"empty host", func(c *Config) { c.Network.Host = "" }},
		{"port below range", func(c *Config) { c.Network.Port = 0 }},
		{"port above range", func(c *Config) { c.Network.Port = 70000 }},
		{"zero databases", func(c *Config) { c.Network.Databases = 0 }},
		{"unknown log format", func(c *Config) { c.Log.Format = "xml" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Default()
			tt.mutate(&cfg)
			if err := cfg.Validate(); !errors.Is(err, ErrInvalid) {
				t.Fatalf("Validate() = %v, want error wrapping ErrInvalid", err)
			}
		})
	}
}

func TestNetworkAddrJoinsHostAndPort(t *testing.T) {
	n := Network{Host: "127.0.0.1", Port: 6380}
	if got, want := n.Addr(), "127.0.0.1:6380"; got != want {
		t.Fatalf("Addr() = %q, want %q", got, want)
	}
}

func TestNetworkAddrBracketsIPv6Literals(t *testing.T) {
	n := Network{Host: "::1", Port: 6380}
	if got, want := n.Addr(), "[::1]:6380"; got != want {
		t.Fatalf("Addr() = %q, want %q", got, want)
	}
}
