# Personal Aliases — Feature Specification

> **Spec ahead of code — build pending.** This is a behavior contract for
> server-side, per-character command aliases that synchronize to every client
> over GMCP. It resolves the fork recorded in
> [`../clients/parity-matrix.md`](../clients/parity-matrix.md): power-user input
> features live **server-side + GMCP** so the same alias behaves identically in
> raw telnet, Mudlet, and the web client, and roams with the character.

## 1. Overview

### Concept

A **personal alias** is a player-defined shorthand: a name that, when it appears
as the verb of an input segment, is replaced by a stored **template** before the
command is parsed and dispatched. `alias k kill $1` lets the player type `k rat`
and have the server run `kill rat`. Aliases belong to the **character**, are
saved with the character, and are pushed to whatever client the character is
using — so they work the same everywhere and follow the player across sessions
and clients.

Expansion is performed **entirely server-side, before dispatch.** The client
never expands an alias; it only *displays* the alias set and offers edit
affordances that reduce to the ordinary management verbs (§5). This is the same
authority model tab-completion uses for `Input.Complete`
(`tab-completion §13`) and the web-client contract requires
(`web-client-plan §2.3`): the client is a view, the server owns behavior.

### 1.1 Distinction from registry aliases

`commands-and-dispatch §2.3` already uses the word "alias" for a **command
synonym** baked into a command's registration (`i` → `inventory`, `eq` →
`equipment`). Those are author-defined, global, and immutable at runtime.

**Personal aliases are a different thing entirely** and this spec never
overloads the term:

| | Registry alias (§2.3) | Personal alias (this spec) |
|---|---|---|
| Defined by | content/command author | the player, at runtime |
| Scope | global, all characters | one character |
| Storage | command registration | player save + GMCP |
| Substitution | none (pure synonym) | template with `$` parameters |
| Resolution stage | inside `Resolve` | a pre-parse stage *before* `Resolve` |

### 1.2 The roaming rationale (resolved decision)

The parity fork asked whether aliases live client-side (reimplemented per
surface, configs never sync) or server-side + GMCP (one behavior, roams). It was
resolved **server-side + GMCP**. The division of labor this spec implements:

- The **server owns** the alias table, the substitution grammar, and expansion.
- The **client owns** only presentation — showing the alias list and providing
  an edit UI whose submits are plain `alias` / `unalias` commands.

### Goals

- Player-defined, per-character command aliases with positional and
  whole-tail argument substitution.
- Expansion that composes cleanly with existing input parsing — chaining,
  repeat expansion, and tokenization (`commands-and-dispatch §4`).
- Deterministic, loop-safe expansion with a bounded recursion depth.
- Durable persistence with the character; append-only save migration.
- Live synchronization to every GMCP-capable client via a `Char.Aliases`
  package, with client edits reduced to the management verbs.
- Preservation of the **authority invariant**: after expansion the server only
  ever dispatches ordinary commands the actor could have typed by hand.

### Non-goals

- **Client-side macros / key bindings.** Binding a keyboard key or on-screen
  button to a command is a *client* concern (the keyboard lives in the client).
  The server contributes nothing beyond letting the bound target *be* an alias.
  Cross-referenced, not specced here.
- **Triggers / highlights.** Reacting to server output with regex is
  render-side and stays per-client. If trigger *pattern storage* is later made
  to roam, it is a **sibling spec** following this same server-side + GMCP
  division — out of scope here.
- **Global or account-wide aliases.** Aliases are per-character in v1
  (see open questions).
- **Scripting.** Aliases expand to text; they are not a scripting surface. Lua
  lives in `scripting-and-packs`.

---

## 2. The expansion stage

Personal-alias expansion is a new **pre-parse stage** in the input pipeline. It
runs on the raw line *before* the parser of `commands-and-dispatch §4` does its
work, and its output re-enters that same parser unchanged.

### 2.1 Placement in the pipeline

For each input line submitted by a player actor:

1. **Chain split.** The line is split on the chain separator (`commands-and-
   dispatch §4.1`), yielding ordered segments.
2. **Alias expansion (this spec).** For each segment, the **first token** is
   looked up in the character's alias table by **exact match** (§4). On a hit,
   the segment is replaced by the alias template with arguments substituted
   (§3). The replacement text MAY itself contain the chain separator and MAY
   itself begin with an alias name; it is re-fed to steps 1–2 (bounded by the
   depth cap, §2.3).
3. **Repeat expansion + tokenization.** The fully expanded segments pass through
   repeat expansion (`§4.2`) and tokenization (`§4.3`) as they do today.
4. **Dispatch.** The resulting `(verb, args)` commands dispatch in order,
   exactly as an un-aliased line would.

Alias expansion is **lexical and pre-registry**, mirroring how repeat expansion
does not consult the registry (`§4.2`). The expander does not know or care
whether the alias template names real verbs; if it expands to nonsense, dispatch
reports the ordinary "unknown command" for the resulting verb.

### 2.2 Mob and non-player actors

Alias expansion applies to the **player input path only**. Mob command sources
(`commands-and-dispatch §3.2`) have no alias table and skip the stage entirely.

### 2.3 Recursion, depth, and loop safety

An alias template may name another alias (`k` → `kill $1`, `bb` → `k $1;
say die`). Expansion is therefore recursive and MUST be bounded:

- A configured **expansion depth cap** bounds how many times a single input line
  may re-enter the expansion stage. On reaching the cap, expansion stops and the
  partially expanded text is dispatched as-is (no error spam, no infinite loop).
- The total number of produced commands remains bounded by the existing **chain
  length cap** (`§4.1`); alias expansion can never produce more commands than
  that cap, and trailing overflow is dropped silently as it is today.
- A direct self-reference (`x` → `x`) or a cycle (`a`→`b`, `b`→`a`) terminates
  at the depth cap rather than looping.

### 2.4 The bypass prefix

A configured **bypass prefix** character, when it is the first character of a
segment, suppresses alias expansion for that one segment and is then stripped.
This lets a player who has aliased `n` to something else still issue the literal
`n` command, and guarantees the management verbs remain reachable even if
shadowed (§4.2).

### Acceptance criteria

- [ ] Alias expansion runs after chain split and before repeat
      expansion/tokenization.
- [ ] Only the first token of a segment is treated as an alias name.
- [ ] An alias template containing the chain separator produces multiple
      commands, subject to the chain cap.
- [ ] A template whose first token is itself an alias expands recursively.
- [ ] Expansion depth is capped; a self-referential or cyclic alias terminates
      and does not hang or overflow.
- [ ] Total produced commands never exceed the chain cap; overflow is dropped
      silently.
- [ ] Mob/non-player command sources skip alias expansion.
- [ ] A segment beginning with the bypass prefix is dispatched literally with
      the prefix stripped and no alias expansion.
- [ ] The commands dispatched after expansion are indistinguishable from the
      same commands typed by hand (authority invariant).

---

## 3. Substitution grammar

A template is stored verbatim and expanded against the **arguments** of the
invoking segment — the whitespace-delimited tokens *after* the alias name.

The substitution tokens are an **interoperability contract** (clients display
and round-trip them), so they are fixed, not configurable:

| Token | Expands to |
|---|---|
| `$1` … `$9` | the Nth argument, or empty string if absent |
| `$*` | all arguments, space-joined, or empty if none |
| `$0` | the alias name as invoked |
| `$$` | a literal `$` |

Rules:

- Substitution is a single left-to-right pass over the template; expanded
  argument text is **not** re-scanned for further `$` tokens (an argument that
  contains `$1` is inserted literally).
- A `$` not forming one of the tokens above is emitted literally.
- **Unconsumed trailing arguments.** If a template references no `$*` and fewer
  positional parameters than the invocation supplied, the leftover arguments are
  **appended to the last produced command**, space-separated. This makes the
  common no-parameter alias behave intuitively: `alias gc get coins` then
  `gc from corpse` runs `get coins from corpse`. (An alias that references any
  parameter or `$*` is assumed to place its arguments deliberately and receives
  no auto-append.)

### Acceptance criteria

- [ ] `$1`..`$9` substitute the corresponding argument; missing ones are empty.
- [ ] `$*` substitutes all arguments space-joined; empty when there are none.
- [ ] `$0` substitutes the invoked alias name.
- [ ] `$$` yields a literal `$`; a lone `$x` (x not in the token set) is literal.
- [ ] Substituted argument text is not re-scanned for `$` tokens.
- [ ] An alias with no parameter tokens appends leftover args to the last
      produced command.
- [ ] An alias that uses any parameter or `$*` does not auto-append leftovers.

---

## 4. Precedence and shadowing

### 4.1 Alias-before-verb

Because expansion happens before `Resolve` (§2.1), a personal alias whose name
**exactly** equals a real verb or registry alias **shadows** it for that
character. `alias n say no` makes `n` stop meaning `north` for that player. This
is intentional and matches player expectation from other clients; it is the
player's own table, changing only their own input.

Shadowing is **exact-match only.** Alias lookup never does prefix matching, so
defining `alias no ...` does not intercept `north`; only an alias literally named
`north` (or `n`) shadows those.

### 4.2 Protected reachability

The alias **management verbs** (`alias`, `unalias`, and the list form, §5) MUST
remain reachable even when a player has shadowed them, so a player can never
brick their own input. Two mechanisms guarantee this together:

- The bypass prefix (§2.4) always issues a literal command.
- Management-verb resolution MAY additionally be made non-shadowable (an alias
  named `unalias` still lists/defines, so recovery never depends on the player
  remembering the bypass character). Which mechanism is authoritative is a
  configuration/policy choice (see config surface + open questions).

### Acceptance criteria

- [ ] An alias whose name equals a verb shadows that verb for its character only.
- [ ] Shadowing is exact-match; it never intercepts prefix completions of other
      verbs.
- [ ] After shadowing a core verb, the player can still reach the literal verb
      via the bypass prefix.
- [ ] The management verbs remain usable to inspect and remove aliases even when
      an alias shares their name.

---

## 5. Management verbs

Three operations, exposed through the ordinary command registry so telnet,
Mudlet, and web all reach them identically:

- **Define / replace** — `alias <name> <template...>`. Stores `name → template`,
  replacing any existing entry for `name`. The template is the remainder of the
  line verbatim (no tokenization; `commands-and-dispatch §4.3` free-text rule).
- **List** — `alias` (no args) or `aliases`. Renders the character's alias table
  (name → template), stably ordered (§8), or a "no aliases defined" line.
- **Remove** — `unalias <name>`. Deletes the entry; reports whether one existed.

### 5.1 Validation

Definitions are validated at the boundary (`coding-style`: validate at system
boundaries); a rejected definition changes nothing and returns a clear message:

- **Name charset.** A name is a single token: no whitespace and no chain
  separator. A name MUST NOT begin with a decimal digit (that space belongs to
  repeat expansion, `§4.2`, and a digit-led alias would be unreachable). Further
  charset narrowing is a config policy.
- **Template non-empty.** An empty template is rejected; use `unalias` to remove.
- **Per-character count cap.** A configured maximum number of aliases per
  character bounds save size and GMCP payload; defining past the cap is rejected
  with a message naming the limit.
- **Template length cap.** A configured maximum template length bounds expansion
  cost and payload.

Validation deliberately does **not** check that the template names real verbs —
templates may reference other aliases not yet defined, and the registry is the
authority on verbs at dispatch time, not at definition time.

### Acceptance criteria

- [ ] `alias k kill $1` stores the entry and confirms.
- [ ] `alias k ...` again replaces the prior template.
- [ ] `alias` / `aliases` lists the table stably; empty table renders a clear
      "none" line.
- [ ] `unalias k` removes it and reports; `unalias nope` reports no such alias.
- [ ] A name with whitespace, a chain separator, or a leading digit is rejected.
- [ ] An empty template is rejected.
- [ ] Defining past the per-character count cap is rejected with the limit named.
- [ ] A template past the length cap is rejected.
- [ ] A valid definition/removal marks the character dirty for autosave (§6) and
      emits the GMCP update (§7).

---

## 6. Persistence

Aliases are part of the character and persist with the player save, following
the same durability contract as other per-character state (`recall §6`,
`persistence`).

- The alias table is written into the player save as a name → template map.
- The save is **versioned with an append-only migration**: adding aliases bumps
  the current player-save version and adds one migration that defaults absent
  aliases to an empty table. No existing migration is edited
  (`CLAUDE.md`: migration chain is append-only).
- A defining or removing operation flips the character's dirty bit so the
  autosave tick and shutdown flush commit it (`session` autosave contract).
- Load is tolerant: a save with no alias field loads as an empty table.

### Acceptance criteria

- [ ] Defined aliases survive logout/login and server restart.
- [ ] A pre-alias save (older version) loads with an empty table and no error.
- [ ] Defining/removing marks the character dirty; the change is present after
      the next autosave/flush.
- [ ] The migration is additive; older-version saves migrate forward without
      touching unrelated fields.

---

## 7. GMCP surface — `Char.Aliases`

Aliases synchronize to GMCP-capable clients so Mudlet and the web client render
the same table and can offer edit UI. The package is **additive and ignorable**
(`web-client-plan §2.1`): clients that do not consume it are unaffected.

### 7.1 Emission

- **Full set on login** — the character's complete alias table is emitted once
  the character is playing (same trigger family as `Char.Login`/`Char.Status`,
  `session` GMCP flushers).
- **On change** — every successful define/replace/remove re-emits the full set.
  A full replace (rather than a delta) keeps the client's model trivially
  correct; the table is small (bounded by the count cap, §5.1).

### 7.2 Payload shape

An ordered list of entries, each carrying the **name** and the **template**
verbatim (including its `$` tokens, so a client edit UI can show and round-trip
them). The concrete field names are fixed when the package ships and recorded in
`networking-protocols` and `parity-matrix.md`'s authoritative surface list.

### 7.3 Client → server edits reduce to verbs (authority invariant)

A client MUST NOT mutate alias state directly. An edit affordance submits an
ordinary `alias` / `unalias` command over the existing command envelope; the
server validates (§5.1), persists (§6), and re-emits (§7.1). There is no
alias-specific write channel. This is the same reduction tab-completion and
every rich web package follow (`web-client-plan §2.3`).

### Acceptance criteria

- [ ] The full alias table is emitted when the character starts playing.
- [ ] Each successful define/replace/remove re-emits the full table.
- [ ] The payload carries name + verbatim template (parameters intact).
- [ ] No inbound alias-specific mutation package exists; client edits arrive as
      `alias`/`unalias` commands and flow through §5 validation.
- [ ] A client that ignores `Char.Aliases` sees no behavior change.

---

## 8. Observability

- Define/replace/remove log at an appropriate level with the character and alias
  name as structured fields (`ROADMAP` foundations F2), never the raw combat
  path — this is input configuration, not hot-loop.
- Listing order is **stable** (e.g. lexical by name) so successive lists and
  successive GMCP emissions do not reorder spuriously and diff cleanly.

### Acceptance criteria

- [ ] Define/remove emit a structured log line with character + alias name.
- [ ] List output and GMCP payload order are stable across calls with an
      unchanged table.

---

## 9. Configuration surface

| Setting | Meaning | Notes |
|---|---|---|
| Max aliases per character | Upper bound on table size | Rejects past the cap (§5.1); bounds save + GMCP payload |
| Max template length | Upper bound on a single template | Bounds expansion cost + payload (§5.1) |
| Expansion depth cap | Max times a line re-enters the expansion stage | Loop/recursion safety (§2.3) |
| Chain length cap | Max produced commands per line | **Reused** from `commands-and-dispatch §4.1`, not new |
| Bypass prefix character | Leading char that suppresses expansion for a segment | §2.4; default recorded when shipped |
| Management-verb protection policy | Whether management verbs are non-shadowable in addition to the bypass escape | §4.2 |
| Name charset policy | Any narrowing beyond "no space / no separator / no leading digit" | §5.1 |
| `Char.Aliases` package name | GMCP package identifier | §7; fixed when shipped |

All numeric values live here, never in prose (`spec conventions`).

---

## 10. Open questions

- **Per-character vs account-wide.** v1 scopes aliases to the character. Many
  players want one alias set across all their characters (account-level). That
  is a larger persistence surface (account save + which-scope-wins precedence)
  and is deferred; the per-character table is the subset that must exist either
  way.
- **Shadowing policy.** §4.1 lets aliases shadow core verbs (Mudlet-like power).
  A more conservative policy would forbid shadowing a protected verb set
  outright rather than relying on the bypass escape. Left as a config/policy
  lever (§9) until real play shows which players expect.
- **Nested-alias argument binding.** When alias `a` expands to `b $1` and `b` is
  itself an alias, `$1` is bound at `a`'s invocation and the result is re-parsed;
  `b`'s own parameters then bind against `a`'s already-substituted tokens. The
  left-to-right, no-re-scan rule (§3) makes this deterministic but occasionally
  surprising for deeply nested tables; documented rather than "solved."
- **Speedwalk as an expansion sibling.** Compound movement runs (`4n2e3s` in one
  token) are the same *family* as aliases (a lexical pre-dispatch expansion) and
  were named in the resolved parity decision, but they are a **distinct feature**
  (movement-letter grammar, ambiguity between a direction letter and a verb
  prefix). They are intentionally **not** folded in here; when built they get
  their own short spec or a `world-rooms-movement` section, reusing this spec's
  pre-parse-stage placement and chain-cap bound.
- **Argument defaulting.** No `${1:-default}`-style defaulting in v1; a missing
  positional is empty. Add only if play demands it.

## Cross-references

- `commands-and-dispatch` — input parsing (§4 chaining/repeat/tokenization),
  resolution + registry aliases (§2.3), the dispatch path expansion feeds.
- `tab-completion §13` — the sibling input-side GMCP surface and its
  client-reduces-to-verbs authority model.
- `web-client-plan §2.1–2.3` — additive-package convention + the authority
  invariant for client→server intents.
- `../clients/parity-matrix.md` — the resolved server-side + GMCP decision this
  spec implements, and the per-surface parity tracking.
- `recall §6`, `persistence` — the per-character save + migration contract.
- `networking-protocols` — where the `Char.Aliases` package shape is recorded
  when it ships.
