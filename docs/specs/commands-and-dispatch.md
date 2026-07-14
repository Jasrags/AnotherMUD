# Commands and Dispatch — Feature Specification

**Status:** Draft · **Scope:** Command registration and resolution,
input parsing (chaining + repeat expansion), argument typing and
resolution, dispatch to player and mob actors, bad-input tracking,
emote registration, help-topic generation from registrations, and
the ability/command bridge that turns ability definitions into
typeable commands · **Audience:** Anyone reimplementing or porting
this feature in any language.

This document describes *what* the dispatch feature must do, not
*how* to implement it. Specific keyword sets, the chain length
limit, exact error strings, and the canonical engine arg type set
are policy / engine-defined and configurable at the integration
points called out below.

---

## 1. Overview

A command is the unit through which a player or a mob makes the
world do something. The dispatch feature is the bridge between
"a string the actor typed (or scripted)" and "a typed function
invocation against an entity in a room". Every other feature that
exposes a verb to players plugs in here.

The pipeline is:

```
raw input string
  ─► parse:  split on ';' into a chain; expand "Nverb" repeats
       │
       ▼
  ─► route per parsed entry:
       ├─► resolve registration by keyword (exact → prefix; priority wins)
       ├─► role gate (player/admin/mob source)
       ├─► build ActorContext
       └─► invoke handler
              │
              ├─► (optional) arg resolution into typed values
              └─► feature work (sends messages, mutates state)
```

The feature does NOT own:

- Flood control / token-bucketing (the session layer does — it
  consumes tokens before calling Route).
- Tick scheduling (the game loop does — it drains per-session
  input queues at the top of every tick).
- The actual side effects of any specific verb (`look`, `kill`,
  `say`, …). Each feature registers its own handlers.

### Core concepts

- **Command registration** — a content- or engine-supplied record
  binding a *keyword* (with optional aliases) to a *handler*
  invoked when the keyword is matched. Carries priority, role
  list, optional arg definitions, optional GMCP routing, plus
  metadata used by help and UI.
- **Keyword** — the primary string a user types. Resolution is
  case-insensitive.
- **Alias** — an alternate string that maps to the same
  registration. Aliases do not participate in prefix matching.
- **Priority** — integer disambiguator when multiple
  registrations share a keyword. Higher wins. Packs use this to
  override engine defaults.
- **Role** — `player`, `mob`, or `admin`. Constrains which
  *sources* can invoke the command (§3).
- **ActorContext** — the per-invocation object passed to a
  handler. Carries entity id, name, current room, the input
  source (`player` or `mob`), the raw input, the resolved verb,
  and the raw arg tokens.
- **CommandContext** — the wire-level package the session hands
  to the router for player input: player entity id, raw input,
  resolved verb, split args, and a chargen flag.
- **ArgDefinition** — a content-supplied typing declaration for
  one named argument of a command. Drives both runtime resolution
  and the auto-generated help syntax line.

### Goals

1. Provide a single keyword registry that all features (engine
   built-ins, packs, ability auto-registrations) use.
2. Resolve user input to exactly one registration when possible,
   with clear precedence rules (exact > prefix; higher priority
   wins; earlier registration breaks ties).
3. Gate access by source so mobs cannot invoke admin commands and
   players cannot invoke mob-only commands.
4. Parse compound input ("`n;n;e`" or "`3n`") deterministically
   with a bounded chain length.
5. Resolve typed arguments — inventory items, room entities,
   players, doors, containers, numbers, free text — through a
   small, fixed engine type set extended by pack types.
6. Track unknown-verb usage as a stream of structured events for
   admins and telemetry.
7. Auto-generate help topics from registrations that declare arg
   definitions, with pack-authored help overriding generated
   topics.
8. Provide a bridge that automatically registers every active
   ability as a typeable command, with visibility driven by
   proficiency.

### Non-goals

- Parser support for quoted strings, escapes, or shell-style
  pipe/redirection. Tokenization is space-split.
- Asynchronous commands or per-command cooldowns. Commands
  resolve synchronously within a tick.
- Permission systems beyond the three-role model. Fine-grained
  capability checks are the handler's responsibility.
- The shape of GMCP packages sent in response to commands. The
  registration's GMCP config is carried opaquely; the GMCP
  feature interprets it.
- The input *source* for mob commands — see `docs/specs/mobs-ai-
  spawning.md` for the mob command queue. This spec defines what
  happens once a mob command verb is routed.

---

## 2. Command registration

### 2.1 Shape

A registration MUST carry:

- A keyword (primary string).
- A handler that takes an ActorContext and returns nothing.

A registration MAY carry:

- A list of alias strings, each routable to the same
  registration.
- A priority integer (default zero).
- A pack name and a source file path (used by diagnostics and
  help; set automatically by the pack loader for pack-authored
  registrations).
- A description and a category (used by help and listing UIs).
- A visibility predicate `Entity → bool`. Used by listing UIs
  (`commands`, `skills`) to hide commands the actor cannot
  currently use. Visibility predicates MUST NOT be used to gate
  *execution* — they affect rendering only. The handler itself
  is responsible for re-checking preconditions.
- A roles list defaulting to `["player"]`.
- An ArgDefinitions map (named arguments in declaration order;
  see §5).
- A GMCP config (channel routing for feedback messages produced
  by the handler).

### 2.2 The registry

A single registry holds every registration, keyed
case-insensitively. Multiple registrations may share the same
keyword (different priorities), and multiple keys (keyword +
aliases) may point at the same registration.

Operations:

- **Register.** Append to the per-keyword list under the primary
  keyword and under each alias. Record an internally-monotonic
  registration-order index for tie-breaking.
- **Unregister.** Remove every alias entry for matching primary
  registrations, then remove the primary entry.
- **All keywords.** Iterate every key in the registry, including
  alias keys. Used for diagnostics.
- **Primary keywords.** Iterate the *distinct* primary keywords
  across all registrations (collapsing alias entries to their
  primary form). Used by listing UIs.

### 2.3 Resolve

Resolution takes a candidate string and (optionally) a source
keyword. The base algorithm:

1. **Exact match.** Look up the candidate as a key (primary or
   alias). If hits exist, return the registration with the
   highest priority. Ties on priority do not need a deterministic
   resolution at this stage (exact-match collisions on the same
   priority would indicate registration error).
2. **Prefix match.** If no exact match, scan all *primary*
   keywords (not aliases) and collect every registration whose
   primary keyword starts with the candidate. Sort the resulting
   set by descending priority, then ascending registration order;
   return the first.

Prefix matching MUST NOT consider aliases — only primary
keywords. This keeps the prefix-completion behavior of a typed
short verb predictable (a player who types `n` should get
"north", not the first alias of some unrelated command that
happens to start with `n`).

When a source keyword is supplied:

- After the base resolution, the source MUST match the
  registration's role list:
  - `mob` source matches when the role list contains `mob`.
  - `player` source matches when the role list contains `player`
    OR `admin`. (Admin commands are reachable from the player
    input path; the router applies an additional role check on
    the actor, §3.1.)
  - Any other source matches every registration.
- A source mismatch causes resolution to return no registration.

**Acceptance criteria**

- [ ] Both keyword and aliases route to the same registration.
- [ ] Highest-priority registration wins on exact matches.
- [ ] Prefix matches consider primary keywords only.
- [ ] Prefix ties resolve by ascending registration order at
      equal priority.
- [ ] Source `mob` matches only `mob`-tagged registrations.
- [ ] Source `player` matches `player` and `admin` registrations.

---

## 3. Routing

### 3.1 Player route

Routing player input takes a CommandContext (entity id, raw
input, verb, split args, chargen flag). The router MUST:

1. **Empty-input gate.** If the verb is empty or whitespace,
   return without effect.
2. **Resolve** the verb with source `player`. If unresolved:
   - Record the unknown verb to the bad-input tracker with the
     verb, full raw input, actor name, and current room id (§6).
   - Send a "Huh?" reply to the actor.
   - Return.
3. **Admin gate.** If the resolved registration's role list
   contains `admin`, look up the actor entity. If the entity is
   missing OR does not carry the `admin` role, send "Huh?" and
   return. This treats admin commands as invisible to
   non-admins — both presence and parameters are hidden.
4. **Build ActorContext** carrying `Source = "player"`, the
   raw input, verb, and arg tokens.
5. **Invoke** the handler.

The router does NOT consume flood-control tokens, does not log
beyond bad-input recording, and does not catch handler
exceptions (the session layer wraps the call).

### 3.2 Mob route

Mob input enters by a different path because mobs do not have
sessions. The router exposes `route-for-mob(entityId,
commandStr, roomId, name)`:

1. **Tokenize.** Split on space, dropping empties. If no tokens
   remain, return.
2. **Resolve** the first token with source `mob`. If unresolved,
   silently return. Mob unknown verbs are NOT recorded by the
   bad-input tracker; mob command verb sets are tightly
   controlled by content, and silent failures keep noisy
   misbehaving handlers from flooding admin telemetry.
3. **Build ActorContext** carrying `Source = "mob"`, the full
   command string as raw input, the verb, and the remaining
   tokens as args.
4. **Invoke** the handler.

### 3.3 Player vs mob differences

| Behavior | Player route | Mob route |
|---|---|---|
| Empty input | no-op | no-op |
| Unknown verb | "Huh?" + bad-input record | silent |
| Admin gate | enforced via actor role | n/a |
| Source string in ActorContext | `player` | `mob` |
| Raw input population | from CommandContext | full passed string |

### 3.4 Handler contract

Handlers receive a single ActorContext. They are responsible
for:

- Re-checking any precondition that the visibility predicate
  encodes for rendering. Visibility is a UI hint, not a gate.
- Resolving arguments. The dispatch feature provides an Arg
  resolver (§5) as a service; handlers MAY call it directly OR
  inspect `RawArgs` themselves. Whether a handler uses the
  resolver is a per-handler choice.
- Producing all output to the actor and the room via session-
  layer helpers; the dispatch feature does not buffer output.
- Emitting any feature-specific events.

Handlers MUST NOT mutate the registration itself or the actor's
ActorContext (it is intended to be read-only data).

**Acceptance criteria**

- [ ] Unknown player verbs always produce a "Huh?" and a bad-
      input record.
- [ ] Admin commands are unreachable for non-admin actors and
      produce the same "Huh?" response (no distinct error).
- [ ] Mob route never produces "Huh?" or admin gates.
- [ ] Handlers receive exactly the ActorContext the router
      built; subsequent route invocations build fresh contexts.

---

## 4. Input parsing

Player input passes through a parser that splits the raw line
into one or more `(verb, args)` pairs.

### 4.1 Chaining

The raw line is split on the chain separator (semicolon). Each
non-empty segment is trimmed and parsed independently. A configured
**chain length cap** bounds the number of produced commands; once
the cap is reached, remaining segments are dropped silently.

This means `n;e;w;say done` runs four commands in order, and a
crafted line of two hundred semicolons does not produce two
hundred commands.

### 4.2 Repeat expansion

Within a single segment, if the first token begins with one or
more decimal digits followed by additional non-digit characters
(e.g. `3n`, `12east`, `2pick`), the token is split into a
**count** (the leading digits) and a **verb** (the remainder).
The segment is then expanded into `min(count, remaining cap)`
identical `(verb, args)` entries.

The expansion is purely lexical — the parser does NOT consult
the registry to decide whether to expand. Tokens that start with
digits and a non-digit suffix are universally expanded; tokens
that start with digits and contain only digits (e.g. `3`) are
NOT expanded (no suffix) and pass through as a single command
whose verb is the digit string.

Count zero is treated as "no expansion" (a request to run zero
times is the same as running once); see open questions.

### 4.3 Tokenization within a segment

A segment is space-split with empty tokens dropped. The first
token is the verb. Remaining tokens are the args. No quoting,
escaping, or multi-word literal support is provided by the
parser; commands that need free text consume the remaining args
as joined text via the arg resolver (§5.4).

### 4.4 Parser output

The parser yields an ordered list of `(verb, args)` pairs. The
caller (session layer) typically enqueues each pair to be
dispatched on subsequent ticks, subject to per-tick rate caps
that live outside this feature.

**Acceptance criteria**

- [x] Chain separator splits independently per segment.
- [x] Chain length cap drops trailing segments silently.
- [x] Repeat expansion handles `3n`, `12east`, `2pick item`.
- [x] Pure-digit tokens (`3`) are not expanded.
- [x] Repeat count is bounded by the remaining chain cap so a
      `999n` cannot blow past the cap.
- [x] No quoting/escaping is interpreted; everything is space-
      split.

> **Implementation (`internal/command/parse.go` `ParseInput`).** The parser
> returns the ordered, expanded command segments as ready-to-dispatch
> strings (a (verb, args) pair is lossless as a string under the
> no-quoting rule), so the session pump feeds each straight to `Dispatch`.
> The cap is `Config.ChainCap` (env `ANOTHERMUD_CHAIN_CAP`, default 10).
>
> **v1 dispatches the expanded commands synchronously, in order, within
> the one input read** — it does NOT enqueue them across ticks (the §4.4
> "subsequent ticks" model). Per-tick pacing is explicitly out of scope
> here (§4.4: "subject to per-tick rate caps that live outside this
> feature"); movement and most verbs already dispatch synchronously, the
> chain cap bounds expansion, and the per-line flood gate counts the whole
> submission once. A paced input queue would be a separate, larger build.

---

## 5. Argument typing and resolution

### 5.1 The ArgDefinition

A command may declare a *map* of named argument definitions, in
declaration order. Each definition carries:

- A **type** string (engine-known or pack-registered, §5.2).
- A **required** flag (default true).
- A **bulk** flag (default false), meaningful only for the
  bulk-capable inventory and room-item types.
- A **prepositions** list. Tokens listed here are consumed by
  the resolver — silently skipped — when seen *immediately
  before* this argument's expected token. Used so commands can
  spell themselves naturally ("`put gem in chest`",
  "`unlock door with key`") without each handler hand-parsing
  the preposition.
- A **bypass-visibility** flag. When true, the resolver does NOT
  filter candidates through the visibility filter — used for
  commands that intentionally operate on hidden entities (e.g.
  `look at <fixture>`).

The map's iteration order MUST be the declaration order; the
resolver consumes argument tokens left to right in that order.

### 5.2 Engine arg type set

The dispatch feature recognizes a fixed set of engine arg type
keywords. Content packs may extend the set (§5.3) but MUST NOT
override an engine type.

The canonical engine types and what they resolve to:

| Type | Resolves to |
|---|---|
| `keyword` | the raw token verbatim |
| `text` | the joined remainder of the args as a single string |
| `number` | an integer, or failure with "That's not a number." |
| `inventory` | an item in the actor's contents, possibly bulk |
| `room_item` | a non-actor item in the actor's current room, possibly bulk |
| `entity` | a player or mob in the actor's current room |
| `player` | a player in the actor's current room |
| `npc` | a mob in the actor's current room |
| `container` | a container in inventory first, then in the room |
| `visible` | any visible entity (self/inventory/room) with source tag |
| `findable` | an item in inventory first, then in the room |
| `door` | a door reachable by direction or keyword resolution |

The `entity` / `player` / `npc` types intentionally exclude the
actor from candidates so that "`kill self`" requires the explicit
self handling done by handlers, not accidental input matching.

The `visible` type returns extra metadata — a `source` field
indicating whether the match came from inventory, the room, or
self — so commands like "`look`" can render differently based on
where the target was found.

The `door` type returns a structured shape with the direction
short string and the door's name/closed/locked/key fields, so
door handlers can act on door state without re-querying.

### 5.3 Pack-registered arg types

Packs may register additional arg type names with custom
resolvers. The registration:

- MUST be rejected with a warning when the name collides with an
  engine type. Engine types are immutable.
- MAY collide with another pack type; the last registration
  wins, with a warning.

An unknown arg type encountered at resolution time MUST fall
back to passthrough (treat the token as a `keyword`) with a
warning log.

### 5.4 Resolution

The resolver consumes tokens in declaration order. For each
argument:

1. **Preposition skip.** If the next token is in the
   declaration's preposition list (case-insensitive), advance
   past it.
2. **Text type early-out.** If the type is `text`, slurp the
   remainder of the tokens (joined with spaces) into the
   resolved value and stop consuming further tokens. If no
   tokens remain and the arg is required, fail with
   "What &lt;argName&gt;?".
3. **Missing required.** If no tokens remain and the arg is
   required, fail with "What &lt;argName&gt;?".
4. **Resolve token.** Run the engine or pack resolver with the
   actor context, the definition, and the token. On failure for
   a required arg, fail with the resolver's error (or a generic
   fallback).
5. **Store result.** Place the resolved value (or `null` on an
   optional miss) under the argument's name in the result map.

The final result is `(success, named map, error)`. The first
required-arg failure short-circuits; the error string is what
gets sent back to the player.

**Usage-on-error.** When the short-circuit is specifically a
*missing required argument* (step 2 / step 3 — the player typed
the verb but not its operands), the dispatcher appends the
command's synthesized usage line (see §8 and `ui-rendering-help`
§10.4) below the "What &lt;argName&gt;?" prompt, so the player
learns the command's shape. A resolver *value* failure (step 4 —
a token is present but invalid, or a named target doesn't
resolve) already carries a specific message and does NOT get the
usage echo. Hand-parsed commands (§5, those that resolve their
own arguments) are outside this path; each surfaces its own
guidance.

**Acceptance criteria**

- [ ] A missing required argument sends the prompt plus the
      synthesized usage line.
- [ ] A present-but-invalid token sends only the resolver's
      message, no usage line.

### 5.5 Ordinal selectors

For type that accept ordinal selection (inventory, room item,
entity, player, npc, container, visible, findable), the resolver
accepts a `<positive integer>.<keyword>` token. The integer is a
1-based ordinal selecting among matches; ordinals outside the
match range produce the type's default not-found error.

Bulk operations (`all`, `all.<keyword>`) coexist with the
ordinal syntax but are mutually exclusive within a single token.

### 5.6 Resolved value shapes

Each engine type returns a stable shape:

- `keyword` / `text`: the raw or joined string.
- `number`: an integer.
- `inventory` / `room_item` / `container` / `findable`:
  a structured item object carrying id, name, keyword
  (best-effort canonical), and template id. Bulk variants
  return an array of these.
- `entity` / `player` / `npc`: a structured object carrying id,
  name, and type.
- `visible`: id, name, type, and a `source` discriminator.
- `door`: a direction short string and a nested door object
  (name, closed, locked, key id).

Handlers that bypass the resolver do not get these shapes —
they see only the raw string tokens.

**Acceptance criteria**

- [ ] Engine types cannot be overridden by pack registrations.
- [ ] Unknown arg types pass through as keyword with a warning.
- [ ] Prepositions are consumed only when present immediately
      before their argument.
- [ ] `text` slurps the joined remainder.
- [ ] First required-arg failure short-circuits resolution.
- [ ] Ordinal selectors work uniformly across selecting types.
- [ ] `all` and `all.<keyword>` produce arrays in bulk-capable
      types.
- [ ] Resolved entity / item shapes are stable and include the
      template id where applicable.

---

## 6. Bad-input tracking

The router records every unknown player verb in a structured
in-memory tracker. The tracker MUST:

- Key entries by the verb (lower-cased, trimmed).
- Maintain a count, first-seen timestamp, and last-seen
  timestamp per verb.
- Increment OR insert atomically (concurrent input is allowed).
- Log each occurrence with the verb, the full raw input, the
  player name, and the current room id.
- Increment an OpenTelemetry counter (when configured) tagged
  by the verb, so dashboards can rank misfires.
- Expose a snapshot accessor returning entries sorted by count
  descending. Used by admin commands and dashboards.
- Expose a clear operation for tests and operator triage.

The tracker is informational. The router never uses the
tracker's state to change routing behavior; entries persist for
the life of the process unless explicitly cleared.

Mob unknown verbs are NOT recorded by the tracker.

**Acceptance criteria**

- [x] Every unknown player verb increments the tracker by
      exactly one.
- [x] Concurrent unknown verbs do not lose count. *(single mutex over
      increment+insert; race-tested.)*
- [x] Snapshot returns entries sorted by descending count.
- [x] Mob unknown verbs do not appear in the tracker. *(by construction —
      only the player `Dispatch` records; mobs route through `internal/ai`.)*

> **Implementation (`internal/command/badinput.go` `BadInputTracker`).**
> The dispatcher records + logs (`event=command.unknown`) at the unknown-
> verb miss, NOT at the admin-gate "Huh?" (that verb is known, just refused).
> The `badinput` admin verb renders the snapshot; `badinput clear` resets it.
> The **OTel counter** ("when configured") is deferred — the project ships
> no metrics infra yet (a nil-able hook lands with the Ops track); all four
> acceptance criteria above hold without it.

---

## 7. Emotes

The emote registry holds named emote definitions that command
handlers (typically a single `emote` or per-emote registrations)
use to render templated text in three views: self, room, and
optionally a target and target-room view.

A definition MUST carry:

- A name (registry key).
- A self message template (what the actor sees).
- A room message template (what others in the room see).
- Optional target message template (what a named target sees).
- Optional target-room message template (what the rest of the
  room sees when the emote has a target).

The registry exposes a `format(template, actorName,
targetName?)` helper that substitutes:

- `{name}` → actor's name.
- `{possessive}` → actor's name + apostrophe-s.
- `{target}` → target's name (only when supplied).

Templates that reference `{target}` without a target supplied
will pass through as literal text; this is content-author
responsibility, not a runtime error.

This feature does not own the emote command handler itself; the
handler is registered by content or by an engine-built-in and
uses the registry as a lookup.

**Acceptance criteria**

- [ ] Registry lookups are case-insensitive on the emote name.
- [ ] `{name}`, `{possessive}`, `{target}` substitute as
      specified.
- [ ] Missing target in a template referencing `{target}` does
      not raise an error.

---

## 8. Help generation

For every registration that declares an arg definition map, the
help-generation pipeline produces a help topic with:

- A title (the keyword).
- A category (the registration's category, or `commands` when
  unset).
- A brief and body derived from the description and roles list.
- A synthesized syntax line: keyword followed by each argument
  rendered as `[argName]` (required) or `([argName])`
  (optional), with bulk args rendered as
  `[argName | all | all.argName]`. Prepositions appear in the
  syntax line in their declared position.
- A keywords list containing the primary keyword.

Generated topics are added to the help service at a low
load-order priority, so pack-authored help files (which carry a
higher load order) shadow generated topics. Commands without arg
definitions do NOT produce generated topics — content can supply
free-form help for them or none at all.

**Acceptance criteria**

- [x] Topics generated for every registration with arg
      definitions, none for those without. *(impl is a deliberate
      **superset**: a topic is generated for every command — an untyped one
      gets its hand-authored Syntax + Brief, a typed one gets a **synthesized**
      syntax line. This gives every verb baseline help without a pack file;
      `internal/command/help.go` `GenerateHelpTopics` / `synthesizeSyntax`.)*
- [x] Generated syntax uses `[ ]` for required and `( [ ] )` for
      optional and `[ | all | all.X ]` for bulk. *(prepositions render in
      position: `put [gem] in [chest]`.)*
- [x] Pack help overrides generated topics at higher load order.
      *(generated topics load at order 0; the generator also skips a keyword
      a pack already covered — `HasTopic`.)*

---

## 9. Ability / command bridge

After packs finish loading, the bridge iterates every *active*
ability in the ability registry and registers a command for each.

### 9.1 Keyword and aliases

For each active ability:

- **Primary keyword.** The ability's declared command-name when
  set, otherwise the **short id**: the suffix of the ability id
  after the last colon. (For `legends-forgotten:fireball`, the
  short id is `fireball`.)
- **Alias.** When the primary keyword differs from the full
  ability id, the full id is registered as an alias so that
  ability ids remain typeable for scripting and admin use.

### 9.2 Visibility predicate

The registration's visibility predicate returns true iff the
actor's proficiency in the ability is greater than zero. The
listing UIs use this to show only abilities the actor has
learned; the handler ALSO re-checks proficiency at execution
time so a hidden-but-resolvable command still fails with a
friendly message.

### 9.3 Handler

The auto-generated handler:

1. Look up the actor entity. Return silently if missing.
2. Verify proficiency > 0. If not, send "You don't know how to
   &lt;display name&gt;." to player actors; mob actors silently
   return.
3. Resolve a target (§9.4). If unresolved, return.
4. If the target is another entity AND the actor is not
   currently in combat, auto-engage combat with the target
   first. Honor flee cooldown ("you're too winded from
   fleeing") for player actors; report engagement refusal as
   "you can't attack that".
5. Append a queue entry to the actor's ability action queue
   with the ability id and target id.

The actual ability resolution then happens on the next ability-
resolution pulse; see `docs/specs/abilities-and-effects.md` §4.

### 9.4 Target resolution

Target precedence:

1. **Explicit args.** When the handler received args, join them
   as a single lower-cased phrase. Special cases `self`, `me`,
   or the actor's own name resolve to the actor itself. An
   `<integer>.<name>` ordinal works against entities in the
   actor's room. If the room has no matching entity, fall back
   to the actor's current primary combat target (so a stale
   target arg during combat does not strand the cast).
2. **No args, in combat.** Use the actor's current primary
   combat target.
3. **No args, not in combat, self-targetable ability.** Use the
   actor.
4. **No args, not in combat, not self-targetable.** For player
   actors, send "Use &lt;display name&gt; on whom?" and return
   no target. For mob actors, return no target silently.

### 9.5 Priority and overrides

Auto-generated registrations register at priority 0. Pack
authors who need custom logic for a specific ability register
their own command at priority > 0 to shadow the auto-generated
entry, without unregistering it.

**Acceptance criteria**

- [ ] Only active abilities are bridged.
- [ ] Primary keyword honors `command-name` when set, otherwise
      uses the short id.
- [ ] The full ability id is always typeable as an alias when
      it differs from the keyword.
- [ ] Visibility predicate uses proficiency > 0; handler
      re-checks the same condition.
- [ ] Auto-engage runs only when the target is not the actor
      and the actor is not already in combat.
- [ ] Target resolution falls back from explicit args to combat
      target to self per the precedence in §9.4.
- [ ] Pack handlers at priority > 0 shadow auto-generated
      handlers cleanly.

---

## 10. Observable events

The dispatch feature itself does NOT emit events for routing
successes or failures. The bad-input tracker emits logs and an
OpenTelemetry counter (§6); handlers emit feature-specific
events. This is intentional — every command does a different
thing, and a generic "command run" event would be useless noise.

Features that *do* emit events on command execution:

- Communication-style commands (say, tell, channels) emit
  `communication message` events with payload routed by the
  registration's GMCP config.
- Movement commands wrap the world's move primitive (see
  `docs/specs/world-rooms-movement.md`) and emit a player-moved
  event with old/new room ids.

These belong to the respective features, not to dispatch.

---

## 11. Configuration surface

The following are externally configurable and not fixed by this
spec.

| Policy | Where it applies |
|---|---|
| Engine command set (built-in registrations) | §2 |
| Chain length cap (number of commands per input line) | §4.1 |
| Engine arg type set | §5.2 (extensible, not overridable) |
| Pack arg type registrations | §5.3 |
| OpenTelemetry counter wiring for bad input | §6 |
| Bad-input tracker caps (max distinct verbs, max verb key length) | §6 |
| Emote catalog (engine + pack) | §7 |
| Help loading order for generated vs file-based topics | §8 |
| Ability-bridge behavior toggles (e.g. skip-bridge tag) | §9 |

---

## 12. Open questions / future work

- **Prefix-match ambiguity is silent.** Two registrations whose
  primary keywords share a prefix the user types (e.g. `cast`
  and `castigate` with prefix `c`) both qualify; the higher-
  priority one wins, the other is invisible. A "did you mean…"
  hint when prefix matches > 1 candidate would be friendlier.
- **No quoting in tokenizer.** `say "hello world"` becomes verb
  `say` and three tokens. The `text` arg type slurps the
  remainder, which works for `say` but not for commands that
  want a quoted literal followed by more args.
- **Repeat expansion is purely lexical.** `0n` runs `n` zero
  times → no commands. Whether that is the intended behavior
  ("zero means once" is the legacy MUD convention; current
  behavior is "zero means zero") should be locked in.
- **Visibility is rendering-only.** Commands hidden from listing
  remain resolvable by typing them directly. This is
  intentional for muscle-memory and tab-complete but means
  visibility predicates cannot be used as security. The spec
  states this; codebases that grow tempted to use visibility as
  access control should be redirected to the role system.
- **Admin commands appear as "Huh?".** Hiding admin commands
  behind the same response as unknown verbs prevents leakage
  but makes legitimate typos by admins indistinguishable from
  unknown verbs. An admin-aware response that distinguishes
  would be friendlier.
- ~~**Bad-input tracker grows unbounded.**~~ **Resolved.** The map is
  capped at a maximum number of distinct verbs (existing entries keep
  counting; new keys past the cap are dropped until `badinput clear`), and
  each verb key is truncated to a maximum rune length so a single key can't
  bloat. A creative attacker can no longer fill memory with arbitrary keys.
  LRU eviction (vs. drop-on-full) remains a possible refinement.
- **No mob bad-input record.** Silent mob failures make
  misbehaving mob behaviors hard to spot. A separate "mob
  unknown verb" log path would help operators without polluting
  the bad-input dashboard.
- **Arg resolver is per-token.** Multi-word entity names (e.g.
  "fire elemental") are matched via substring on a single
  token. A multi-token entity resolution mode (consume tokens
  until a unique entity name is matched) would be more natural
  but interacts badly with `text` and ordinal syntax.
- **Auto-engage in ability bridge happens inside dispatch.**
  Auto-engaging combat from a command handler couples
  dispatch's bridge to the combat feature. A clean alternative
  is for the ability resolution phase itself to engage on first
  hit, but that delays the engagement signal.
- **Pack-type override warning is non-fatal.** A pack that
  overrides another pack's arg type registration causes a
  warning and a silent last-wins. A pack manifest that declares
  the override explicitly (or names the predecessor) would
  catch unintended overrides.

---

<!-- Generated: 2026-05-21 · Scope: CommandRegistry + CommandRouter + CommandInputParser + ArgResolver + ArgDefinition + ActorContext + BadInputTracker + EmoteRegistry + CommandHelpGenerator + AbilityCommandBridge · Spec style: narrative + acceptance criteria · Detail level: behavior only -->
