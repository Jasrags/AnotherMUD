# Hidden Exits (Secret Doors) — Feature Specification

**Status:** Draft · **Scope:** Exits concealed until a character
discovers them — both *secret doors* (a hidden exit that also
carries a door) and *secret passages* (a hidden exit with no
door): the `hidden` + `search_difficulty` representation on the
Exit, discovery through the visibility `search` mechanic,
knowledge-gated traversal, per-observer ephemeral discovery
state, exit-list rendering, and the `exit.discovered` event ·
**Audience:** Anyone reimplementing or porting this feature in
any language.

This document describes *what* the hidden-exit surface must do,
not *how*. Verb wording, difficulty numbers, and message policy
live in the configuration-surface table at §8.

Hidden exits are the **second item in the Gameplay Systems
cluster** (`BACKLOG.md` §2), built directly on top of
[visibility](visibility.md). The visibility spec already settled
the discovery *machinery* — the `search` verb
(`visibility.md` §4.4) explicitly "tests concealed exits
alongside concealed entities", and per-observer **sticky
detection memory** (`visibility.md` §4.1) is where a discovered
exit is remembered. This spec is the **exit-side half**: how an
exit declares itself hidden, how discovery gates traversal, and
how the exits line renders. It introduces no new detection model
— it reuses visibility's.

---

## 1. Overview

A hidden exit is a normal directed exit (`world-rooms-movement.md`
§3.2) that is **concealed from a character until that character
discovers it**. Before discovery, the exit is — for that
character — as if it did not exist: not listed, not walkable, its
door not operable. After discovery, it behaves exactly like any
ordinary exit for that character.

Discovery is **per-character and ephemeral** (PD-3): each
character finds it for themselves, the memory lives in
visibility's per-observer detection set, and it clears on leaving
the room, logging out, or a restart. There is no persisted
"found" flag and no save change.

### 1.1 Two shapes, one mechanism

- **Secret door** — a hidden exit that *also* carries a door
  (`world-rooms-movement.md` §5.1). Discovery reveals the exit's
  existence; the door then behaves normally (it may still be
  closed and need opening). Discovery does **not** auto-open the
  door (§8 default).
- **Secret passage** — a hidden exit with *no* door. Discovery
  reveals it and it is immediately walkable.

Both are the same `hidden` exit; the door is orthogonal.

### 1.2 What hidden exits are *not*

- **Not a new detection model.** All "how is it found" lives in
  `visibility.md` §4 (the `search` verb, the perception contest,
  `detect_hidden`/admin auto-pierce, sticky memory). This spec
  only adds the exit as a thing that *can be* a concealment
  instance.
- **Not listing-only flavor (PD-4).** Concealment gates
  **traversal**, not just the exits line — an undiscovered hidden
  exit cannot be walked even if the player types the direction.
  "Concealment you can defeat by being told `go north`" was
  explicitly rejected.
- **Not a move-primitive change.** `world-rooms-movement.md` §3.3
  keeps the move primitive **unconditional**; the knowledge gate
  lives in the player command layer (§4), not the primitive.
- **Not persisted.** No save field; discovery is ephemeral
  (§7).

### 1.3 Pre-decisions

| ID | Decision | Status |
|---|---|---|
| PD-1 | Representation: a `hidden` flag + optional `search_difficulty` on the **Exit** (works with or without a door), mirroring the Door's existing `pickable` / `pick-difficulty` fields. Not an entity tag — exits are edges, not entities, and the entity tag index does not apply to them. | Defaulted — open to revisit |
| PD-2 | Discovery uses the existing visibility `search` mechanic (`visibility.md` §4.4); exits are **search-only by default** (passive auto-detect of exits is off by default — a wall does not move; §3.2). | Defaulted — open to revisit |
| PD-3 | Discovery is **per-character and ephemeral**, stored in visibility's per-observer detection memory (`visibility.md` §4.1); cleared on leaving the room / logout / restart. No persisted "found" set. | Decided |
| PD-4 | Concealment is **knowledge-gated**: an undiscovered hidden exit is non-existent for the character — unlisted *and* unwalkable *and* its door un-operable — not merely hidden from the exits line. | Decided |

---

## 2. Representation

A hidden exit is declared in content on the Exit
(`world-rooms-movement.md` §3.2), which already carries a target
room id, an optional door, an optional display name, and a
condition bag. This feature adds:

- **`hidden`** — a boolean. When set, the exit is concealed until
  discovered.
- **`search_difficulty`** — an optional number: the concealment
  score the searcher's perception contest (`visibility.md` §4.2)
  must beat. Absent → a content default (§8). This is the
  exit-side analog of the Door's `pick-difficulty`.

A hidden exit may independently carry a door; the two are
orthogonal (§1.1). The reverse-direction exit is a *separate*
Exit with its own `hidden` flag, so a passage hidden from one
side but obvious from the other falls out naturally — author each
side's flag independently (§9).

### 2.1 Acceptance — representation

- [ ] An exit with `hidden` unset behaves exactly as today
      (listed, walkable, door operable) — full legacy parity.
- [ ] An exit may be `hidden` with or without a door.
- [ ] `search_difficulty` defaults to the configured value when
      absent.
- [ ] The reverse exit's hidden state is independent of the
      forward exit's.

---

## 3. Discovery

Discovery is a thin wrapper over visibility's detection. A
hidden exit is a **concealment instance** of source type
`hidden-exit`, which is **roll-gated** (`visibility.md` §1.1):
the searcher runs the §4.2 perception contest against the exit's
`search_difficulty`.

### 3.1 Finding via `search`

- The `search` verb (`visibility.md` §4.4) is the primary —
  and, by default (PD-2), the *only* — way to find a hidden
  exit. A `search` runs the perception contest (with the
  active-search bonus) against every hidden exit in the room
  *and* every concealed entity, in one action.
- A successful contest against a hidden exit adds that exit to
  the searcher's **detection set** (`visibility.md` §4.1) and
  emits the discovery message (§5.3) and the `exit.discovered`
  event (§6). A `search` that finds nothing emits the standard
  empty-result line.

### 3.2 Passive detection is off by default

Unlike concealed *entities* — which `visibility.md` §4.1 detects
passively on `look`/entry — hidden *exits* are **not** found by
passive auto-detect by default (PD-2): a character must
deliberately `search`. A config knob (§8) may enable a passive
find chance for high-perception characters, but it is off in v1.

### 3.3 Auto-pierce paths

The visibility counters that auto-pierce roll-based concealment
apply unchanged:

- A `detect_hidden` trait (`visibility.md` §4.3) auto-discovers
  hidden exits (no contest), the same way it auto-pierces hidden
  entities.
- Admin sight / `BypassVisibility` (`visibility.md` §2.1,
  `admin-verbs.md` §3) sees and uses hidden exits without
  searching — admins are never blocked by a secret door.

### 3.4 Ephemeral memory

A discovered exit lives in the per-observer detection set and is
invalidated on the same triggers as any visibility detection
(`visibility.md` §4.1): the character **leaves the room**, logs
out, or the server restarts. A returning character must
re-search. (This is PD-3 and matches traditional secret-door
behavior; the rejected "found stays found" variant is noted in
§9.)

### 3.5 Acceptance — discovery

- [ ] `search` in a room with a hidden exit runs a perception
      contest against its `search_difficulty`; success adds it to
      the actor's detection set.
- [ ] A hidden exit is **not** revealed by passive `look`/entry
      with the v1 default (passive-find off).
- [ ] A `detect_hidden` trait reveals hidden exits with no
      contest.
- [ ] An admin (or any `BypassVisibility` caller) sees and uses a
      hidden exit without searching.
- [ ] Discovery clears when the character leaves the room, logs
      out, or the server restarts.

---

## 4. Traversal gating (knowledge-gated)

PD-4: an undiscovered hidden exit is **non-existent** for the
character — across listing, movement, and door operations.

### 4.1 The gate lives above the move primitive

`world-rooms-movement.md` §3.3 keeps the move primitive
**unconditional** (it does not check cost, locks beyond closed,
or alignment — those are caller concerns). Knowledge-gating is
the same kind of caller concern, and crucially it is
**per-character**, so it MUST NOT live in the shared primitive —
otherwise mob AI, scripted teleports, and admin moves would
inherit a *player's* discovery memory.

The rule:

- **Player-volition callers** — the directional movement command
  and `flee` — filter the room's exits to the **discovered set**
  before acting. A player typing the direction of an
  *undiscovered* hidden exit fails exactly like there is no exit:
  **silently** (`world-rooms-movement.md` §3.3 step 3 is a silent
  failure). `flee` chooses only among exits the fleeing character
  has discovered (you cannot bolt through a door you do not know
  exists).
- **Non-volition callers** — mob AI, scripted/temporary-exit
  moves, admin teleport — are **not** gated; they may use the
  move primitive on a hidden exit directly. (A mob that lairs
  behind a secret door comes and goes freely; whether a given mob
  *should* is `mobs-ai-spawning.md` AI policy, §9.)

### 4.2 Door operations are gated too

The door target resolver (`world-rooms-movement.md` §5.5) for a
**player** command MUST NOT resolve a door on an undiscovered
hidden exit: a player cannot `open <door>` / `pick <door>` for a
secret door they have not found — it resolves to "none" exactly
as a non-existent exit would. Once discovered, the door resolves
and operates normally.

### 4.3 Acceptance — traversal

- [ ] A player typing the direction of an undiscovered hidden
      exit fails silently (no movement, no "no exit" leak beyond
      the standard silent fail).
- [ ] After discovery, the same direction moves the player
      normally (honoring the door if present).
- [ ] `flee` never selects an undiscovered hidden exit.
- [ ] Mob AI / scripted / admin moves through a hidden exit are
      **not** blocked by player discovery state.
- [ ] A player cannot operate (open/close/lock/unlock/pick) the
      door of an undiscovered hidden exit; after discovery, door
      ops work normally.

---

## 5. Rendering and messaging

### 5.1 The exits line filters hidden exits

The room renderer asks the visibility layer which exits an
observer may see — the exit analog of `VisibleEntities`
(`visibility.md` §2): a hidden exit not in the observer's
detection set is omitted from the obvious-exits line. This
extends the visibility filter's "Room render" consumer
(`visibility.md` §5) from entities to exits; it is a single
integration point in the renderer.

- A discovered hidden exit renders **identically to an ordinary
  exit**, including door state (a discovered-but-closed secret
  door shows its closed indicator per existing render policy).
- An undiscovered hidden exit contributes nothing to the line —
  it is indistinguishable from a wall.

### 5.2 GMCP room exits

The GMCP room package's exit list
(`networking-protocols.md`) is filtered with the same
per-observer rule, so a client's mapper does not leak an
undiscovered secret door.

### 5.3 Discovery messaging

- On a successful `search` discovery, the actor sees a
  discovery line naming the direction
  ("You discover a hidden passage leading north."; wording is
  policy, §8). It is **actor-only** — discovery is per-character,
  so other room occupants are not told (they must search
  themselves).
- Whether a discovery also produces a subtle room cue is a §9
  open question; v1 default is silent to others.

### 5.4 Acceptance — rendering

- [ ] An undiscovered hidden exit never appears in the exits
      line or the GMCP exit list for that observer.
- [ ] A discovered hidden exit renders identically to a normal
      exit (door state included).
- [ ] Discovery emits an actor-only message naming the
      direction; other occupants see nothing by default.

---

## 6. Observable events

| Event | Fields | When | Cancellable |
|---|---|---|---|
| `exit.discovered` | actor, room, direction, target_room | a character first discovers a hidden exit | no |

- `exit.discovered` is a post-fact signal content can react to —
  it is the natural hook for a quest objective like "find the
  hidden vault" (`quests.md` advance-on-event). It fires once per
  discovery instance (re-entering and re-searching the same room
  after the memory cleared fires it again — there is no persisted
  "already discovered" suppression, by PD-3).
- The act of searching itself is covered by visibility — this
  feature adds no search event; only the *discovery outcome* on
  an exit.

### 6.1 Acceptance — observability

- [ ] `exit.discovered` fires with the documented payload on a
      successful discovery.
- [ ] No discovery event fires when `search` finds nothing.

---

## 7. Persistence

Nothing in this feature persists (PD-3):

- The `hidden` flag and `search_difficulty` are **content** —
  authored on the exit, loaded with the room like any other room
  data; not session state.
- Discovery memory is **ephemeral**, living in visibility's
  per-observer detection set, which the README "NOT persisted"
  list already covers (`visibility.md` §7).

No player- or account-save field is added; no migration is
needed.

### 7.1 Acceptance — persistence

- [ ] No new field appears on the player or account save.
- [ ] A character who discovers a secret door, logs out, and
      returns must re-search to use it.

---

## 8. Configuration surface

| Setting | Default | Meaning |
|---|---|---|
| Exit `hidden` flag | unset | Marks an exit concealed until discovered. |
| Exit `search_difficulty` | content default | Concealment score for the §3 perception contest; analog of door `pick-difficulty`. |
| Passive exit-find enabled | off | Whether `look`/entry can passively reveal a hidden exit (§3.2); v1 off. |
| Door auto-opens on discovery | no | Whether discovering a secret *door* also opens it (§1.1); v1 leaves it closed. |
| Discovery message | "You discover a hidden passage leading $DIR." | Actor-only line on a successful find (§5.3). |
| Room cue on others' discovery | none | Optional subtle cue to other occupants (§5.3, §9); v1 silent. |

`search` verb wording, the active-search bonus, and the contest
comparator are **visibility's** config (`visibility.md` §8), not
duplicated here.

---

## 9. Open questions / future work

- **Trigger-based reveal.** Classic secret doors sometimes open
  by a lever, a spoken keyword, or a pressure plate rather than a
  perception roll ("pull statue", "say friend"). v1 is
  search-only; a trigger mechanism (an exit referencing a
  room-object/keyword that reveals it) is a future extension and
  would likely lean on the condition bag the Exit already carries
  (`world-rooms-movement.md` §3.2) and/or scripting.
- **Persisted discovery.** The rejected PD-3 alternative
  ("found stays found") could return as an opt-in config: a
  per-character discovered-exit set on the save. It is the one
  place this feature would touch persistence; deferred unless the
  re-search-each-visit feel proves annoying.
- **Asymmetric secret doors.** Independent per-side `hidden`
  flags (§2) already allow "hidden from the dungeon side, obvious
  from the vault side". No extra mechanism is needed; called out
  so authors know it is intentional, not a gap.
- **Mob use of secret doors.** The traversal gate is
  player-only (§4.1), so mobs ignore hidden-ness mechanically.
  Whether a specific mob's AI *should* path through a secret door
  (a guardian that retreats into its lair) is `mobs-ai-spawning.md`
  behavior, not settled here.
- **Group search.** One party member finding a secret door and
  the rest seeing it is deferred to the grouping/party feature
  (`BACKLOG.md` §2), the same way `visibility.md` §9 defers group
  sneak.
- **Passive perception for exits.** §3.2 defaults passive-find
  off; whether a very high perception should occasionally spot a
  secret door on entry is a tuning call left to the config knob.

---

## Cross-references

- `visibility` — the parent feature: §4.4 `search` is the
  discovery mechanic, §4.1 sticky per-observer memory is where a
  discovered exit is remembered, §4.2 is the perception contest,
  §4.3 `detect_hidden`/admin auto-pierce, §2 the filter this
  spec extends from entities to exits, §7 the ephemeral-state
  rule.
- `world-rooms-movement` — §3.2 the Exit this spec annotates,
  §3.3 the unconditional move primitive the gate stays *above*,
  §5.1/§5.5 the door state + target resolver gated in §4.2, §5.6
  temporary exits (a different "dynamic exit" axis, not hidden).
- `commands-and-dispatch` — the player movement command and
  `flee` are the volition callers that filter to the discovered
  set (§4.1); `BypassVisibility` (§5.4) ungates admin traversal.
- `admin-verbs` — §3 visibility bypass: admins are never blocked
  by a secret door (§3.3).
- `networking-protocols` — the GMCP room exit list filtered per
  §5.2.
- `mobs-ai-spawning` — mob AI use of hidden exits is its concern,
  not gated here (§4.1, §9).
- `quests` — `exit.discovered` (§6) is the advance-on-event hook
  for "find the hidden room" objectives.
- `persistence` — unchanged; this feature adds no save state
  (§7).
- `docs/specs/README.md` — reading-order placement (layer 2,
  beside visibility), the `exit.discovered` event, and the
  unchanged NOT-persisted surface.
- `BACKLOG.md` — §2 Gameplay Systems cluster; this is the
  exit-side half visibility §9 handed off.
