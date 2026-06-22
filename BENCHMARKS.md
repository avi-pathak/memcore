# Memcore vs Redis: a measured comparison

This document compares Memcore against Redis using real benchmarks, not
assertions. It follows the project's stance: Memcore is not "faster Redis," and
the numbers below are reported as they came out, including where Memcore loses.
Everything here is reproducible from the methodology section.

Date of run: 2026-06-19. Hardware: Intel Core i7-1185G7, Windows, Docker
Desktop. Redis 7 (alpine). Memcore built from this tree, distroless image.

## TL;DR

- **Raw throughput: Redis wins, by roughly 2x to 5.5x** depending on the command
  and whether requests are pipelined. This is expected; Redis is fifteen years
  of C and jemalloc tuning.
- **Memory on small objects: Redis wins, by roughly 4x.** This is the result
  that matters most, because memory efficiency is one of Memcore's stated goals.
  The measurement does not support claiming an advantage there. See
  [Honest conclusions](#honest-conclusions).
- **Memcore is competitive on a few operations** (list pops in the
  non-pipelined run) and its per-command CPU cost in-process is low.
- The differentiators Memcore is actually designed around -- per-shard
  parallelism under concurrent multi-core load, and fork-free persistence tail
  latency -- are **not captured** by these micro-benchmarks and are not claimed
  as proven wins here.

## Methodology

Fairness matters, so both servers were driven identically:

- Redis (`redis:7-alpine`) and Memcore (this repo's image) ran as containers on
  the same user-defined Docker bridge network.
- A third container ran `redis-benchmark` and reached **both** servers over that
  same bridge, so neither got a shorter network path than the other.
- Identical flags for every throughput run. The command set was restricted to
  commands Memcore implements.

```
# non-pipelined: 100k requests, 50 connections, randomized keyspace
redis-benchmark -h <srv> -p <port> -n 100000 -c 50 -r 100000 \
  -t ping,set,get,incr,lpush,rpush,lpop,rpop,sadd,hset --csv

# pipelined: 300k requests, 50 connections, pipeline depth 16
redis-benchmark -h <srv> -p <port> -n 300000 -c 50 -P 16 -t set,get --csv
```

The memory test loaded 200,000 small hashes (`HSET h:<i> a 1 b 22 c 333`, three
short fields each) into each server through one pipelined connection, then read
container RSS with `docker stats --no-stream`. Both servers keep small hashes in
a compact encoding (Redis listpack, Memcore packed bytes), so this is a
compact-vs-compact comparison, not compact-vs-naive.

Caveats, stated up front:

- Docker Desktop's networking on Windows adds latency and noise, which
  compresses absolute throughput for both servers. Ratios are more trustworthy
  than absolute numbers.
- `redis-benchmark` prints `WARNING: Could not fetch server CONFIG` against
  Memcore. That is expected: Memcore's `CONFIG GET` only exposes its reloadable
  subset, so the benchmark's `CONFIG GET save` returns nothing. It does not
  affect the measured operations.
- Memcore runs on the Go runtime with a garbage collector. At the default
  `GOGC=100`, resident memory includes roughly a generation of GC headroom, so
  part of its memory figure is collectable slack rather than live data.

## In-process command cost (Memcore only)

Memcore's own Go benchmarks measure the command path with no network, which
isolates CPU and allocation cost:

```
BenchmarkSet-8   8629176   232.8 ns/op   64 B/op   3 allocs/op
BenchmarkGet-8   9066991   254.9 ns/op    8 B/op   1 allocs/op
```

So the engine itself is not the bottleneck in the networked numbers below;
syscalls, the RESP round trip, and the Docker network path dominate.

## Throughput, non-pipelined (50 connections, pipeline depth 1)

Requests per second; higher is better. Ratio is Memcore / Redis.

| Command | Redis rps | Memcore rps | Memcore / Redis |
| --- | ---: | ---: | ---: |
| PING_INLINE | 81,833 | 31,397 | 0.38x |
| PING_MBULK | 65,274 | 29,976 | 0.46x |
| SET | 88,417 | 38,790 | 0.44x |
| GET | 95,329 | 34,977 | 0.37x |
| INCR | 98,522 | 25,361 | 0.26x |
| LPUSH | 90,009 | 35,373 | 0.39x |
| RPUSH | 68,353 | 43,573 | 0.64x |
| LPOP | 30,553 | 45,977 | **1.50x** |
| RPOP | 71,736 | 43,995 | 0.61x |
| SADD | 71,788 | 31,606 | 0.44x |
| HSET | 71,788 | 37,651 | 0.52x |

Redis is faster on everything except `LPOP`, where this particular run showed
Redis with a high p95/p99 tail and Memcore ahead. Treat the single LPOP win
cautiously; it is one run on a noisy network.

## Throughput, pipelined (50 connections, pipeline depth 16)

| Command | Redis rps | Memcore rps | Memcore / Redis |
| --- | ---: | ---: | ---: |
| SET | 689,655 | 124,740 | 0.18x |
| GET | 598,802 | 281,426 | 0.47x |

Pipelining widens the gap on writes: Redis reaches ~690k SET/s, Memcore ~125k.
Memcore's read path (one allocation, shared lock) holds up far better than its
write path (value copy, exclusive lock, append-log formatting).

## Memory: 200,000 small hashes

Resident set size after loading identical data; lower is better.

| Server | Baseline | After 200k hashes | Data delta | Per hash |
| --- | ---: | ---: | ---: | ---: |
| Redis | 8.2 MiB | 26.5 MiB | ~18.3 MiB | ~96 B |
| Memcore | 5.1 MiB | 110.9 MiB | ~105.8 MiB | ~555 B |

Memcore used about **4x** the total memory and roughly **5-6x** per object on the
exact workload its compact encoding targets.

Where the bytes go in Memcore:

- The Go map storing entries has higher per-entry overhead than Redis's dict.
- Every stored entry carries a `time.Time` expiry field (24 bytes) whether or
  not it has a TTL.
- The tagged `Value` struct carries a byte-slice header plus several pointers,
  most unused for any given kind, so even a hash value pays for fields it does
  not use.
- The GC keeps headroom at `GOGC=100`, so a portion of the 110 MiB is
  collectable slack, not live data. Even discounting that generously, Memcore
  remains well above Redis.

The compact encoding is still the right call: without it, each small hash would
be a full Go map and the figure would be much worse. It closes the gap to a
naive Go implementation; it does not close the gap to Redis.

## Honest conclusions

- On the two things these benchmarks measure -- raw throughput and small-object
  memory -- **Redis is better**, decisively on memory and by 2-5x on throughput.
  Positioning Memcore as faster or leaner than Redis would not survive contact
  with these numbers.
- Memcore's defensible value is elsewhere and is **not proven by this document**:
  per-shard locking gives real intra-server parallelism that a single
  `redis-benchmark` stream does not exercise, and fork-free persistence is about
  tail-latency behavior during snapshots on large datasets, which these runs did
  not measure. Those remain design intentions until benchmarked the same honest
  way.
- The most actionable finding is the memory result. If memory efficiency is to
  be a headline, the `Value` struct and `Entry` layout need work (a smaller
  value representation, separating expiry out of the common path) and the GC
  needs tuning (`GOMEMLIMIT`), and then it should be re-measured. Until then, the
  README's "memory efficiency on small objects" framing overstates what the code
  currently delivers against Redis.

## Reproducing

```
# build the image
docker build -t memcore/memcored:bench .

# topology
docker network create benchnet
docker run -d --name redis-srv  --network benchnet redis:7-alpine
docker run -d --name memcore-srv --network benchnet memcore/memcored:bench
docker run -d --name bench-client --network benchnet redis:7-alpine sleep infinity

# throughput
docker exec bench-client redis-benchmark -h redis-srv  -p 6379 -n 100000 -c 50 -r 100000 -t set,get,incr,hset --csv
docker exec bench-client redis-benchmark -h memcore-srv -p 6380 -n 100000 -c 50 -r 100000 -t set,get,incr,hset --csv

# Memcore in-process micro-benchmarks
go test ./internal/command/ -run='^$' -bench='BenchmarkGet|BenchmarkSet' -benchmem
```
