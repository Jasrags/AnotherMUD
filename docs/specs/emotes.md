# Emotes — Feature Specification

**Status:** Draft · **Scope:** Player-emitted social actions
expressed as room-scoped output (`smile`, `nod`, `wave`, `bow`,
the freeform `emote <text>`); the pronoun/name substitution
that gives actor / target / room observers each a correctly-
phrased view; the registry shape that lets packs ship their
own emote tables · **Audience:** Anyone reimplementing or
porting this feature in any language.

This document describes *what* the emote surface must do, not
*how* to implement it. Specific substitution token syntax,
default emote table, and policy on edge cases live in the
configuration-surface table at §8.

Emotes are the third sibling in the social-MUD theme (see
`docs/THEME-AXIS-PLAN.md` Theme A and
`docs/themes/social-mud-plan.md`). Unlike channels and tells,
emotes are **not** addressed messages — they are room-scoped
output, like `say`. They do not publish through the
[notifications](notifications.md) queue.

---

## 1. Overview

An emote is a player-emitted line of flavor text rendered with
three potentially-distinct points of view in the same room:

- **Actor view** — what the player who emoted sees.
- **Target view** — what the player being emoted-at sees (if
  any).
- **Room view** — what every other observer in the room sees.

The three views are constructed by substituting tokens
(actor's name, target's name, pronouns) into per-view
template strings declared by each emote.

Two flavors of emote exist:

1. **Table-driven emotes** — a named entry in the emote
   registry. `smile`, `nod`, `wave`. Each entry ships the
   three view templates and accepts an optional target.
2. **Freeform emote** — `emote <freeform text>` (canonical
   verb name in §8). Prepends the actor's display name to
   the supplied text and broadcasts as the room view. Actor
   sees the same line they typed (with their own name) as
   confirmation.

### 1.1 What emotes are not

- **Not addressed messages.** Emotes are room-scoped. A player
  in another room will never see them.
- **Not persisted.** No emote scrollback, no transcript. If
  you weren't in the room, you didn't see it.
- **Not gated by the notifications substrate.** Emotes use
  the existing per-room broadcast path (the same surface
  that backs `say`, movement arrival/departure, combat
  messages).
- **Not subject to ignore.** When ignore lands as a follow-
  up, it will filter channel and tell output. Whether it
  also filters emotes is an explicit open question (§9).

---

## 2. The emote registry

Each emote in the registry carries:

- **id** — short stable identifier (`smile`, `nod`). Used as
  the verb name and as the registry key.
- **aliases** — optional list of alternate verb names that
  resolve to the same emote (`bow` → `bows`, `grin` → `grins`).
- **requires_target** — boolean. When `true`, invoking the
  emote without a target argument returns
  `NoTargetSpecified`. When `false` (the common case), the
  emote has two template sets — one for the targeted form,
  one for the no-target form.
- **templates** — view templates organized as below.

### 2.1 Template shape

Each emote ships up to two template groups:

```
no_target:
  actor_view: "You smile."
  room_view:  "$n smiles."

targeted:
  actor_view:  "You smile at $N."
  target_view: "$n smiles at you."
  room_view:   "$n smiles at $N."
```

Rules:

- If `requires_target: true`, only the `targeted` group is
  present. Invoking without a target fails.
- If `requires_target: false`, both groups must be present.
  Invoking with no target uses `no_target`; with a target,
  uses `targeted`.
- A template missing a required view (e.g., no `room_view`
  in `no_target`) is a load-time error.

### 2.2 Substitution tokens

The substitution grammar is policy (exact syntax in §8) but
the substrate must support at least these meanings:

| Meaning | Notation (example) |
|---|---|
| Actor display name (subject case) | `$n` |
| Actor possessive | `$s` |
| Actor reflexive | `$m` |
| Target display name (object case) | `$N` |
| Target possessive | `$S` |
| Target reflexive | `$M` |

Pronouns derive from the actor / target's pronoun-set
field. Pronoun sets are a single-field per-player property
(`he/him`, `she/her`, `they/them`, etc.) configurable at
character creation or via a future settings verb (not in
v1 scope; v1 uses a default set per character — see §9).

Pronouns are *not* hardcoded per token. The substitution
layer asks the actor/target for their pronoun set, then
fills in the appropriate form. Adding a new pronoun set is
a content / config concern, not a code change.

### 2.3 Declaration source

Emotes are declared in:

- **Engine baseline.** A small fixed set (configurable in §8).
- **Packs.** Same convention as channels: `<pack>/emotes/*.yaml`
  files (glob in §8). Pack emote ids must not collide with
  engine baseline ids or with another pack's ids. Collisions
  are load-time errors (same as channel collisions; see
  [chat-channels-and-tells](chat-channels-and-tells.md) §3.2).

### Acceptance — registry

- [ ] Each emote carries id, optional aliases,
      requires_target, and the appropriate template groups.
- [ ] A `requires_target: false` emote that omits either
      `no_target` or `targeted` is a load-time error.
- [ ] A missing required view inside a template group is a
      load-time error.
- [ ] Pack-declared emotes that collide with engine baseline
      ids or sibling pack ids are rejected at load time.
- [ ] Engine baseline emotes load before pack discovery and
      are available with no packs loaded.

---

## 3. Substitution

The substitution pass:

1. Reads the chosen view template (actor / target / room).
2. Walks tokens left-to-right.
3. For each token, asks the appropriate entity (actor or
   target) for the field it needs (display name, pronoun
   form).
4. Replaces the token with the resolved string.

Rules:

- A token referencing the target (`$N`, `$S`, `$M`) in a
  template chosen because there is no target is a template-
  authoring error caught at load time, not a runtime error.
- A token whose pronoun form is not defined for the
  entity's pronoun set falls back to a configurable default
  (e.g., "they" when no third-person-singular form exists).
  This is a config decision, not a substrate decision.
- Substitution never escapes or interprets the surrounding
  text — template authors own the prose.
- Output is the rendered line ready for the per-room
  broadcast path. No further interpretation.

### Acceptance — substitution

- [ ] All substitution tokens documented in §2.2 work in
      every template view they appear in.
- [ ] A targeted-template token (`$N` etc.) used in a
      no-target template is rejected at load time.
- [ ] Substitution never mangles surrounding template text
      (escape characters, multi-byte sequences, ANSI
      decoration stay intact).
- [ ] A pronoun form missing on an entity's pronoun set
      falls back to the configured default without
      crashing.

---

## 4. Verb dispatch

Every loaded emote registers a verb (its id + aliases) at
boot. Invocation flow:

1. Player types `<emote> [target]`.
2. The verb resolves to the registry entry.
3. If `target` is empty:
   - If `requires_target`: return `NoTargetSpecified` to
     the actor with the canonical failure copy.
   - Else: render the `no_target` group.
4. If `target` is non-empty:
   - Resolve the target via the keyword resolver, scoped
     to entities in the actor's room. Players, mobs, and
     possibly items (open question, §9) may be valid
     targets.
   - If resolution returns nothing: return `NoSuchTarget`
     to the actor.
   - Render the `targeted` group.
5. Render three views (actor, target if applicable, room).
6. Broadcast:
   - Send the actor view to the actor.
   - If there is a target with a live session in the room,
     send the target view to the target.
   - Send the room view to every other entity in the room
     that can receive room broadcasts (excludes the actor;
     excludes the target if a separate target view was
     sent).

### 4.1 Freeform `emote` verb

The freeform variant has a fixed dispatch:

1. Player types `emote <freeform text>`.
2. Empty text returns `NothingToEmote` to the actor.
3. The substrate prepends the actor's display name to the
   text and broadcasts as the room view (same path as
   table-driven `room_view`).
4. The actor sees the same constructed line (so they get
   the visible confirmation everyone else does).
5. No target resolution. No pronoun substitution. The
   actor is responsible for grammar in their own freeform
   text.

### Acceptance — dispatch

- [ ] Every registered emote has a verb and dispatches to
      the same flow above.
- [ ] Aliases dispatch to the same registry entry as the
      primary id.
- [ ] A targeted emote with `requires_target: true` and
      no target returns `NoTargetSpecified` and does not
      broadcast.
- [ ] An emote with an unresolved target returns
      `NoSuchTarget` and does not broadcast.
- [ ] Target view is sent only when the target is a player
      with a live session in the actor's room.
- [ ] Freeform `emote <text>` always broadcasts; the
      actor's own copy mirrors what others see.

---

## 5. Targeting rules

- Targets are scoped to the actor's current room. Cross-room
  targeting is impossible.
- Players, mobs, and items are all considered (subject to
  the open question in §9 about non-player targets).
- The keyword resolver from
  [commands-and-dispatch](commands-and-dispatch.md) is the
  single authority — same precedence (exact → prefix →
  substring), same ordinal syntax (`2.guard`), same
  failure modes.
- Self-targeting (`smile me`, `smile self`, or aliasing the
  actor's own name) is allowed and uses the `targeted`
  templates; the substitution layer feeds the actor as
  both actor and target. Authors are expected to write
  templates that read sensibly when this happens
  (`You smile at yourself.` etc.).

### Acceptance — targeting

- [ ] Cross-room targeting never resolves.
- [ ] Targeting uses the shared keyword resolver with
      identical precedence to other commands.
- [ ] Self-targeting is allowed and produces sensible
      output when authors write templates accordingly.

---

## 6. Observability

| Event | Fields | When |
|---|---|---|
| `emote.invoked` | emote_id, actor, target?, room | dispatch resolves |
| `emote.no_target` | emote_id, actor | requires_target failed |
| `emote.no_such_target` | emote_id, actor, query | resolver returned nothing |
| `emote.freeform` | actor, room, length | `emote <text>` invoked |
| `emote.collision` | id, pack, colliding_pack | duplicate id at pack load |

Routine invocations log at `debug` (a busy MUD doesn't need
every wave logged). Load-time errors and unresolved targets
log at `warn` and `info` respectively.

### Acceptance — observability

- [ ] Every observable transition emits exactly one log
      line.
- [ ] Routine invocations log at `debug`; load-time errors
      at `warn`.

---

## 7. Persistence

**Emotes do not persist.** The registry is rebuilt from
content on every boot. There is no emote history, no
favorite-emote list, no per-player emote cooldown state.

Pronoun sets are persisted on the player file (alongside
other character properties), but that is the responsibility
of `character-creation` / `progression`, not this spec.

### Acceptance — persistence

- [ ] No file under `saves/` is written or read by the
      emote subsystem.

---

## 8. Configuration surface

| Setting | Default (suggested) | Meaning |
|---|---|---|
| Engine baseline emotes | `smile`, `nod`, `wave`, `bow`, `grin`, `shrug`, `laugh` | The set that exists with no packs loaded |
| Pack emote filename glob | `emotes/*.yaml` under pack root | Loader glob |
| Substitution token syntax | `$n`, `$N`, `$s`, `$S`, `$m`, `$M` (Diku-derived) | Token grammar |
| Freeform verb name | `emote` | Canonical name for the freeform form |
| Default pronoun set | `they/them` | Used when a character has no pronoun set assigned |
| Pronoun-form fallback | subjective form | Used when a pronoun set is missing a specific form |
| Target receives separate view | `true` | If `false`, target sees the room view instead |
| Self-target allowed | `true` | Whether `smile self` resolves |

---

## 9. Open questions

- **Item targeting.** Can a player `smile` at an item? The
  keyword resolver makes it cheap to allow; the substitution
  layer needs to know an item's "name" and pronoun set
  (probably `it/its`). Lean: allow it in v1, ship reasonable
  defaults. Pin during impl.
- **Mob targeting.** Same question for mobs. Lean: yes.
  Aggressive mobs probably shouldn't react to a `wave` —
  but reaction is an AI concern, not this spec's. Lean:
  emote succeeds; mob ignores. Pin during impl.
- **Ignore interaction.** When ignore lands, does the
  ignorer see the ignored player's room-view emotes? Pin
  when ignore lands; not this spec's call.
- **Pronoun set source.** v1 may not have a character
  creation step that asks for pronouns. Default
  `they/them` for everyone in v1; add a `pronouns` verb
  in M13.7 polish OR push to character-creation work.
- **Freeform punctuation.** `emote 's cape billows`
  expects the actor's name + `'s cape billows` → `Alice's
  cape billows`. Some MUDs strip the leading space when
  the text starts with `'`. Lean: simple
  space-after-name with no special handling; players
  learn to type `emote ,` (comma start) or punctuation-
  starting freeform forms work as space-prepended. Pin.
- **Two-target emotes.** `introduce alice to bob`. Out of
  v1 scope; the template / dispatch model supports it
  with `$N` plus a `$T` (third party), but no v1 emote
  uses it.
- **Emote rate-limit / flood gate.** Should the existing
  session flood gate (see `session-lifecycle`) catch
  emote spam without a special carve-out? Lean: yes,
  the existing gate is sufficient. No emote-specific
  cooldown in v1.

---

## Cross-references

- `commands-and-dispatch` — verb registration, the
  keyword resolver used for target resolution.
- `world-rooms-movement` — the per-room broadcast path
  emotes use to fan out to room observers.
- `scripting-and-packs` — pack manifest format, content
  globs, dependency ordering, the `emotes/*.yaml` glob
  this spec proposes.
- `session-lifecycle` — flood gate that catches spam
  emote runs.
- `chat-channels-and-tells` — sibling spec; same
  declaration / collision conventions, different
  persistence and addressing model.
- `notifications` — explicitly *not* used by emotes
  (room-scoped, not addressed).
- `docs/themes/social-mud-plan.md` — Theme A live plan.
- `docs/specs/README.md` — spec layer placement and
  cross-cutting indexes.
