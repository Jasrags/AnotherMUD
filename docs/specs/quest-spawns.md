# Quest-Scoped Spawns — Feature Specification

**Status:** Draft (design ahead of code) · **Scope:** Runtime creation
of a quest's mobs and items when a player reaches the stage that needs
them, instead of pre-placing them in the world at boot; per-player
ownership of the spawned entities; and their cleanup when the quest
ends · **Audience:** Anyone reimplementing or porting this feature in
any language.

This document describes *what* quest-scoped spawning must do, not *how*
to implement it. Concrete tag/property key names, the per-stage spawn
cap, and re-spawn policy live in the configuration-surface table at §9.

Quests today place their props the same way any room does: a `mobs:` /
`items:` list on a room, resolved at boot. That means a run's targets —
the paydata chip, the gangers stripping the courier's body — sit in the
world whether or not anyone is on the run, are grabbable by players who
aren't, and never regenerate. Quest-scoped spawns move that content out
of the static world and into the **quest's own lifecycle**: it appears
when a player reaches the stage that calls for it, is owned by that
player, and is removed when their quest ends.

**Phase 1** was *shared-world, per-player-owned*: spawned entities went
into the real room visible to everyone, each player's stage activation
creating its **own** set, tagged to them for cleanup. **Phase 2 (LANDED)**
adds *per-observer visibility*: a spawned entity is stamped with its
owning player's id and gated so **only its owner sees or targets it** —
the shared-world visual duplication and grab-the-wrong-chip edge are gone
(§4.2, §10). The rest of the model (ownership, cleanup, re-derivation) is
unchanged.

---

## 1. Overview

A quest stage may declare **spawns**: mob and item templates to
instantiate into named rooms when that stage becomes a player's active
stage. The spawned entities:

- are created in the **shared world** (the real room; everyone present
  sees them — no per-observer gating in Phase 1);
- are **owned** by the (player, quest, stage) that triggered them, via a
  marker the engine records on each spawned entity (§4);
- **advance objectives normally** — the existing watcher (`quests` §7)
  credits `kill` / `collect` / `visit` off the same bus events
  regardless of how the entity entered the room, so this feature adds
  **no** advancement logic (§6);
- are **cleaned up** when the quest completes or is abandoned, and are
  **re-derived** rather than persisted across reboots (§5, §7, §8).

### 1.1 Goals

- Content that only exists while a player is actually running the quest
  that needs it — no boot-time litter, no props for non-questers.
- A declarative per-stage spawn block authors write alongside the
  objectives it feeds, in the quest file.
- Correct solo and small-multiplayer gameplay: each player's run has its
  own targets, so two runners never contend for a single objective item.
- Reuse of the existing spawn/despawn primitives (the same ones the
  static room lists, corpses, and mounts/hirelings already use) and the
  existing quest lifecycle events as triggers — no new advancement path.

### 1.2 Non-goals

- **Party-shared visibility.** Phase 2 gates a spawn to its single
  owning player; a party-mate does not see another member's spawns (they
  each spawn their own set anyway, §4.1). Sharing a run's spawns across a
  party is deferred with the broader group quest-credit question (§10).
- **Persisting spawned entities.** Spawned mobs/items are transient like
  every other runtime-created entity; they are re-derived from the
  player's active stage on login, never saved (§8).
- **Instanced rooms / dungeons.** Rooms remain shared; only the quest's
  entities are quest-scoped.
- **Dynamic room prose.** A room's description text is static. A dead
  courier's *body* can be a spawned item (a fixture-style item); the
  room's narration is not rewritten per player.

---

## 2. Declaration

Spawns are declared on a **stage**, alongside that stage's objectives.
Each stage carries an optional ordered list of spawn entries:

```
stages:
  - id: job
    description: Put down the scavengers and recover the chip.
    spawns:
      - { kind: mob,  template: ganger,       room: avondale, count: 2 }
      - { kind: item, template: paydata-chip, room: avondale }
    objectives:
      - { type: kill,    target: ganger,       count: 2 }
      - { type: collect, target: paydata-chip }
```

Each spawn entry carries:

- **kind** — `mob` or `item`. Required.
- **template** — the mob or item template id to instantiate. Bare ids
  resolve against the quest's pack namespace at load, qualified ids
  cross packs (same rule as objective targets, `quests` §2).
- **room** — the room id to place the entity in. Bare/qualified as
  above. Optional; when omitted it defaults to the room named by the
  stage's first objective target of a compatible type (config, §9).
- **count** — how many to spawn. Optional; defaults to one.

Rules:

- A stage with no `spawns` block spawns nothing (the common case;
  fully backward-compatible with existing quests).
- Spawn declarations are **independent of objective counts**. The author
  keeps them consistent (spawn as many gangers as the `kill` objective
  needs). A mismatch is legal but is an authoring smell, surfaced by the
  world-doc health audit rather than rejected at load (§9, open in §10).
- A spawn `template` / `room` that fails to resolve is a **load-time
  error** (unlike a spawn-time miss on static content) — the quest names
  concrete content it owns, so a typo should fail the pack, not the run.
- The per-stage spawn count (summed over entries × counts) is bounded by
  a configured cap (§9) as a runaway guard.

### Acceptance — declaration

- [ ] A stage may carry an optional ordered `spawns` list of
      `{kind, template, room?, count?}` entries.
- [ ] `kind` is `mob` or `item`; anything else is a load-time error.
- [ ] `template` and `room` ids namespace-qualify exactly like objective
      targets; an unresolvable id is a load-time error.
- [ ] `room` defaults to the stage's first compatible objective target
      room when omitted.
- [ ] `count` defaults to one; a stage's total spawn count over the cap
      is a load-time error.
- [ ] A quest with no `spawns` blocks behaves exactly as before.

---

## 3. Spawn trigger — stage activation

A stage's spawns fire when that stage becomes the player's **active
stage**. Stage activation happens at three moments:

1. **On accept** — the quest's first stage activates the instant the
   quest is accepted (`quests` §3.1).
2. **On stage advance** — a later stage activates when the previous
   stage's objectives complete and progression advances the stage
   (`quests` §4.2).
3. **On login re-derivation** — when a player with an in-progress quest
   logs in, the quest's currently-active stage is re-activated so its
   spawns are recreated (§7).

At each activation the engine spawns every entry in the newly-active
stage's `spawns` list into its target room, recording ownership (§4).
Activation is **idempotent** (§4): re-firing an already-active stage
(e.g. a redundant event, or a login when the spawns already exist) does
not duplicate them.

The stage-triggered model means the props appear exactly when the run
reaches them: a stage-1 "reach Avondale" objective completes on arrival,
which activates stage 2, which spawns the courier's body and the
scavengers **as the player walks in** — not while they are still at the
Downtown meet.

### Acceptance — trigger

- [ ] Accepting a quest spawns its first stage's declared content.
- [ ] Advancing into a later stage spawns that stage's declared content.
- [ ] Re-firing activation for an already-active, already-spawned stage
      does not create duplicates.
- [ ] A stage's spawns land in their declared (or defaulted) rooms with
      the declared counts.

---

## 4. Ownership and idempotency

Every spawned entity records an **owner reference** identifying the
(player, quest, stage) that created it (the concrete marker — a tag plus
a source-scoped key — is config, §9). Ownership serves two purposes:

- **Cleanup accounting** (§5): the engine can enumerate and remove
  exactly the entities a given player's quest created.
- **Idempotency**: before spawning a stage's content the engine checks
  whether this (player, quest, stage) already owns live spawns; if so it
  does not spawn again.

Ownership **drives** visibility (Phase 2). The owner marker is the same
reference the per-observer visibility gate reads: a spawned entity is
visible and targetable **only** to its owning player (and a bypassing
admin inspection verb). The `collect`/`kill` watcher still credits the
acting player by template match (`quests` §7); with the gate a runner can
only ever interact with their **own** owned instance, so crediting is
unambiguous.

### 4.1 Multiplayer in the shared world

Because each player's stage activation creates its **own** owned set,
two players both on stage 2 in the same room each have their own chip and
ganger pair placed into the shared room. Phase 2 gates each set to its
owner, so a runner sees and can target **only** their own — the props
occupy the same room but do not exist for the other runner. This gives
both **mechanical** correctness (no contention over a single objective
item) and **visual** cleanliness (no duplicate props). The gate is an
existence gate (`visibility` §1.2 exception): it fails closed, so a spawn
never leaks to a non-owner.

### Acceptance — ownership

- [x] Each spawned entity carries an owner reference to its (player,
      quest, stage).
- [x] A stage does not double-spawn for a player who already owns its
      live spawns.
- [x] Objective crediting is by template match for the acting player and
      is unaffected by which owned instance they interact with.

### Acceptance — per-observer visibility (Phase 2)

- [x] A spawned entity is visible and targetable only to its owning
      player; a non-owner in the same room neither sees it in the room
      render nor can resolve it as a command target.
- [x] The owner sees and can interact with their own spawn normally
      (self is never gated out).
- [x] Two runners on the same stage in the same room each see exactly
      their own spawn set — never the other's.
- [ ] A bypassing admin inspection verb still reaches a foreign spawn.
      The visibility primitive honors `Bypass` (unit-tested), but no
      production verb constructs a bypassing observer for quest spawns
      yet, so this is primitive-only — see §10 "Admin bypass".

---

## 5. Cleanup

A player's quest spawns are removed when the quest **leaves the active
set** for that player:

- **On completion** (`quests` §4.3 / §4.3a turn-in) — the run is done;
  its spawns (including any spawned item the player is still carrying —
  the courier's chip is "handed over") are removed.
- **On abandonment** (`quests` §4.5) — same removal.
- **On the owning player going offline** (logout / link-dead sweep) —
  the player's live quest spawns are removed so the shared world does
  not accumulate orphaned quest content while they are away; they are
  re-derived on the player's next login (§7).

Removal targets the player's **surviving** owned entities wherever they
are — on a room floor, in a container, or in the owner's inventory. It
does **not** resurrect or claw back consequences that already resolved:
a spawned mob the player already killed is gone (its corpse and loot
follow the normal corpse lifecycle and are not quest-scoped); XP already
awarded stands.

### 5.1 What is *not* cleaned

- Corpses and loot dropped by killed quest-spawn mobs (normal corpse
  decay owns them).
- Reward items, currency, XP, and standing already granted on
  completion.
- Anything not carrying the quest's owner reference.

### Acceptance — cleanup

- [ ] Completing a quest removes its surviving owned spawns, including a
      collected spawn item still in the player's inventory.
- [ ] Abandoning a quest removes its surviving owned spawns.
- [ ] A player going offline removes their live owned spawns; they are
      recreated on next login while the stage is still active (§7).
- [ ] Cleanup never removes corpses/loot of already-killed spawn mobs,
      nor any already-granted reward.
- [ ] Cleanup removes only entities carrying that quest's owner
      reference — never unrelated content in the same room.

---

## 6. Objective interaction

This feature is **additive to** and **orthogonal from** objective
advancement. The watcher (`quests` §7) advances `kill` / `collect` /
`visit` objectives off `mob.killed` / `item.picked_up` / `player.moved`
bus events, matching on template id for the acting player. A spawned
mob or item fires those events identically to a statically-placed one,
so:

- No objective type is added, changed, or made spawn-aware.
- A spawned mob's `kill` and a spawned item's `collect` credit exactly
  as the static versions did (the current run's behavior is preserved
  when its props move from the room lists into a `spawns` block).
- `visit` objectives need no spawns (a room is reached, not created);
  they are the natural stage-1 trigger that activates a stage-2 spawn.

### Acceptance — objectives

- [ ] Killing a spawned mob advances a matching `kill` objective for the
      killer.
- [ ] Picking up a spawned item advances a matching `collect` objective
      for the picker.
- [ ] No change to objective types, matching, or the watcher is required
      by this feature.

---

## 7. Reboot and login re-derivation

Spawned entities are transient (§8). The **quest's active-stage state is
persisted** (`quests` persistence), so on login the engine re-activates
the player's currently-active stage (§3, moment 3), recreating its
spawns. This makes an interrupted run resumable across a server restart
without persisting the entities themselves.

Because re-derivation recreates the **full** stage declaration,
already-credited partial progress can leave a benign surplus — e.g. a
player who killed one of two spawned gangers, then reconnected, faces
two freshly-spawned gangers while their `kill` objective still reads
1/2. The extra kill is harmless (more XP); tightening this to spawn only
the shortfall is an open question (§10).

### Acceptance — re-derivation

- [ ] A player who logs in mid-quest has their active stage's spawns
      recreated.
- [ ] Re-derivation is idempotent with any spawns that already exist for
      that (player, quest, stage).
- [ ] Persisted quest state is sufficient to re-derive spawns; no
      spawned entity is read from a save.

---

## 8. Persistence

**Quest spawns do not persist.** No spawned mob or item is written to or
read from `saves/`. The save surface is unchanged: the player's quest
progress (active stage, objective counts) already persists, and that is
the sole input to login re-derivation (§7). This matches how mobs,
corpses, temporary portals, mounts, and hirelings are all handled.

### Acceptance — persistence

- [ ] No file under `saves/` gains a quest-spawn field.
- [ ] Spawned entities are absent from the player and world saves.

---

## 9. Configuration surface

| Setting | Default | Meaning |
|---|---|---|
| `spawns` block | absent | Optional per-stage list of spawn entries; absent = no spawns. |
| Spawn entry fields | `kind`, `template`, `room?`, `count?` | `kind` ∈ {mob,item}; ids namespace-qualified; `room` defaults to the stage's first compatible objective target; `count` defaults to 1. |
| Owner marker | a `quest_spawn` tag + an owner key of (player, quest, stage) | How a spawned entity is linked to its origin for idempotency + cleanup (§4). |
| Per-stage spawn cap | a small fixed maximum | Runaway guard; a stage whose total spawn count exceeds it is a load-time error (§2). |
| Cleanup-on-logout | on | Whether an offline player's live spawns are removed (and re-derived on login). Off = spawns linger while offline. |
| Re-derive-on-login | on | Whether login recreates the active stage's spawns (§7). |
| Load-time id validation | strict | Unresolvable `template`/`room` fails the pack (§2), unlike fail-silent static spawn. |

---

## 10. Open questions

- **Per-observer visibility (Phase 2) — LANDED.** A spawned entity is
  gated so only its owner sees and interacts with it, removing the
  shared-world visual duplication and the grab-the-wrong-chip edge
  (§4.1). Implemented by stamping the owning player's id on the entity
  (`questspawn.OwnerProperty` + a `quest_spawn` tag) and adding a
  `SourceQuestSpawn` existence-gate layer to the per-observer
  `visibility` predicate (fails closed, §1.2 exception) plus a matching
  render-side filter so "what you see" and "what you can target" agree.
  **Party sharing** — should a party-mate see a member's spawns? — is
  still open, below.
- **Partial-progress re-derivation.** Should login re-spawn only the
  *shortfall* (one ganger, not two) to match already-credited progress,
  rather than the full stage declaration (§7)? Phase 1 spawns the full
  set for simplicity.
- **Party sharing.** When grouping is active, should one member's quest
  spawns satisfy the whole party, or does each member spawn their own?
  Interacts with group quest-credit (`grouping` open questions).
- **Gate coverage beyond the render/target seam.** Phase 2 enforces the
  owner gate on the shared render list and the generic target resolver
  (`ArgEntity` → `CanSee`). Feature-specific room scans that resolve a
  *secondary role* directly off room placement (a harvest node, shop NPC,
  mount, auctioneer, campfire, corpse) do not consult the gate. Latent
  today — no content stamps a quest spawn with a secondary interactable
  role — but a pack that did would let a non-owner interact with a spawn
  that is invisible to them. Harden (route those scans through the
  predicate) if a quest spawn ever gains such a role.
- **Admin bypass.** The visibility primitive honors a `Bypass` observer
  (proven in unit tests), but no production verb yet constructs a
  bypassing observer for quest spawns, so an admin cannot currently
  see/inspect another player's foreign spawn. Wire an explicit admin
  bypass if debugging foreign spawns becomes necessary.
- **Set-dressing props.** A pure-flavor spawn (a lootless "courier's
  body" object) is expressible as an item spawn today. Whether such
  props deserve a distinct non-interactable `kind` (so they can't be
  picked up or mistaken for loot) is deferred.
- **Objective/spawn consistency.** A `spawns` count that disagrees with
  the matching objective `count` is currently an authoring smell flagged
  by the health audit, not a load error. Promote to a load-time check?
- **Timed / respawn behavior.** Phase 1 spawns once per stage
  activation. A stage that should keep respawning a target until an
  objective is met (a wave) is out of scope; revisit if a run needs it.

---

## Cross-references

- `quests` — the lifecycle this hooks: acceptance (§3.1), stage advance
  (§4.2), completion / turn-in (§4.3/§4.3a), abandonment (§4.5), the
  auto-tracking watcher (§7), and the `EventSink` lifecycle events
  (Started / StageAdvanced / Completed / Abandoned) that are the spawn /
  cleanup triggers.
- `mobs-ai-spawning` — the mob template + spawn primitive reused to
  instantiate a spawned mob; disposition/AI apply unchanged.
- `inventory-equipment-items` — item instantiation + placement for
  spawned items; the pickup that advances `collect`.
- `world-rooms-movement` — the per-room entity placement spawns target;
  the `player.moved` event that both advances `visit` and drives the
  stage-1 → stage-2 activation.
- `visibility` — the per-observer predicate Phase 2 extends with the
  `SourceQuestSpawn` existence-gate layer to owner-gate spawns (§4.1,
  §10).
- `session-lifecycle` — logout / link-dead sweep that triggers offline
  cleanup, and the login that re-derives spawns.
- `loot-and-corpses` — the normal corpse/loot lifecycle that owns a
  killed spawn-mob's remains (explicitly *not* quest-scoped).
- `docs/specs/README.md` — spec layer placement and cross-cutting
  indexes.
