// Package metrics exposes Memcore's runtime metrics over Prometheus. The
// collectors here are the only place the project depends on the Prometheus
// client; the server records through a small interface it defines, so the metric
// backend stays replaceable.
package metrics

import (
	"net"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Collectors holds Memcore's Prometheus metrics. It is safe for concurrent use;
// every method may be called from any connection goroutine.
type Collectors struct {
	commands    *prometheus.CounterVec
	errors      *prometheus.CounterVec
	latency     *prometheus.HistogramVec
	connections prometheus.Gauge
	registry    *prometheus.Registry
}

// New builds the collectors and registers them on a private registry, so the
// exposed surface holds only Memcore's metrics plus the standard Go runtime
// collectors.
func New() *Collectors {
	reg := prometheus.NewRegistry()
	c := &Collectors{
		commands: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "memcore",
			Name:      "commands_total",
			Help:      "Commands processed, labeled by command name.",
		}, []string{"command"}),
		errors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "memcore",
			Name:      "command_errors_total",
			Help:      "Commands that returned an error reply, labeled by command name.",
		}, []string{"command"}),
		latency: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "memcore",
			Name:      "command_duration_seconds",
			Help:      "Command execution latency, labeled by command name.",
			Buckets:   prometheus.ExponentialBuckets(50e-9, 4, 12), // ~50ns to ~3.4ms
		}, []string{"command"}),
		connections: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "memcore",
			Name:      "connections",
			Help:      "Currently open client connections.",
		}),
		registry: reg,
	}
	reg.MustRegister(c.commands, c.errors, c.latency, c.connections)
	reg.MustRegister(collectors.NewGoCollector())
	return c
}

// ObserveCommand records one command execution: its name, latency, and whether
// it returned an error.
func (c *Collectors) ObserveCommand(name string, d time.Duration, isError bool) {
	c.commands.WithLabelValues(name).Inc()
	c.latency.WithLabelValues(name).Observe(d.Seconds())
	if isError {
		c.errors.WithLabelValues(name).Inc()
	}
}

// ConnectionOpened increments the open-connection gauge.
func (c *Collectors) ConnectionOpened() { c.connections.Inc() }

// ConnectionClosed decrements the open-connection gauge.
func (c *Collectors) ConnectionClosed() { c.connections.Dec() }

// Handler returns an HTTP handler that serves the metrics in the Prometheus text
// format.
func (c *Collectors) Handler() http.Handler {
	return promhttp.HandlerFor(c.registry, promhttp.HandlerOpts{})
}

// Server wraps the collectors in an HTTP server that exposes /metrics. It owns
// no goroutine of its own; the caller runs Serve and calls Shutdown.
type Server struct {
	http *http.Server
}

// NewServer returns a metrics HTTP server bound to addr that serves c at
// /metrics.
func NewServer(addr string, c *Collectors) *Server {
	mux := http.NewServeMux()
	mux.Handle("/metrics", c.Handler())
	return &Server{http: &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}}
}

// Serve listens on ln and blocks until the server is shut down, returning nil on
// a clean shutdown.
func (s *Server) Serve(ln net.Listener) error {
	if err := s.http.Serve(ln); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Shutdown stops the metrics server.
func (s *Server) Shutdown() error { return s.http.Close() }
