# Contributing

Contributions are welcome. The bar is ordinary good Go: clear names, small
interfaces defined where they are used, errors as values, and tests that read as
behavior.

## The one rule

Respect the layer boundaries. This is the rule reviewers enforce above all
others, because it is what keeps the engine testable and the packages reusable:

- The storage core (`value`, `keyspace`, `shard`) must not import networking,
  protocol, or persistence packages. Dependencies point inward.
- Time comes from the injected `Clock`. Do not call `time.Now` outside
  `SystemClock`.
- RESP error strings are produced only in the `server` package. Lower layers
  return Go errors and sentinels.

A change that crosses a boundary to take a shortcut will be sent back, even if it
works. See [ARCHITECTURE.md](ARCHITECTURE.md) for the full picture.

## Before you open a pull request

Run the same checks CI runs:

```
gofmt -l .
go vet ./...
staticcheck ./...
go test -race ./...
```

All four must be clean. New behavior needs a test; the domain packages
(`value`, `keyspace`, `command`) are expected to stay at high coverage, and the
RESP reader changes should keep the fuzz target passing.

## Style

- Name tests as behavior sentences, not `TestFoo1`.
- Comments explain why, not what. A long comment-free stretch is fine.
- Do not introduce an interface until a second real implementation justifies it.
- No new dependency without a reason that outweighs the cost.

## Commits

Imperative mood, terse, no decorative prefixes. A commit should build and pass
its tests on its own.
