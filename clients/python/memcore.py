"""A minimal, dependency-free Memcore client speaking RESP2 over TCP.

Memcore is wire-compatible with Redis, so this module also works against a Redis
server, and any existing Redis client (redis-py) works against Memcore. Use this
when a zero-dependency client is preferable to a full-featured one.

Example:
    with Memcore(host="127.0.0.1", port=6380) as c:
        c.set("greeting", "hello")
        print(c.get("greeting"))  # "hello"
"""

from __future__ import annotations

import socket
from typing import Optional, Sequence, Union

Arg = Union[str, int, bytes]
Reply = object  # str | int | bytes | None | list, decoded from RESP2


class MemcoreError(Exception):
    """Raised for a server error reply (a RESP '-ERR ...' line).

    This is distinct from transport failures, which raise ConnectionError or
    OSError.
    """


class Memcore:
    """A synchronous connection to a Memcore server.

    Not safe for concurrent use from multiple threads; give each thread its own
    connection.
    """

    def __init__(self, host: str = "127.0.0.1", port: int = 6380, timeout: float = 5.0):
        self._sock = socket.create_connection((host, port), timeout)
        self._sock.setsockopt(socket.IPPROTO_TCP, socket.TCP_NODELAY, 1)
        # A buffered reader handles the line-and-length framing of RESP cleanly.
        self._reader = self._sock.makefile("rb")

    def command(self, *args: Arg) -> Reply:
        """Send one command and return its decoded reply."""
        self._sock.sendall(_encode(args))
        return _read_reply(self._reader)

    def get(self, key: str) -> Optional[str]:
        return self.command("GET", key)

    def set(self, key: str, value: Arg) -> Reply:
        return self.command("SET", key, value)

    def delete(self, *keys: str) -> int:
        return self.command("DEL", *keys)

    def ping(self, message: Optional[str] = None) -> Reply:
        return self.command("PING") if message is None else self.command("PING", message)

    def close(self) -> None:
        try:
            self._reader.close()
        finally:
            self._sock.close()

    def __enter__(self) -> "Memcore":
        return self

    def __exit__(self, *_exc) -> None:
        self.close()


def _encode(args: Sequence[Arg]) -> bytes:
    out = bytearray(b"*%d\r\n" % len(args))
    for arg in args:
        b = arg if isinstance(arg, (bytes, bytearray)) else str(arg).encode()
        out += b"$%d\r\n" % len(b)
        out += b
        out += b"\r\n"
    return bytes(out)


def _read_reply(reader) -> Reply:
    line = reader.readline()
    if not line:
        raise ConnectionError("memcore: connection closed")
    kind, payload = line[:1], line[1:-2]  # strip the trailing CRLF

    if kind == b"+":
        return payload.decode()
    if kind == b"-":
        raise MemcoreError(payload.decode())
    if kind == b":":
        return int(payload)
    if kind == b"$":
        length = int(payload)
        if length == -1:
            return None
        data = reader.read(length)
        reader.read(2)  # discard the CRLF after the body
        return data.decode()
    if kind == b"*":
        count = int(payload)
        if count == -1:
            return None
        return [_read_reply(reader) for _ in range(count)]
    raise MemcoreError("memcore: unknown reply type %r" % kind)
