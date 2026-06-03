# Tab-completion over GMCP (`Input.Complete`)

Client integration guide for the tab-completion **Phase 1** GMCP surface. The
**wire protocol below is authoritative** (it's what the server implements,
`docs/specs/tab-completion.md §13`). The Mudlet snippet is a **starting point** —
the GMCP/keybinding calls vary by Mudlet version, so verify those against yours.

## Wire protocol

Request/response. The client initiates on Tab; the server replies once. No
push, no state change — safe to send on every keystroke if you like.

### Request — `Input.Complete` (client → server)

```json
{ "line": "get sw" }
```

`line` is the partial command line **up to the cursor**. (The server tokenizes
it the same way it dispatches a submitted line; a trailing space means "complete
a fresh next token".)

### Response — `Input.Complete.List` (server → client)

```json
{
  "line": "get sw",
  "target": "argument",
  "verb": "get",
  "common": "sword",
  "truncated": false,
  "candidates": [
    { "value": "sword", "display": "a short sword", "kind": "item" }
  ]
}
```

| Field | Meaning |
|---|---|
| `line` | the request line, echoed — match the reply to the request you sent |
| `target` | `"verb"`, `"argument"`, or `"none"` (no completable slot) |
| `verb` | resolved verb when `target == "argument"` (omitted otherwise) |
| `common` | longest common prefix of the candidate `value`s — **complete the token to this**, then show the list (the decided LCP behavior) |
| `truncated` | the candidate set was capped; show "…and more" |
| `candidates` | ordered list; `value` is the token to insert (round-trips through resolution), `display` is the label, `kind` ∈ `verb`/`item`/`entity`/`door`/`bulk` |

### Client algorithm (on Tab)

1. Send `Input.Complete` with the current input line.
2. On `Input.Complete.List` for that `line`:
   - If `candidates` is empty → beep / do nothing.
   - If one candidate → replace the token under the cursor with `value`.
   - If many → replace the token with `common` (if it extends what's typed), then
     display the `candidates` (e.g. in a popup or echoed list).

## Mudlet starter

> ⚠️ The GMCP send call, JSON encoding, and "read the current input line" API
> differ across Mudlet versions — treat these as placeholders to adapt.

**1. Advertise support** (so the server counts you as a completion-capable client
per §12; optional for request/response since the request itself proves support):

```lua
-- run once on connect
sendGMCP([[Core.Supports.Add ["Input 1"]]])
```

**2. A key binding on `Tab`** that sends the current input line as a request:

```lua
-- Mudlet: bind this to the Tab key.
-- getCurrentCommandLine() is a placeholder — use your Mudlet version's
-- API for the unsent input buffer (and track the cursor if you want
-- mid-line completion). Falls back to the whole line.
local line = getCurrentCommandLine and getCurrentCommandLine() or ""
-- Send the request. yajl ships with Mudlet for JSON encoding; some
-- versions also accept sendGMCP(name, table).
sendGMCP("Input.Complete " .. yajl.to_string({ line = line }))
```

**3. A GMCP event handler** for the reply. Register a script on the event
`gmcp.Input.Complete.List`; Mudlet decodes the payload into the table
`gmcp.Input.Complete.List`:

```lua
local r = gmcp.Input.Complete.List
if not r or #r.candidates == 0 then return end

if #r.candidates == 1 then
  -- complete to the single value
  replaceLastToken(r.candidates[1].value)          -- your input-edit helper
elseif r.common ~= "" then
  replaceLastToken(r.common)                        -- complete to the shared prefix
end

-- show the options (one match needs no list)
if #r.candidates > 1 then
  local names = {}
  for _, c in ipairs(r.candidates) do
    names[#names+1] = c.value .. (c.display ~= c.value and (" ("..c.display..")") or "")
  end
  cecho("\n<grey>" .. table.concat(names, "  ") .. (r.truncated and "  …" or "") .. "\n")
end
```

`replaceLastToken(s)` is your own helper that swaps the token under the cursor in
the command line (Mudlet: `printCommandLine` / `clearCommandLine` + re-set).

## Testing without Mudlet

Any WebSocket client works (the server's WS transport speaks the same packages as
JSON envelopes). Send `{"type":"gmcp","package":"Input.Complete","data":{"line":"get s"}}`
and read the `{"type":"gmcp","package":"Input.Complete.List", ...}` reply. The
line-mode `suggest <partial>` verb exercises the identical query if you just want
to eyeball candidates on raw telnet.
