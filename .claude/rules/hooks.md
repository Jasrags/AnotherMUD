---
paths:
  - "**/*.go"
  - "**/go.mod"
  - "**/go.sum"
---
# Go Hooks — AnotherMUD

> Extends the global common hooks rules with this repo's Go conventions.

## PostToolUse (after editing `.go` files)

Configure in `~/.claude/settings.json`:

- **gofmt / goimports** — auto-format edited `.go` files.
- **go vet** — static analysis on the edited package.
- **staticcheck** — extended static checks (if installed).

## Stop (session-end verification)

Mirror the pre-merge gate so a session can't end on a broken tree:

```bash
go build ./...
go test -race ./...
```

`-race` is the mandatory gate for this engine (see `testing.md`).

## Git note (not a hook, but binding)

Work **directly on `main`** — **no feature branches** for this repo. Per-slice
commits on `main` are the rhythm. Commit format: `<type>: <description>`
(feat/fix/refactor/docs/test/chore/perf/ci). This intentionally overrides any
default "branch off main first" convention.
