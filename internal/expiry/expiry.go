// Package expiry runs background active expiry: it periodically samples keys
// that carry a TTL and evicts those past their deadline, bounded by a per-cycle
// work budget so reclamation never causes a latency spike.
package expiry

import (
	"context"
	"log/slog"
	"time"

	"github.com/avinashpathak/memcore/internal/clock"
	"github.com/avinashpathak/memcore/internal/shard"
)

// Config tunes the active-expiry loop.
type Config struct {
	Interval time.Duration // wall-clock time between cycles
	// SamplePerShard returns the per-shard work budget for a cycle. It is read
	// each cycle so an operator can adjust it through CONFIG SET without a
	// restart. A non-positive return disables that cycle.
	SamplePerShard func() int
}

// Runner evicts expired keys from a set of databases on a fixed interval. Run
// owns the loop and is meant to run on its own goroutine; it blocks until its
// context is canceled.
type Runner struct {
	dbs   []*shard.DB
	clock clock.Clock
	cfg   Config
	log   *slog.Logger
}

// New returns a Runner over dbs.
func New(dbs []*shard.DB, clk clock.Clock, cfg Config, log *slog.Logger) *Runner {
	return &Runner{dbs: dbs, clock: clk, cfg: cfg, log: log}
}

// Run drives active-expiry cycles until ctx is canceled. A non-positive interval
// or a nil sample function disables the loop, leaving expiry to the write path.
func (r *Runner) Run(ctx context.Context) {
	if r.cfg.Interval <= 0 || r.cfg.SamplePerShard == nil {
		<-ctx.Done()
		return
	}
	ticker := time.NewTicker(r.cfg.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.cycle()
		}
	}
}

// cycle runs one active-expiry pass over every database, returning how many keys
// it examined and evicted. It is separate from Run so tests can drive it against
// a manual clock without waiting on the ticker.
func (r *Runner) cycle() (examined, evicted int) {
	limit := r.cfg.SamplePerShard()
	if limit <= 0 {
		return 0, 0
	}
	now := r.clock.Now()
	for _, db := range r.dbs {
		ex, ev := db.ExpireCycle(now, limit)
		examined += ex
		evicted += ev
	}
	if evicted > 0 {
		r.log.Debug("active expiry reclaimed keys", "examined", examined, "evicted", evicted)
	}
	return examined, evicted
}
