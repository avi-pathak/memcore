// A minimal, dependency-free Memcore client speaking RESP2 over TCP. Memcore is
// wire-compatible with Redis, so this also works against a Redis server, and any
// existing Redis client (ioredis, node-redis) works against Memcore. Use this
// when a zero-dependency client is preferable to a full-featured one.
//
// The reply parser is incremental: Node delivers socket data in arbitrary
// chunks, so a single reply may arrive across several "data" events, and several
// replies may arrive in one. Pending requests are resolved in the order they
// were sent, which is also what makes pipelining work.

import net from "node:net";

// MemcoreError is thrown for a server error reply (a RESP "-ERR ..." line). It
// is distinct from transport errors, which surface as ordinary Error objects.
export class MemcoreError extends Error {
  constructor(message) {
    super(message);
    this.name = "MemcoreError";
  }
}

// connect opens a connection and resolves once it is established.
export function connect({ host = "127.0.0.1", port = 6380 } = {}) {
  const client = new Client(host, port);
  return client._connect().then(() => client);
}

class Client {
  constructor(host, port) {
    this.host = host;
    this.port = port;
    this.socket = null;
    this._pending = []; // queued {resolve, reject}, one per in-flight request
    this._buffer = Buffer.alloc(0);
  }

  _connect() {
    return new Promise((resolve, reject) => {
      this.socket = net.createConnection({ host: this.host, port: this.port });
      this.socket.setNoDelay(true);
      const onError = (err) => reject(err);
      this.socket.once("error", onError);
      this.socket.once("connect", () => {
        this.socket.removeListener("error", onError);
        this.socket.on("data", (chunk) => this._onData(chunk));
        this.socket.on("error", (err) => this._fail(err));
        this.socket.on("close", () => this._fail(new Error("connection closed")));
        resolve();
      });
    });
  }

  // command sends a command and resolves with its decoded reply. Arguments are
  // coerced to strings; pass a Buffer for binary-safe values.
  command(...args) {
    return new Promise((resolve, reject) => {
      this._pending.push({ resolve, reject });
      this.socket.write(encodeCommand(args));
    });
  }

  get(key) {
    return this.command("GET", key);
  }
  set(key, value) {
    return this.command("SET", key, value);
  }
  del(...keys) {
    return this.command("DEL", ...keys);
  }
  ping(message) {
    return message === undefined ? this.command("PING") : this.command("PING", message);
  }

  close() {
    if (this.socket) this.socket.end();
  }

  _onData(chunk) {
    this._buffer = Buffer.concat([this._buffer, chunk]);
    for (;;) {
      const parsed = parseReply(this._buffer, 0);
      if (parsed === null) break; // a complete reply is not yet buffered
      this._buffer = this._buffer.subarray(parsed.offset);
      const request = this._pending.shift();
      if (!request) continue; // reply with no waiter; ignore defensively
      if (parsed.value instanceof MemcoreError) request.reject(parsed.value);
      else request.resolve(parsed.value);
    }
  }

  _fail(err) {
    while (this._pending.length) this._pending.shift().reject(err);
  }
}

function encodeCommand(args) {
  const parts = [Buffer.from(`*${args.length}\r\n`)];
  for (const arg of args) {
    const buf = Buffer.isBuffer(arg) ? arg : Buffer.from(String(arg));
    parts.push(Buffer.from(`$${buf.length}\r\n`), buf, Buffer.from("\r\n"));
  }
  return Buffer.concat(parts);
}

// parseReply decodes one reply starting at offset. It returns { value, offset }
// where offset is the index just past the reply, or null when the buffer does
// not yet hold a complete reply.
function parseReply(buf, offset) {
  if (offset >= buf.length) return null;
  const crlf = buf.indexOf("\r\n", offset + 1);
  if (crlf === -1) return null;

  const type = buf[offset];
  const line = buf.toString("utf8", offset + 1, crlf);
  const afterLine = crlf + 2;

  switch (type) {
    case 0x2b: // '+' simple string
      return { value: line, offset: afterLine };
    case 0x2d: // '-' error
      return { value: new MemcoreError(line), offset: afterLine };
    case 0x3a: // ':' integer
      return { value: Number(line), offset: afterLine };
    case 0x24: { // '$' bulk string
      const length = Number(line);
      if (length === -1) return { value: null, offset: afterLine };
      const end = afterLine + length;
      if (buf.length < end + 2) return null; // body plus its CRLF not all here
      return { value: buf.toString("utf8", afterLine, end), offset: end + 2 };
    }
    case 0x2a: { // '*' array
      const count = Number(line);
      if (count === -1) return { value: null, offset: afterLine };
      const items = [];
      let cursor = afterLine;
      for (let i = 0; i < count; i++) {
        const element = parseReply(buf, cursor);
        if (element === null) return null;
        items.push(element.value);
        cursor = element.offset;
      }
      return { value: items, offset: cursor };
    }
    default:
      throw new Error(`memcore: unknown reply type ${String.fromCharCode(type)}`);
  }
}
