# Faction / Standing — Feature Specification

**Status:** Draft · **Scope:** Per-character **standing** with
content-defined **factions** — a signed standing integer per
(character, faction), the ordered named **ranks** that partition
it, the rank tags mirrored for the world index, bounded history,
the cancellable shift pipeline, the `ResolveRanks` gating helper
content consults, the earn/lose loop (quest rewards, kills of
faction mobs), and the persistence + events. **Audience:** Anyone
reimplementing or porting this feature in any language.

This document describes *what* the faction surface must do, not
*how*. Rank ladders, thresholds, on-kill deltas, and message
policy live in the configuration-surface table at §9.

Faction is the **third item in the Gameplay Systems cluster**
(`BACKLOG.md` §2). It is genuinely greenfield — no port
reference — but it is **not designed from scratch**: the engine
already ships a single hardcoded faction in all but name.
**Alignment** (`progression.md` §6) is a signed integer
(−1000..+1000) partitioned into three named buckets mirrored as
tags, with bounded history, a cancellable
`alignment.shift.check` → `alignment.shifted`
→ `alignment.bucket.changed` chain, admin-immune shifts, and a
`ResolveBuckets` helper content gates consult. Faction is **that
same architecture generalized to N named axes** (PD-1).

---

## 1. Overview

A **faction** is a content-defined group a character can stand in
relation to — the City Watch, the Thieves' Guild, a temple, a
clan. A character's **standing** with each faction is a signed
integer; named **ranks** (Hostile … Neutral … Allied) partition
the range. Standing rises and falls through gameplay (completing
the faction's quests, killing its members or its enemies) and
gates content (a guard's hostility, a faction-only shop discount,
a faction-locked quest or area).

### 1.1 Relationship to alignment (PD-1)

Faction is a **parallel sibling** of alignment, not a
replacement and not a coupling:

- It **reuses alignment's architecture** (the model, operations,
  events, tags, history, and gating helper below are deliberate
  generalizations of `progression.md` §6).
- It **does not touch** alignment. Alignment remains the
  morality axis with its existing consumers
  (combat/abilities/rooms/mobs); this feature refactors none of
  them and adds a *new* registry, manager, and player-save field.
- The two **do not interact** in v1. A faction shift does not
  move alignment and vice-versa; disposition consults each
  independently. Cross-effects (a faction that nudges alignment)
  are deferred content/event wiring (§10).

### 1.2 Standing is linear per character (PD-2)

v1 stores exactly **one signed standing per (character,
faction)**. A reputation event shifts exactly the factions it
names — there is **no opposition ripple** (raising City Watch
does not auto-lower Bandits) in v1. Opposition is a pure content
layer that can be added later **without changing stored data**
(§10).

### 1.3 What faction is *not*

- **Not authorization.** Standing gates *gameplay* (hostility,
  prices, access to content), never engine capabilities. Admin
  capability is roles (`roles-and-permissions.md`). The only
  thing faction borrows from roles is the **admin-immune shift**
  alignment already has (§4.4) — not a real dependency (PD-3).
- **Not membership.** v1 models *standing*, a number, not a
  binary "you are a sworn member of the Guild". Membership, if
  wanted, is "standing ≥ a threshold" or a content tag; a
  first-class membership concept is deferred (§10).
- **Not alignment.** See §1.1 — separate axis, separate storage.
- **Not renown.** Per-faction *standing* ("does the Tower like
  me?") is not single-axis *renown* ("how famous am I?"). Renown
  is the separate [reputation](reputation.md) sibling — the two
  share this feature's architecture but no data and no consumers
  in v1 (`reputation.md` §1.1). The source material's loose use of
  "reputation" for both is the only overlap.
- **Not global faction war state.** Standing is per-character.
  Whether two factions are "at war" world-wide is not modeled
  (§10).

### 1.4 Pre-decisions

| ID | Decision | Status |
|---|---|---|
| PD-1 | Faction is a **parallel sibling** that reuses alignment's architecture; alignment is left untouched and the two do not interact in v1. | Decided |
| PD-2 | Standing is **linear per character** — one signed int per (character, faction). No inter-faction opposition ripple in v1 (deferrable as a content layer with no data change). | Decided |
| PD-3 | No dependency on roles beyond the same **admin-immune shift** alignment already has (`progression.md` §6.4 step 2). | Defaulted |
| PD-4 | Each faction declares its own ordered **rank ladder** (names + thresholds), defaulting to a configured shared ladder; standing is **signed** (negative = hostile, positive = allied), mirroring alignment. | Defaulted |
| PD-5 | Standing is **per-character** (an entity property), not account-shared, mirroring alignment. | Defaulted |

---

## 2. The faction definition

A **faction** is a pack-registered definition (a new content
registry; namespaced ids like `tapestry-core:city-watch`,
resolving the same way every other registry does —
`scripting-and-packs.md` §4). It carries:

- **id** — namespaced, unique.
- **display name** and **description** — presentation.
- **rank ladder** — an ordered list of named ranks, each with a
  threshold (the lowest standing at/above which the rank
  applies). Absent → the configured default ladder (§9). Exactly
  one rank applies to any standing value (§3.2).
- **standing bounds** — min/max the integer is clamped to;
  default from config.
- **starting standing** — the value an untouched character is
  treated as having (default from config, typically the Neutral
  floor / zero).

Faction definitions are content, loaded with the pack; they are
never written to a save (§8).

### 2.1 Acceptance — definition

- [ ] A faction registers under a namespaced id and is resolvable
      by content (disposition rules, quests, etc.).
- [ ] A faction without an explicit rank ladder uses the default
      ladder; one with an explicit ladder uses its own.
- [ ] Standing bounds and starting standing default from config
      when the faction omits them.

---

## 3. The standing model

### 3.1 Per-character standing

A character's standing with a faction is a **signed integer**
stored on the entity in a per-faction property bag
(faction id → integer). An untouched (faction not present in the
bag) character is treated as the faction's **starting standing**
(§2). Every write is clamped to the faction's bounds.

### 3.2 Ranks

The faction's ordered ladder maps a standing value to exactly
one **rank** — the highest ladder entry whose threshold the value
meets or exceeds. This generalizes alignment's three fixed
buckets (`progression.md` §6.1) to a content-defined N.

### 3.3 Rank tags

Whenever a character's rank with a faction is established or
changes, the manager mirrors it as a tag on the entity, e.g.
`faction:<factionId>:<rank>`. For a given faction, exactly one
rank tag is present at a time (setting the new one removes the
prior). A character carries at most one rank tag **per faction it
has standing with** — untouched factions contribute no tag.

This mirroring lets the world tag index
(`world-rooms-movement.md` §3.4) drive "all characters Allied
with the City Watch" queries efficiently and lets disposition
rules match on tags as well as numeric ranges — exactly as
alignment's bucket tags do (`progression.md` §6.2).

> **Note — tag cardinality.** With many factions, an entity can
> carry many rank tags. This is bounded by *factions touched*,
> not the registry size, and each is single-valued per faction.
> The unbounded-growth watch in `README.md` applies: see §10.

### 3.4 History

Every successful **shift** (not Set) appends to a per-character
**combined** bounded history (one list, each record carrying
faction id, timestamp, delta, reason, resulting value). A single
combined list — rather than one per faction — bounds total
growth regardless of how many factions a character touches. The
cap is configured (§9); oldest entries drop from the front.
Mirrors alignment history (`progression.md` §6.3), with the
faction id added per record.

### 3.5 Acceptance — model

- [ ] An untouched character reads as the faction's starting
      standing.
- [ ] Standing is clamped to the faction's bounds on every write.
- [ ] The rank is the highest ladder entry whose threshold the
      value meets; exactly one rank tag per touched faction.
- [ ] Setting a new rank tag removes the prior rank tag for that
      faction only.
- [ ] History is a single bounded list across all factions, each
      record carrying its faction id.

---

## 4. Operations

The manager mirrors alignment's operation set
(`progression.md` §6.4), keyed additionally by faction id.

- **Get(entity, faction)** → current standing (the faction's
  starting standing for a missing entry).
- **Rank(entity, faction)** → current rank name AND ensures the
  tag mirror is in sync (idempotent).
- **Set(entity, faction, value, reason)** — the **admin /
  scripted override**: clamps to bounds, writes, updates the rank
  tag, emits **no events**, appends **no history**. Used by admin
  commands, character-creation seeding, and tests.
- **Shift(entity, faction, delta, reason, context?)** — the
  **gameplay path**:
  1. Resolve the entity and the faction. If the faction is not
     registered, no-op with an `info`-level warning (a content
     typo must not silently create a ghost faction).
  2. **Admin bypass.** If the entity carries the `admin` role,
     return immediately — admin characters are faction-immune
     (mirrors `progression.md` §6.4 step 2).
  3. Build a `faction.shift.check` event (entity, factionId,
     reason, suggestedDelta, `cancel: false`, plus context
     fields).
  4. Publish. Listeners may set `cancel: true` or rewrite
     `suggestedDelta` (e.g. a tabard that doubles Watch gains).
  5. If cancelled, return.
  6. Resolve the post-event delta (lenient numeric coercion). If
     zero, return without applying.
  7. Apply (§4.1).

### 4.1 Applying a shift

1. Read the old standing (default = starting).
2. Compute the new value clamped to the faction's bounds.
3. `actualDelta = new − old`. If zero, return.
4. Write the new standing, update the rank tag, append a history
   record (with faction id).
5. Emit `faction.shifted` (entity, factionId, old, new,
   actualDelta, reason, `rankChanged` boolean).
6. If the rank changed, ALSO emit `faction.rank.changed`
   (entity, factionId, old rank, new rank).

### 4.2 Acceptance — operations

- [ ] `Set` clamps, updates the tag, and emits no events / no
      history.
- [ ] `Shift` is a no-op for admin entities and for unregistered
      factions (the latter logs).
- [ ] `faction.shift.check` is cancellable and its delta is
      rewritable by listeners.
- [ ] `faction.shifted` fires whenever standing actually changes;
      `faction.rank.changed` ALSO fires when the rank crosses.

---

## 5. Earning and losing standing

Standing is moved only through `Shift` (gameplay) or `Set`
(admin/seed). The engine wires the common gameplay sources;
content authors attach the rest.

### 5.1 Quest rewards and prerequisites

- A quest reward may **grant** a standing shift with one or more
  named factions on completion (`quests.md` rewards). This is the
  primary intended earn path — faction reputation is mostly a
  quest currency.
- A quest prerequisite may **require** a minimum standing (or
  rank) with a faction to be offered/accepted (§6 gating helper).

### 5.2 Kills of faction mobs

- A mob template may declare **faction membership** (a single
  faction in v1; multiple deferred, §10). When a character lands
  the killing blow on a faction member, the engine shifts the
  killer's standing with that faction by a configured on-kill
  delta (typically negative — killing the Watch lowers Watch
  standing), via `Shift` so the cancellable pipeline and any
  modifiers apply. Kill credit reuses combat's killer
  attribution (`combat.md` §10, the same signal loot-and-corpses
  consumes).
- Opposition (killing a faction's *enemy* **raising** standing)
  is **not** in v1 (PD-2); when opposition lands it rides this
  same on-kill hook through the ripple layer (§10).

### 5.3 Scripted / event sources

Pack scripts and other systems shift standing through the same
`Shift` API; the cancellable `faction.shift.check` lets content
veto or scale any of the above uniformly.

### 5.4 Acceptance — earn/lose

- [ ] A quest completion that grants a faction reward moves the
      character's standing via `Shift` (events fire).
- [ ] Killing a faction-member mob shifts the killer's standing
      with that faction by the configured on-kill delta.
- [ ] All earn paths route through `Shift`, so a
      `faction.shift.check` subscriber can veto/scale any of them.

---

## 6. Content gating and consumers

Faction exposes the generalization of alignment's
`ResolveBuckets` (`progression.md` §6.6):

- **`ResolveRanks(faction, rankNames)`** → a `(min, max)`
  standing range for a set of rank names, baking thresholds at
  call time (so changing a ladder at runtime does not
  retroactively rewrite registered rules — same contract as
  alignment).
- **`MeetsStanding(entity, faction, min)`** → a convenience
  predicate for simple threshold gates.

These are the single seam every consumer goes through. As with
alignment, the **capability** ships here; *which* consumers wire
it in v1 is milestone scope (the BACKLOG/ROADMAP decide), not a
spec mandate.

| Consumer | How it consults faction | Spec |
|---|---|---|
| Mob disposition / aggro | disposition rules gain optional faction clauses (min/max standing or rank set with faction X), beside the existing alignment clauses — a Watch guard is hostile to low-Watch characters, faction members defend their own | `mobs-ai-spawning.md` §5 (alignment-rule parallel) |
| Ability gates | an ability may declare a faction-standing requirement beside its alignment range | `abilities-and-effects.md` §6 (alignment-range parallel) |
| Room / area access | a room may carry a faction-standing access range + block message, parallel to the alignment access range, enforced by the command layer | `world-rooms-movement.md` §3.5 |
| Shops | a shop may gate access or scale pricing by standing (ally discount, refuse hostiles) | `economy-survival.md` §3 |
| Quests | prerequisites require standing; rewards grant it (§5.1) | `quests.md` |

### 6.1 Acceptance — gating

- [ ] `ResolveRanks` returns the correct `(min,max)` for a rank
      set, using thresholds at call time.
- [ ] `MeetsStanding` is true iff the entity's standing ≥ the
      given minimum.
- [ ] A consumer gating on "Friendly+ with faction X" admits a
      character at or above that rank and refuses one below.

---

## 7. Observable events

| Event | Fields | When | Cancellable |
|---|---|---|---|
| `faction.shift.check` | entity, factionId, reason, suggestedDelta, **cancel**, context | before a gameplay shift applies (§4) | **yes** |
| `faction.shifted` | entity, factionId, old, new, actualDelta, reason, rankChanged | a shift changed standing (§4.1) | no |
| `faction.rank.changed` | entity, factionId, oldRank, newRank | a shift crossed a rank boundary (§4.1) | no |

Mirrors alignment's three events (`progression.md` §6.4–§6.5).
`Set` (admin/seed) is silent — quest hooks that watch
`faction.shifted` must not be tripped by an admin override
(same contract as `progression.md` §6 "Alignment Set is
silent").

### 7.1 Acceptance — observability

- [ ] Each table event fires with the documented payload.
- [ ] `Set` emits none of them.
- [ ] A cancelled `faction.shift.check` produces no
      `faction.shifted`.

---

## 8. Persistence

Faction adds **one player-save field**: a per-faction standing
bag (faction id → signed int) plus the combined bounded
`faction_history`. Behind a save-version bump per the standard
migration pattern (`persistence.md` §7); the migration sets the
bag empty on legacy saves (indistinguishable from a fresh
character, who reads every faction at its starting standing).

- Rank **tags** are *derived*, not authoritative — they are
  re-synced from the stored standing on load (the manager's
  `Rank`/`Bucket`-style sync), exactly as alignment re-mirrors
  its bucket tag. They are not separately persisted.
- Faction **definitions** are content (the registry), loaded
  with packs; never saved.
- Standing is **per-character** (PD-5); there is no
  account-shared faction balance in v1 (§10).

### 8.1 Acceptance — persistence

- [ ] A fresh character has an empty standing bag and reads every
      faction at its starting standing.
- [ ] After a shift, the bag holds the faction's clamped value
      and round-trips through save/load.
- [ ] Rank tags are reconstructed on load from stored standing,
      not read from the save as authoritative.
- [ ] The save-version migration from the prior version is a
      no-op on content (empty bag) and round-trips.

---

## 9. Configuration surface

| Setting | Default | Meaning |
|---|---|---|
| Default rank ladder (names + thresholds) | a shared signed ladder (e.g. Hostile / Unfriendly / Neutral / Friendly / Honored / Allied) | §2 fallback for factions without their own ladder. |
| Default standing bounds (min/max) | config (e.g. −1000 / +1000) | §2 clamp when a faction omits bounds. |
| Default starting standing | config (e.g. the Neutral floor / 0) | §2/§3.1 value for an untouched character. |
| Combined history capacity | config | §3.4 cap on `faction_history`. |
| On-kill standing delta | per-faction or config default | §5.2 shift when a faction member is killed. |
| Faction-immune roles | `admin` | §4 shift bypass. |
| Standing bag save-field name | `faction_standing` | §8 YAML key (`omitempty`). |
| Rank tag format | `faction:<id>:<rank>` | §3.3 tag mirror. |
| Access / refusal / price messages | policy strings | §6 consumer wording. |

All text above is policy the renderer or pack may override.

---

## 10. Open questions / future work

- **Opposition / relationship ripple.** The rejected PD-2
  alternative: a content-declared relationship table so a shift
  to faction A ripples a weighted shift to related factions
  B/C. It rides the same `Shift` pipeline (§5.2) and needs **no
  stored-data change** — only ripple resolution + a loop/runaway
  guard. The natural next slice if factional choice should feel
  zero-sum.
- **Unify alignment into faction.** The rejected PD-1 option:
  make alignment "faction #0". Elegant but rewrites a shipped
  system with live consumers + a save migration. Deferred.
- **Alignment ↔ faction coupling.** A faction declaring an
  alignment link (standing shifts nudge alignment; disposition
  blends both). Deferred; achievable today via a
  `faction.shifted` subscriber that calls `alignment.Shift`.
- **First-class membership.** A binary "sworn member" distinct
  from numeric standing (oaths, guild ranks with privileges).
  v1 models standing only (§1.3); membership = standing ≥
  threshold or a content tag for now.
- **Multiple faction membership per mob.** v1 mobs belong to one
  faction (§5.2); a mob in several factions is deferred.
- **Standing decay.** Reputation drifting toward neutral over
  in-game time. Not in v1; would be a tick handler reading the
  game clock.
- **Account-shared standing.** Per-character today (PD-5); an
  account-wide faction balance across alts is a future call,
  paralleling the same question banking raises (`BACKLOG.md`
  §2).
- **Tag-count growth.** Many touched factions = many rank tags
  per entity (§3.3). If this pressures the tag index, a cap or a
  "material factions only" tagging policy is the lever; tracked
  with the README unbounded-growth watch.
- **Faction-vs-faction world war state.** A global hostility
  matrix independent of any character's standing. Out of scope.

---

## Cross-references

- `progression` — §6 alignment is the architectural template
  this feature generalizes (model, operations, events, bucket
  tags, history, `ResolveBuckets`); the two stay independent
  (§1.1).
- `mobs-ai-spawning` — §5 disposition gains faction clauses
  beside alignment ones (§6); mob faction membership drives
  on-kill shifts (§5.2).
- `combat` — §10 killer attribution is the signal the on-kill
  standing shift consumes (§5.2).
- `quests` — the primary earn/gate path: rewards grant standing,
  prerequisites require it (§5.1).
- `abilities-and-effects` — §6 ability gates may require faction
  standing beside an alignment range (§6).
- `world-rooms-movement` — §3.5 room access gains a faction
  range parallel to the alignment one (§6); §3.4 tag index
  consumes the rank tags (§3.3).
- `economy-survival` — §3 shops may gate/price by standing (§6).
- `roles-and-permissions` — the admin-immune shift is the only
  borrowing (§1.3, PD-3); standing is not authorization.
- `persistence` — §4 player serialization (the standing bag added
  by §8) and §7 versioning/migration.
- `scripting-and-packs` — §4 the new namespaced Faction registry
  (§2); scripts shift standing through the same API (§5.3).
- `docs/specs/README.md` — reading-order placement (layer 2,
  beside progression), the registry table (Faction), the
  cancellable-events table (`faction.shift.check`), and the
  player-save surface (§8).
- `BACKLOG.md` — §2 Gameplay Systems cluster, third item.
