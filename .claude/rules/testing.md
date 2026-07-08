---
paths:
  - "**/*.go"
  - "**/go.mod"
  - "**/go.sum"
---
# Go Testing — AnotherMUD

> Extends the global common testing rules with this repo's Go conventions.

## The gate: `-race` is mandatory

The hard pre-merge gate is a clean:

```bash
go build ./...
go test -race ./...
```

**The race detector is not optional** — concurrency (tick loop, event bus,
double-buffered tag index, session manager) is core to the engine, and `-race`
green is the bar. Coverage is encouraged but is a secondary signal; a green
`-race` run plus code review is what actually gates a slice.

## Style

- Standard `go test` with **table-driven tests** and subtests (`t.Run`).
- Arrange-Act-Assert; descriptive test names that state the behavior under test.
- Deterministic tests. RNG-dependent behavior (combat rolls, spawns) must be
  seeded or asserted on invariants, not exact outcomes.

## Integration & live tests

- Live end-to-end tests (e.g. the telnet harness in `internal/telnettest` /
  `cmd/telnet-smoke`) are **env-gated** and do not run in the default suite.
- A few RNG-sensitive live tests are intentionally gated **out of CI** because
  they flake (e.g. the weave-interrupt test). Do not un-gate them without a
  combat-determinism hook — see the deferred-fix notes in memory.

## Coverage (secondary)

```bash
go test -cover ./...
```

## Reference

See skill: `golang-testing` for detailed Go testing patterns and helpers.
