# Memcore

Memcore is a Redis-protocol-compatible in-memory data store written in Go. It
speaks RESP2 over TCP, so existing Redis clients and `redis-cli` work against it
unchanged, and it is built to run as the core engine of a hosted service.

It is not a drop-in replacement for Redis, and it is not positioned as "faster
Redis." A Go reimplementation will not beat fifteen years of C tuning on raw
throughput. Memcore is built instead for three things an operator pays for:
predictable memory on small objects, persistence that does not stall on large
datasets, and first-class observability. See [ARCHITECTURE.md](ARCHITECTURE.md)
for how it works and why.

## Running

Build and run from source:

```
go build ./cmd/memcored
./memcored --port 6380
```

Or with Docker:

```
docker run --rm -p 6380:6380 memcore/memcored:latest
```

Then point any Redis client at it:

```
redis-cli -p 6380 ping
redis-cli -p 6380 set greeting hello
redis-cli -p 6380 get greeting
```

### Configuration

Every option has a flag; defaults are conservative and the listener binds
loopback unless told otherwise.

| Flag | Default | Purpose |
| --- | --- | --- |
| `-host` | `127.0.0.1` | Listener bind address. |
| `-port` | `6380` | Listener port. |
| `-databases` | `16` | Number of logical databases reachable with `SELECT`. |
| `-shards` | `GOMAXPROCS` | Shards per database; raises intra-database parallelism. |
| `-persistence` | `false` | Enable the snapshot and append log. |
| `-data-dir` | `data` | Directory for persistence files. |
| `-fsync` | `everysec` | Append-log durability: `always`, `everysec`, or `no`. |
| `-metrics` | `false` | Expose Prometheus metrics over HTTP. |
| `-metrics-port` | `9121` | Port for the metrics endpoint. |
| `-log-format` | `text` | `text` or `json`. |
| `-log-level` | `info` | `debug`, `info`, `warn`, or `error`. |

A documented subset of configuration reloads at runtime with `CONFIG SET`,
without a restart: `slowlog-threshold-ms`, `expiry-sample-per-shard`, and
`slowlog-enabled`.

## Supported commands

```
Strings   GET SET INCR DECR
Keys      DEL UNLINK EXISTS EXPIRE PEXPIREAT TTL PERSIST TYPE
Lists     LPUSH RPUSH LPOP RPOP LRANGE LLEN
Hashes    HSET HGET HDEL HGETALL
Sets      SADD SREM SMEMBERS SINTER
ZSets     ZADD ZRANGE ZSCORE
Server    PING SELECT CONFIG FLUSHDB
```

## Protocol notes

- RESP2 is the wire protocol. The reply layer is structured so RESP3 is an
  additive change rather than a rewrite.
- The parser handles a single request split across TCP packet boundaries and is
  fuzzed against split and malformed input.
- Inline commands (the form a raw terminal sends) are accepted alongside the
  multi-bulk form clients use.
- Errors use the conventional uppercase prefixes, including `WRONGTYPE` and
  `ERR`.

## Persistence model

Persistence is off by default. When enabled, writes append to a segmented log
with one of three fsync policies, and a background goroutine periodically folds
the log into a snapshot. Compaction does not fork the process: the live state is
captured into memory under a brief lock, then written to disk off the lock. On
boot the server loads the latest snapshot and replays the log over it. A write
left half-written by a crash is detected and ignored. The tradeoff, discussed in
[ARCHITECTURE.md](ARCHITECTURE.md), is an honest one: avoiding fork trades
copy-on-write memory growth for a short pause proportional to the live data.

## Observability

With `-metrics`, the server exposes Prometheus metrics at `/metrics`:
per-command counters, a per-command latency histogram, an error counter, and an
open-connection gauge. Commands slower than a configurable threshold are written
to a slow-command log. Both the threshold and the active-expiry sample size are
reloadable at runtime.

## Limitations

Memcore implements a focused subset of Redis. It deliberately does not (yet) have:

- Authentication, ACLs, or TLS. Run it on a trusted network or behind a proxy.
- Replication, clustering, or any multi-node mode. It is a single node.
- The full command set. There is no `SCAN`, no pub/sub, no transactions
  (`MULTI`/`EXEC`), no Lua scripting, and no streams.
- Command options beyond the basics: `SET` has no `EX`/`NX`/`GET`, `ZADD` has no
  `GT`/`LT`/`NX`/`XX`, and `EXPIRE` has no `NX`/`XX`/`GT`/`LT`.
- Field-level TTL on hashes (`HEXPIRE`). The keyspace supports per-entry expiry;
  per-field expiry is future work.
- Eviction under a memory ceiling (no `maxmemory` policy).

These are scope choices, not architectural limits, and the layering is meant to
make them additive.

## License

MIT. See [LICENSE](LICENSE).
