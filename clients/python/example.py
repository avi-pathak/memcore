"""Run against a local Memcore: python example.py"""

from memcore import Memcore

with Memcore(host="127.0.0.1", port=6380) as c:
    print("PING    ->", c.ping())
    print("SET     ->", c.set("greeting", "hello"))
    print("GET     ->", c.get("greeting"))
    print("INCR    ->", c.command("INCR", "counter"))

    c.command("RPUSH", "fruits", "apple", "banana", "cherry")
    print("LRANGE  ->", c.command("LRANGE", "fruits", "0", "-1"))

    c.command("HSET", "user:1", "name", "ada", "role", "admin")
    print("HGETALL ->", c.command("HGETALL", "user:1"))

    c.command("SADD", "s1", "a", "b", "c")
    c.command("SADD", "s2", "b", "c", "d")
    print("SINTER  ->", c.command("SINTER", "s1", "s2"))
