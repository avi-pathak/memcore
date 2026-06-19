package metrics

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestObservedCommandsAppearInTheScrape(t *testing.T) {
	c := New()
	c.ObserveCommand("get", 200*time.Microsecond, false)
	c.ObserveCommand("get", 300*time.Microsecond, false)
	c.ObserveCommand("set", 1*time.Millisecond, true)

	body := scrape(t, c)
	for _, want := range []string{
		`memcore_commands_total{command="get"} 2`,
		`memcore_command_errors_total{command="set"} 1`,
		"memcore_command_duration_seconds_bucket",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("scrape missing %q\n---\n%s", want, body)
		}
	}
}

func TestConnectionGaugeTracksOpenConnections(t *testing.T) {
	c := New()
	c.ConnectionOpened()
	c.ConnectionOpened()
	c.ConnectionClosed()
	if body := scrape(t, c); !strings.Contains(body, "memcore_connections 1") {
		t.Fatalf("connection gauge not 1\n---\n%s", body)
	}
}

func scrape(t *testing.T, c *Collectors) string {
	t.Helper()
	srv := httptest.NewServer(c.Handler())
	defer srv.Close()
	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("scrape: %v", err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return string(b)
}
