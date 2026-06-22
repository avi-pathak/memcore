// Run against a local Memcore: node example.mjs
import { connect } from "./memcore.mjs";

const c = await connect({ host: "127.0.0.1", port: 6380 });
try {
  console.log("PING    ->", await c.ping());
  console.log("SET     ->", await c.set("greeting", "hello"));
  console.log("GET     ->", await c.get("greeting"));
  console.log("INCR    ->", await c.command("INCR", "counter"));
  await c.command("RPUSH", "fruits", "apple", "banana", "cherry");
  console.log("LRANGE  ->", await c.command("LRANGE", "fruits", "0", "-1"));
  await c.command("HSET", "user:1", "name", "ada", "role", "admin");
  console.log("HGETALL ->", await c.command("HGETALL", "user:1"));

  // Pipelining: fire several commands without awaiting between them. The client
  // resolves the replies in the order the commands were sent.
  const pipelined = await Promise.all([
    c.command("SET", "a", "1"),
    c.command("INCR", "a"),
    c.command("GET", "a"),
  ]);
  console.log("pipeline->", pipelined);
} finally {
  c.close();
}
