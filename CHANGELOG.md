# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project aims to
follow semantic versioning once it reaches a tagged release.

## Unreleased

### Added

- RESP2 server speaking the Redis wire protocol over TCP, with a streaming
  parser that handles requests split across packet boundaries and is fuzzed
  against split and malformed input.
- Commands: `GET`, `SET`, `INCR`, `DECR`, `DEL`, `UNLINK`, `EXISTS`, `EXPIRE`,
  `PEXPIREAT`, `TTL`, `PERSIST`, `TYPE`, `LPUSH`, `RPUSH`, `LPOP`, `RPOP`,
  `LRANGE`, `LLEN`, `HSET`, `HGET`, `HDEL`, `HGETALL`, `SADD`, `SREM`,
  `SMEMBERS`, `SINTER`, `ZADD`, `ZRANGE`, `ZSCORE`, `PING`, `SELECT`, `CONFIG`,
  and `FLUSHDB`.
- Multiple logical databases via `SELECT`.
- Keyspace sharded across independently locked shards for intra-database
  parallelism on single-key commands.
- Compact packed encodings for small lists, hashes, sets, and sorted sets, with
  configurable promotion thresholds to the full data structures.
- Background active expiry with a bounded per-cycle work budget, and `UNLINK`
  freeing of large collections on a background reaper.
- Fork-free persistence: a segmented append log with `always`, `everysec`, and
  `no` fsync policies, periodic snapshots, and crash-safe recovery that replays
  the log over the latest snapshot.
- Prometheus metrics with per-command latency histograms, a slow-command log,
  and runtime reload of a documented subset of configuration via `CONFIG SET`.
- Multi-stage Docker build producing a distroless, non-root, digest-pinned image;
  Makefile targets for multi-arch images; and CI running format, vet,
  staticcheck, and the race detector, with a tag-triggered Docker Hub release.
