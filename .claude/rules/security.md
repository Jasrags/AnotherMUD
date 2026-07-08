---
paths:
  - "**/*.go"
  - "**/go.mod"
  - "**/go.sum"
---
# Go Security — AnotherMUD

> Extends the global common security rules with this repo's Go conventions.

## Configuration & secrets

- All tunables are `ANOTHERMUD_*` env vars, loaded from a gitignored `.env` via
  `internal/dotenv` (the real environment always wins). **When you add a new
  env var you MUST add it to `.env.example`** (commented, with its default) and
  tell the user to add it to their own `.env`.
- Never hardcode secrets. Read from the environment and fail fast if a required
  value is missing.

```go
apiKey := os.Getenv("ANOTHERMUD_SOME_KEY")
if apiKey == "" {
    log.Fatal("ANOTHERMUD_SOME_KEY not configured")
}
```

## Persistence path safety

Save I/O goes through `internal/persistence` (atomic tmp→bak→rename **with path
safety**). Do not build save paths by string-concatenating untrusted input
(player/character names) — route through the store so traversal is prevented.

## Account credentials

- Passwords are stored with **bcrypt** (`internal/account`). Never log, echo, or
  persist a plaintext password; login authenticates by username.

## Lua scripting sandbox

Pack scripts run in a **sandboxed** gopher-lua `LState`: base/table/string/math
only — **no `os`, `io`, `debug`, or `package`**. Do not widen the sandbox to
expose the filesystem, network, or process. New engine bridges go through the
vetted `engine.*` API (`subscribe`/`log`/`schedule`), not raw Lua stdlib.

## Context & timeouts

Use `context.Context` for cancellation/timeouts on I/O and network paths (F1).

```go
ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
defer cancel()
```

## Static analysis (optional)

```bash
gosec ./...
```
