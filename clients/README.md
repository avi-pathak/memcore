# Memcore clients

Memcore speaks RESP2, the Redis wire protocol. The most important consequence is
worth stating plainly:

> **Every existing Redis client already works with Memcore.** Point it at the
> Memcore host and port (default 6380) and use it as you would with Redis, within
> the [supported command set](../README.md#supported-commands).

So in most languages you do not need a Memcore-specific client at all. This
directory provides two things:

1. Examples of using the **stock Redis client** for each language (recommended).
2. Small, **zero-dependency native clients** for Node.js, Python, and Go, for
   when a minimal footprint or a reference implementation is preferable.

## Using a stock Redis client (recommended)

These are mature, full-featured libraries. Memcore is a drop-in target for the
commands it implements.

Node.js, with [ioredis](https://github.com/redis/ioredis):

```js
import Redis from "ioredis";
const redis = new Redis({ host: "127.0.0.1", port: 6380 });
await redis.set("greeting", "hello");
console.log(await redis.get("greeting")); // hello
```

Python, with [redis-py](https://github.com/redis/redis-py):

```python
import redis
r = redis.Redis(host="127.0.0.1", port=6380, decode_responses=True)
r.set("greeting", "hello")
print(r.get("greeting"))  # hello
```

Go, with [go-redis](https://github.com/redis/go-redis):

```go
rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:6380"})
rdb.Set(ctx, "greeting", "hello", 0)
val, _ := rdb.Get(ctx, "greeting").Result() // "hello"
```

Java ([Jedis](https://github.com/redis/jedis)), Rust
([redis-rs](https://github.com/redis-rs/redis-rs)), Ruby, PHP, and .NET clients
work the same way: connect to port 6380.

### Compatibility matrix

| Language | Stock client | Works | Native client here |
| --- | --- | :-: | :-: |
| Node.js | ioredis, node-redis | yes | yes |
| Python | redis-py | yes | yes |
| Go | go-redis, redigo | yes | yes |
| Java | Jedis, Lettuce | yes | -- |
| Rust | redis-rs | yes | -- |
| Ruby | redis-rb | yes | -- |
| PHP | phpredis, Predis | yes | -- |
| .NET | StackExchange.Redis | yes | -- |
| CLI | redis-cli | yes | -- |

"Works" means the commands Memcore implements behave as the client expects.
Features Memcore does not implement -- authentication, pub/sub, transactions,
scripting, cluster -- are not available through any client; see
[Limitations](../README.md#limitations).

## Native zero-dependency clients

Use these when you want no third-party dependency, a small audit surface, or a
short reference for the protocol. Each supports a generic command call plus
helpers for the common operations, and each is binary-safe.

### Node.js ([node/](node/))

```js
import { connect } from "./memcore.mjs";
const c = await connect({ host: "127.0.0.1", port: 6380 });
await c.set("k", "v");
await c.get("k");                    // "v"
await c.command("INCR", "counter");  // generic command
c.close();
```

The client parses replies incrementally, so a reply split across TCP packets is
handled correctly, and pipelined commands resolve in order. Run the example with
`node node/example.mjs`.

### Python ([python/](python/))

```python
from memcore import Memcore
with Memcore(host="127.0.0.1", port=6380) as c:
    c.set("k", "v")
    print(c.get("k"))            # "v"
    c.command("INCR", "counter") # generic command
```

Run the example with `python python/example.py`.

### Go ([go/](go/))

The Go client is a separate module with no dependencies, so importing it does not
pull in the server's dependencies.

```go
import memcore "github.com/avinashpathak/memcore/clients/go"

c, err := memcore.Dial("127.0.0.1:6380")
if err != nil { /* ... */ }
defer c.Close()

c.Set("k", "v")
v, found, _ := c.Get("k")     // "v", true
n, _ := c.Del("k")            // 1
reply, _ := c.Do("INCR", "n") // generic command
```

Run the example with `cd go/example && go run .`.

## Notes

- Replies map to native types: simple and bulk strings to the language's string,
  integers to its integer type, a null bulk or array to its null value, and
  arrays to its list type.
- A server error reply (`-ERR ...`) surfaces as a typed error: `MemcoreError` in
  Node and Python, `*memcore.Error` in Go.
- These clients keep one request in flight per call. They are intentionally
  small; for connection pooling, automatic reconnection, or the full command
  surface, use a stock Redis client.
