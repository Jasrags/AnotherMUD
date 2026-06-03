# Tab-Completion — Phase 0: Enumeration Substrate

> **Scope.** This spec covers **Phase 0** of input tab-completion: the
> transport-agnostic *completion query* (given a partial input line and an
> actor, produce the ordered set of candidates for the token under
> completion) and the role-gated `complete` debug verb that exercises it.
> The *surfaces* that turn this into live TAB behavior — GMCP request/response
> for scriptable clients (Phase 1) and server-side character-mode line editing
> for raw telnet (Phase 2) — are **out of scope here** and tracked in
> `docs/proposals/tab-completion.md`. This spec is the substrate all of them sit
> on.

## 1. Overview

### Concept

Completion answers one question: *"For the token the player is currently
typing, what are the valid things they could mean, given who they are and
where they stand?"* The answer is computed entirely server-side — only the
server knows the verb set and the resolver rules — and is returned as an
ordered list of **candidates**, each carrying a human-facing **display label**
and a **completion token** that, if substituted for the partial token and
submitted, resolves to exactly the thing the candidate names.

The substrate reuses, rather than reinvents, three existing contracts:

- the **command registry** for the set of verbs and the prefix-priority order
  already used to route them (`commands-and-dispatch §2.3`);
- each command's **declared typed arguments** (`commands-and-dispatch §5`),
  which already say what kind of thing belongs in each argument slot and carry
  the per-slot flags (bulk, prepositions, visibility bypass);
- the **keyword match rules** (`inventory-equipment-items §6`) used to turn a
  token into an entity, run here in reverse to filter and to choose
  disambiguating tokens.

### The load-bearing invariant

**Completion is a view over resolution, never a parallel matcher.** Every
completion token a candidate offers MUST, when submitted as input, resolve
through the ordinary dispatch + argument-resolution path to the same entity the
candidate named — under the same keyword/ordinal/visibility rules. A
completion that suggests something the resolver would then reject, or that
resolves to a *different* entity, is a defect. This invariant is what lets the
feature lean on the resolver instead of duplicating it, and it is the lens for
every acceptance criterion below.

### Goals

- Enumerate **verb** candidates from the registry for a partial first token.
- Enumerate **argument** candidates for a partial later token, using the
  declared argument type of that slot to pick the scope and the keyword rules
  to filter it.
- Disambiguate multiple matches into individually-addressable completion
  tokens (`§6`), meshing with the existing ordinal / `all.` grammar rather than
  fighting it.
- Degrade safely: unknown verbs, un-migrated (hand-parsed) verbs, empty scopes,
  and absent services all produce a well-defined empty-or-partial result, never
  an error to the player.
- Be observable before any client surface exists, via the `complete` debug verb
  (`§9`).

### Non-goals (Phase 0)

- **No surface.** No TAB key handling, no GMCP package, no character-mode
  negotiation, no line rewriting. The query is a pure function over state; how a
  client invokes it and renders the result is Phase 1/2.
- **No presentation policy.** Cycle-on-repeat vs. longest-common-prefix-then-list
  is a surface decision (`proposal §7`); Phase 0 returns an ordered list and a
  truncation flag and stops there.
- **No fuzzy / typo correction, no ghost-text, no free-text-body completion**
  (say/tell message bodies). Deferred per the proposal.
- **No new matching semantics.** If a token can't already be resolved by the
  rules in `inventory-equipment-items §6`, completion does not invent a way to
  match it.

## 2. The completion query

The query takes a **partial line** (the raw input the player has typed so far,
not yet submitted) and the **actor**, and returns:

- an ordered list of **candidates** (`§3`–`§6`), possibly empty;
- a **truncated** flag, set when the candidate set was capped (`§7`);
- the **completion target** — an indicator of whether the token under
  completion is the verb slot or a specific argument slot (useful to a surface
  for labelling, and to the debug verb for output).

The token **under completion** is determined by tokenizing the partial line on
whitespace (the same field-splitting dispatch uses) and applying the
trailing-space rule:

- **No trailing whitespace** → the player is still typing the **last token**;
  that token is the partial being completed, and the candidate set is filtered
  by it as a prefix.
- **Trailing whitespace** → the player has finished the previous token and is
  beginning a **new, empty token** at the next position; the partial is the
  empty string and the candidate set is the unfiltered scope for that position
  (subject to the cap).

The **slot** is then:

- **Verb slot** — the token under completion is the first token (index 0).
- **Argument slot N** — the token under completion is at index ≥ 1; N counts
  argument positions after the verb.

### Acceptance criteria

- [ ] `look` (no trailing space) reports target = verb slot, partial = `look`.
- [ ] `loo` reports target = verb slot, partial = `loo`.
- [ ] `get ` (trailing space) reports target = argument slot 0 of `get`, partial = empty.
- [ ] `get sw` reports target = argument slot 0 of `get`, partial = `sw`.
- [ ] `put gem in ch` reports target = the argument slot the `ch` token falls in
      after preposition handling (`§5`), partial = `ch`.
- [ ] An empty or whitespace-only partial line reports verb slot with an empty
      partial (the candidate set is "all verbs", subject to the cap).
- [ ] Tokenization matches dispatch's field-splitting exactly (same whitespace
      handling), so the slot the query computes is the slot dispatch would
      resolve the submitted line into.

## 3. Verb enumeration

For the verb slot, candidates are the registered command keywords whose name
has the partial as a prefix, ordered by the **same priority dispatch uses to
route** (`commands-and-dispatch §2.3`): an exact match first, then ascending
registration order. This makes the first candidate the verb that *would
actually run* if the player submitted the partial as-is — completion never
disagrees with dispatch about which verb wins.

- Both primary keywords and aliases are routable input, so both are eligible
  verb candidates. (Whether a surface dedups an alias against its primary —
  e.g. hiding `n` when `north` is shown — is a presentation choice deferred to
  Phase 1; the substrate returns both, each tagged so the surface can decide.)
- A verb's **completion token** is its keyword.
- A verb's **display label** is its one-line brief when the command carries
  listing metadata (`commands-and-dispatch §8`), else just the keyword.

### Admin / hidden verbs

Admin-marked commands are refused at dispatch for actors who lack the configured
admin role, with the *identical* `Huh?` an unknown verb produces, so the verb is
not discoverable (`admin-verbs §2`). **Completion MUST honor the same gate:** an
admin verb is a candidate **only** for an actor who holds the admin role. For
everyone else it is invisible to completion exactly as it is invisible to help
and to dispatch — surfacing it would leak the verb's existence and violate the
`§1` invariant (it would suggest a token dispatch then answers `Huh?` to).

### Acceptance criteria

- [ ] Partial `loo` returns `look` as a candidate; partial `xyzzy` (no prefix
      match) returns no verb candidates.
- [ ] When two registered keywords share the partial as a prefix, they are
      ordered by registration priority, and the candidate dispatch would route
      to is first.
- [ ] An exact keyword match sorts ahead of longer prefix matches (parity with
      `§2.3` resolution: `n` ahead of `north` for partial `n`).
- [ ] An admin verb appears as a candidate for an actor holding the admin role
      and does **not** appear for an actor who lacks it.
- [ ] A verb registered without listing metadata (routable but not in the help
      listing) is still offered as a verb candidate — muscle-memory parity with
      dispatch.
- [ ] Every returned verb completion token, submitted alone, routes to a real
      handler (no candidate resolves to `Huh?`).

## 4. Argument enumeration

For an argument slot, the substrate first finds the command the verb resolves to
(by the same routing as `§3`), then reads that command's **declared argument at
the slot's position** (`commands-and-dispatch §5.1`). The declared argument's
**type** selects the scope to enumerate; the partial filters it.

### Scope by argument type

The argument type names below are the `commands-and-dispatch §5.2` set. Each maps
to a candidate scope drawn from the actor's resolve context (the same context the
real resolver consults — actor inventory, non-actor room items, room
players + mobs excluding self, and the room's doors):

| Argument type | Candidate scope |
|---|---|
| `inventory` | items the actor is carrying |
| `room_item` | non-actor items in the current room |
| `container` | carried + room items that are containers |
| `entity` | players + mobs in the room (excluding self) |
| `player` | players in the room (excluding self) |
| `npc` | mobs in the room |
| `visible` | self + carried items + room items + room entities |
| `findable` | the findable scope as the resolver defines it |
| `door` | doors reachable from the current room (by direction or keyword) |
| `keyword` | no scope — free keyword; **no candidates** (nothing to enumerate) |
| `text` | free text remainder; **no candidates** |
| `number` | numeric; **no candidates** |

### Filtering

Within the chosen scope, candidates are the entities matching the partial under
the **same rules the resolver applies** (`inventory-equipment-items §6`): exact
keyword, strict-prefix keyword, or name substring. An **empty partial** (the
trailing-space case) matches the **entire scope**. Filtering reuses the existing
"return all matches" rule (`§6.2`), so the candidate set is exactly the set the
resolver would consider for that token — never broader, never narrower.

### Bulk slots

When the declared argument permits bulk selection (`§5.5`, the `all` /
`all.<keyword>` grammar), the substrate MAY additionally offer the bulk tokens
(`all`, and `all.<keyword>` for a keyword shared by ≥ 2 matched entities) as
candidates, tagged as bulk. A non-bulk slot never offers bulk tokens.

### Acceptance criteria

- [ ] `get sw` in a room containing "a sword" returns the sword as a candidate;
      `get xq` returns none.
- [ ] `drop ` (empty partial, bulk-capable inventory slot) returns every carried
      item, plus the `all` bulk token, subject to the cap.
- [ ] `kill ban` with a "grizzled bandit" mob present returns it (matched on the
      `bandit` keyword via prefix), and the completion token resolves back to
      that mob (`§1`, `§6`).
- [ ] A `player`-typed slot does not return mobs; an `npc`-typed slot does not
      return players.
- [ ] A `door`-typed slot enumerates the room's doors by direction and keyword.
- [ ] A `keyword` / `text` / `number` slot returns no candidates (there is
      nothing to enumerate), and the query still reports the correct target slot.
- [ ] The matched set for a partial equals the set the real resolver would
      consider for that same token — verified by resolving each returned
      completion token and confirming membership.

## 5. Slot mapping with prepositions

A declared argument may carry **prepositions** the resolver silently consumes
when they appear immediately before the argument's token (`commands-and-dispatch
§5.1`, e.g. `put <gem> in <chest>`). Completion MUST map the token under
completion to an argument slot using the **same preposition-skipping walk** the
argument driver performs (`§5.4`), so the scope it enumerates is the scope the
resolver would use for that token.

Concretely: walking the declared arguments left to right against the typed
tokens, a token that matches the next argument's preposition is consumed as the
preposition (not as the argument), and the following token is the argument. The
slot the cursor token lands in after this walk is the slot whose type selects
the scope.

### Acceptance criteria

- [ ] `put gem in ch` maps `ch` to the container slot (the `in` preposition is
      consumed), so the scope enumerated is containers, not the first item slot.
- [ ] `put gem ch` (preposition omitted, which the resolver tolerates) still maps
      `ch` to the container slot.
- [ ] A trailing-space partial immediately after a preposition (`put gem in `)
      maps the new empty token to the post-preposition argument slot and returns
      that scope unfiltered.
- [ ] When the typed tokens have already satisfied every declared argument, a
      further token under completion has **no slot** and returns no candidates
      (parity: the resolver would treat it as trailing/unconsumed input).

## 6. Candidate identity & disambiguation

When the matched set for an argument partial contains more than one entity, each
candidate still needs a completion token that resolves to **it specifically**.
The rule (decided for Phase 0):

**Distinguishing name where one exists; ordinal fallback for true duplicates.**

1. **Distinguishable by keyword.** If a matched entity has a keyword that
   uniquely resolves to it within the current scope (no other matched entity
   shares it as an exact/prefix match), that keyword is its completion token, and
   its display label is its full name (so the player sees *what* they're
   completing — "a gold ring" vs "a silver ring" — while the inserted token stays
   a single word that resolves cleanly).
2. **True duplicates.** Entities the scope cannot tell apart (same keywords and
   same name) are assigned **ordinal** completion tokens in scope order: the
   first takes the bare keyword, the rest take `2.<keyword>`, `3.<keyword>`, …
   — the exact selectors `inventory-equipment-items §6.1` already understands.

This honors the `§1` invariant by construction: a distinguishing keyword
resolves (exact/prefix) to its one entity; an ordinal selector resolves to the
Nth match in the same scope order the resolver iterates. The display label is
free to be the full human name; only the **completion token** must round-trip.

### Acceptance criteria

- [ ] Two items "a gold ring" and "a silver ring" (distinct keywords `gold` /
      `silver`) each get a distinguishing single-word completion token, and each
      token resolves back to the correct ring.
- [ ] Three identical "a ring" items get completion tokens `ring`, `2.ring`,
      `3.ring`, each resolving to the 1st/2nd/3rd ring in scope order.
- [ ] A mix — "a gold ring" plus two identical "a ring" — gives the gold ring its
      distinguishing token and the two plain rings ordinal tokens, with no
      collision (no two candidates share a completion token).
- [ ] Every completion token in a multi-match result resolves, under `§6`, to the
      distinct entity its candidate named (round-trip uniqueness).
- [ ] Display labels may repeat (two "a ring" candidates), but completion tokens
      within one result are unique.

## 7. Visibility, ordering, caps, determinism

### Visibility (information-leak safety)

Completion candidates MUST respect the **same visibility filter** the resolver
applies for that slot, including the per-argument visibility-bypass flag
(`commands-and-dispatch §5.1`). Today the visibility filter is a permissive stub
(`CanSee` always true), so there is no live leak — **but completion is a
textbook leak vector**, so this is a hard requirement, not a future nicety: when
hidden/sneak rules land (`BACKLOG §2`), an entity the actor cannot see MUST NOT
appear as a candidate unless the slot bypasses visibility. An acceptance test
asserting "a not-visible entity is absent from candidates" should be written now
against the stub seam so it is wired the day the rule becomes real.

### Ordering

- **Verbs:** routing priority (`§3`) — exact, then registration order.
- **Arguments:** scope iteration order — carried items in pickup order, room
  contents in room order, room entities in their enumeration order — i.e. the
  same order the resolver iterates, so ordinal completion tokens (`§6`) line up
  with the ordinals the resolver would assign.

Ordering is **deterministic** for a fixed world state: the same partial line and
the same room/inventory produce the same ordered candidate list.

### Caps

The candidate set is capped at a configurable maximum (`§8`). When the matched
set exceeds the cap, the result is truncated to the first N by the ordering above
and the **truncated** flag is set, so a surface can honestly signal "…and more"
rather than implying the list is complete. Truncation never drops the
highest-priority candidates (it keeps the front of the ordered list).

### Acceptance criteria

- [ ] A candidate the actor cannot see (once visibility is real) is absent unless
      the slot's argument declares visibility bypass; the bypass slot includes it.
- [ ] Two queries against identical world state return identical ordered results.
- [ ] A scope larger than the cap returns exactly the cap count with the
      truncated flag set, keeping the front of the ordered list.
- [ ] A scope at or under the cap returns the full set with the truncated flag
      clear.

## 8. Degradation & robustness

Completion is an assist; it never errors at the player and never blocks input.
Every path below yields a defined empty-or-partial result:

- **Unknown verb under an argument slot.** If the first token does not resolve to
  any command, there is no argument scope to enumerate; argument completion
  returns no candidates. (Verb completion of that same token still works.)
- **Un-migrated (hand-parsed) verbs.** Commands that declare no typed arguments —
  the verbs still resolving their operands inside the handler — expose no slot
  types, so argument completion returns no candidates for them. Verb completion
  is unaffected. (This is the bulk of the value gap that closes naturally as
  verbs migrate onto the `§5` pipeline; completion improves for free as they do.)
  A hand-parsed verb whose operand scope the auto-resolver can't express (e.g.
  `get`, whose item scope flips on a `from` preposition, or `kill`, whose
  self-check must precede resolution) MAY still opt into argument completion by
  **declaring its arg shape for completion while keeping the handler in charge of
  parsing** — the declaration drives completion (and help synthesis) but does not
  trigger auto-resolution at dispatch. This keeps such verbs completable without
  forcing them onto a resolution path that would change their behavior.
- **Slot past the declared arguments.** Covered in `§5`: no slot, no candidates.
- **Absent services / nil scopes.** A resolve context with an empty or absent
  scope (no room, no inventory, no door lookup) yields no candidates for the
  affected slot, never a panic.
- **Empty registry.** Verb completion over a registry with no matching keyword
  returns no candidates.

### Acceptance criteria

- [ ] `frobnicate xyz` (unknown verb) returns no argument candidates and does not
      error.
- [ ] A verb with no declared typed arguments returns no argument candidates for
      any later token, while still being offered as a verb candidate.
- [ ] A query with a nil/empty resolve context returns an empty candidate set,
      not a panic.
- [ ] No completion query, for any input, returns an error to the player or
      blocks the submitted line.

## 9. The `complete` debug verb (exercise surface)

Phase 0 ships one observable surface: a `complete` command that runs the query
on a supplied partial line and prints the result. Its purpose is to smoke the
substrate against a live room and to back the unit tests; it is **not** the
player-facing completion experience (that is Phase 1/2).

- **Gating.** The verb is **admin/role-gated** — it is an introspection tool that
  reveals candidate sets (and, once visibility is real, must itself not become a
  leak), so it is registered as administrative and hidden behind the admin role
  exactly like other admin verbs (`admin-verbs §2`). For a non-admin it is
  indistinguishable from an unknown verb.
- **Input.** The remainder of the line after the verb is the partial line to
  complete (free text, so it may itself contain spaces and partial tokens), e.g.
  `complete get sw` or `complete put gem in ch`.
- **Output.** A human-readable listing of the result: the completion target
  (verb slot or argument slot N of `<verb>`), and for each candidate its
  completion token, its display label, and its kind (verb / item / entity / door
  / bulk). When the result was capped, the output states it was truncated.
- **No mutation.** The verb is read-only; it changes no world or player state.

### Acceptance criteria

- [ ] `complete loo` (admin actor) lists `look` among verb candidates with the
      verb-slot target.
- [ ] `complete get ` in a room with items lists those items as room-item
      candidates with the argument-slot target.
- [ ] `complete` issued by a non-admin returns the same `Huh?` an unknown verb
      produces (no disclosure that the verb exists).
- [ ] The verb performs no state change (idempotent; safe to run repeatedly).
- [ ] A truncated result is reported as truncated in the output.

## 10. Configuration surface

| Setting | Meaning | Default |
|---|---|---|
| Candidate cap | Maximum candidates returned by one query before truncation (`§7`). | A small fixed list length (tens, not hundreds). |
| `complete` verb role | The role required to invoke the `complete` debug verb (`§9`). | The configured admin role (`admin-verbs §8`). |
| Empty-partial behavior | Whether a trailing-space (empty) partial returns the full scope or nothing. | Full scope (subject to cap). |

(Numeric defaults are deliberately left to the configuration layer per the house
convention that specs carry no magic numbers; the cap's default lives with the
other command-layer limits.)

## 11. Open questions

- **Alias presentation.** The substrate returns both primary keywords and aliases
  as verb candidates (`§3`). Should a surface dedup an alias against its primary,
  or is showing `n`/`north` separately actually useful muscle memory? Deferred to
  the Phase 1 surface; the substrate stays neutral by tagging each.
- **Distinguishing-keyword choice when several qualify.** `§6` says "a keyword
  that uniquely resolves to it"; when an entity has *several* such keywords, which
  is offered (shortest? first-declared? the one matching the partial)? Recommend
  "the one extending the partial when one does, else the first declared," but
  this is a refinement that doesn't change the invariant.
- **Cross-scope ordinals for `visible`.** The `visible` scope spans self +
  inventory + room (`§4`); when duplicates span sub-scopes, the ordinal order
  (`§6`) follows the resolver's concatenation order. Confirm that order is the one
  the `visible` resolver actually iterates so ordinals round-trip.
- **Cap interaction with disambiguation.** If truncation (`§7`) cuts a matched set
  mid-way, the ordinal tokens (`§6`) for the *included* duplicates are still
  correct (they index scope order, not result order), but the player can't see
  the later ones. Acceptable for a capped assist; noted so it isn't mistaken for a
  bug.

## 12. Presentation policy (decided — Phase 1+ surfaces inherit)

These govern any surface that renders/applies completion (Phase 1 GMCP, Phase 2
char-mode). **Signed off 2026-06-03** (`proposal §7`):

- **Multiple matches → longest-common-prefix + list.** A surface completes the
  token to the shared prefix of the candidate set, then presents the list
  (classic shell behavior). Not cycle-on-Tab.
- **Activation is automatic** for any client that advertises the completion
  capability — no per-player/account opt-in setting.
- **Compute model is request/response** — the query runs only when the client
  asks (on Tab) and the server replies once with the candidate set. Proactive
  **push** (streaming candidates for live ghost-text) is explicitly **out of
  scope**; it is additive over this same query and may layer on later without
  changing the request/response path.

## 13. GMCP surface — `Input.Complete` (Phase 1, shipped)

The request/response surface for GMCP-capable clients (Mudlet-class). Unlike the
`Char.*`/`Room.*` packages (server→client state pushes), this is a client→server
**request** with a server→client **reply**, so it lives in its own `Input.*`
namespace. The server side is shipped; the client (Tab key → request → render
reply) is the integration each client owns — see `docs/clients/tab-completion-gmcp.md`.

### Inbound dispatch (foundation)

The server accepts client→server GMCP on both transports through `conn.GmcpConn`
(`SetGmcpHandler`/`SendGmcp`/`SupportsPackage`). Telnet routes non-`Core.Supports`
subnegotiations to the handler; WebSocket dispatches inbound `gmcp` envelopes
(both skip `Core.Supports.*`). The session installs one handler per connection
that runs **synchronously on the read goroutine** — the same consistency a
command handler has. This inbound path is reusable for any future client→server
package.

### Packages

- **`Input.Complete`** (client→server) — `{"line": "<partial up to cursor>"}`.
- **`Input.Complete.List`** (server→client) — the reply:
  - `line` — the request line, echoed (so the client matches reply to request).
  - `target` — `"verb"` | `"argument"` | `"none"`.
  - `verb` — the resolved verb when `target == "argument"` (omitted otherwise).
  - `common` — the longest common prefix of candidate `value`s; the client
    completes the token to this before listing (the §12 LCP rule, server-computed).
  - `truncated` — true when the set was capped.
  - `candidates` — ordered `[{value, display, kind}]` (`value` round-trips through
    resolution; `kind` ∈ verb/item/entity/door/bulk).

### Behavior

On `Input.Complete`, the server runs the same query (`§2`) for the requesting
actor (admin-verb visibility follows the actor's own role) and replies with
`Input.Complete.List`. Request/response, automatic for any client that sends the
request (`§12`). No state change; safe to spam as the player types.

**Rate limit.** Inbound GMCP is throttled per connection by a token bucket
separate from the command flood gate (derived at 2× the command rate, since
completion can fire per keystroke). Over-rate frames are **dropped silently** —
GMCP abuse never disconnects a client (its command channel keeps its own gate);
it only sheds excess GMCP. The limit covers every inbound package, not just
completion. Pre-login frames are dropped (the handler is installed after login).

### Acceptance criteria

- [ ] An inbound `Input.Complete {line}` on telnet OR WebSocket produces one
      `Input.Complete.List` reply on the same connection.
- [ ] The reply's `candidates` match what `complete`/`suggest` would list for the
      same line + actor; `common` is the LCP of the candidate values.
- [ ] `target`/`verb` reflect the slot (verb vs argument); `none` for an
      uncompletable slot, with empty `candidates`.
- [ ] Admin verbs appear in the reply only for an actor holding the admin role.
- [ ] A malformed payload is ignored (no reply, no error to the player).

## 14. Deferred to later phases (not specced here)

- **Phase 2 — server-side character-mode line editing** for raw `telnet`/`nc`
  parity (the only way a line-mode client gets real TAB). Its own proposal.

**Shipped (not deferred):** the **raw-telnet line-mode stopgap** — the player-facing
`suggest <partial>` verb that *lists* completion candidates (no TAB key), rendering
the same query for players that the admin `complete` debug verb renders for
debugging. Single match → the completed line; multiple → the list with a
longest-common-prefix hint (§12); unknown → "no suggestions".
