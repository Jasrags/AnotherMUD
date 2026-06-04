# Gathering — Feature Specification

**Status:** Draft · **Scope:** The non-vendor ingredient source
crafting wants — two biome-sourced models shipped together:
**ambient forage** (a `forage` verb rolling the room's biome
resource table) and **discrete resource nodes** (placed/spawned
harvestable entities you `harvest`). Covers the gathering
proficiency and quality roll, the scarcity controls (cooldown,
node charges + respawn), permissive access, the events, and
persistence · **Audience:** Anyone reimplementing or porting this
feature in any language.

This document describes *what* gathering must do, not *how*.
Cooldowns, yields, respawn timers, and quality weights live in
the configuration-surface table at §8.

Gathering is the **fifth item in the Gameplay Systems cluster**
(`BACKLOG.md` §2), designed in one pass with
[biomes](biomes.md) (the fourth), which classifies *where*
resources come from. It exists to satisfy a need the crafting
spec already named: `crafting-and-cooking.md` §8 demands
ingredients come from "**gathering nodes, mob drops, and player
farming**", **not** fixed-price vendors, to keep crafting a gold
sink rather than a money printer. Gathering is the **gather-in-
wild** half of crafting's "gather-in-wild / craft-in-town" loop
(`crafting-and-cooking.md` §171).

---

## 1. Overview

Gathering turns the world's biomes into ingredient sources.
v1 ships **both** models (PD-1), each keyed off the room's biome
(`biomes.md` §2):

- **Ambient forage** — a `forage` verb rolls the current room's
  **biome forage table** for an ingredient. No entity; the room's
  biome *is* the source. Best for plentiful low-tier flora
  (herbs, berries, fibers).
- **Resource nodes** — a **harvestable entity** (an ore vein, a
  herb patch, a tree) spawned into biome rooms from the biome's
  **node spawn table**. You `harvest` it; it yields its own table
  and depletes. Best for discrete, visible, tool-gated resources
  (ore, wood).

The division of labor: **biome decides what appears where; the
forage/node table decides what it yields.**

### 1.1 Permissive, scarce (the two crafting rules)

Gathering inherits crafting's economic discipline:

- **Permissive access** (`crafting-and-cooking.md` §1.1): friction
  lowers *yield and quality*, never *availability*. Ambient
  forage is **never refused** where a forage table exists — no
  tool just means a worse roll. (The one allowed refusal is a
  node that hard-requires a tool — §3.3 — which is node-declared,
  not a global gate.)
- **Scarcity, not printing** (`crafting-and-cooking.md` §8):
  gathering is **rate-limited** — a per-character forage cooldown
  and finite node charges with respawn timers — so it consumes
  scarcity instead of minting infinite ingredients.

### 1.2 What gathering is *not*

- **Not a vendor.** It is the deliberate *alternative* to vendor
  sourcing (`crafting-and-cooking.md` §8).
- **Not farming.** Player-planted, time-grown crops are a larger
  deferred system (§9); gathering harvests what the world
  already grows/holds.
- **Not skinning.** Harvesting ingredients from mob corpses
  overlaps `loot-and-corpses.md` and is deferred (§9).

### 1.3 Pre-decisions

| ID | Decision | Status |
|---|---|---|
| PD-1 | Ship **both** models — ambient forage *and* discrete nodes — both keyed off the room's biome. | Decided |
| PD-2 | Gathering uses a single **gathering proficiency** with use-based gain (the existing progression proficiency system); per-resource sub-skills (mining/herbalism) are deferred (§9). | Defaulted |
| PD-3 | Output quality is a **rarity tier** on the yielded item instance, the same quality currency crafting and item-decorations use (`crafting-and-cooking.md` §5, `item-decorations.md`). | Defaulted |
| PD-4 | Node harvest-state (charges, respawn timing) and per-room forage depletion are **transient** — not persisted, respawning fresh on restart, like temporary exits / Tier-1 crafting stations (`crafting-and-cooking.md` §9). | Defaulted |

---

## 2. Ambient forage

The `forage` verb (canonical name in §8) attempts to gather from
the current room's biome.

### 2.1 Resolution

1. Resolve the room's biome (`biomes.md` §2.1). If it has **no
   forage table**, the verb reports "nothing to forage here" and
   stops — this is *absence of a source*, not a refusal (§1.1).
2. Publish the cancellable `resource.gathering` event (source =
   forage). Content may cancel (a protected grove, a quest gate)
   with a generic refusal line.
3. If the room enforces **forage depletion** (§5) and is
   currently depleted, report the depleted message and stop.
4. Check the actor's **forage cooldown** (§5). If still cooling
   down, report the cooldown message and stop.
5. Roll the biome forage table, weighted by the §4 quality
   computation (proficiency + tool), to select an ingredient
   kind, quantity, and rarity tier.
6. Create the ingredient item(s) in the actor's inventory, start
   the cooldown, decrement room richness if depletion is on,
   emit `resource.gathered` (§6), and grant use-based proficiency
   (§4).

### 2.2 Acceptance — forage

- [ ] `forage` in a biome with a forage table yields an
      ingredient sized/qualitied by §4.
- [ ] `forage` in a biome with no forage table reports
      "nothing here" and is not treated as a punishing refusal.
- [ ] A cancelled `resource.gathering` aborts with a generic
      line and no yield.
- [ ] The forage cooldown prevents back-to-back foraging within
      the configured window.
- [ ] No forage attempt is ever refused for lack of a tool — a
      tool only improves the roll.

---

## 3. Resource nodes

A **resource node** is a harvestable entity living in a room.

### 3.1 Node definition and spawning

- A node template declares a **yield table** (ingredient kinds /
  quantities / tier weights), a **charge count** (how many
  harvests before it is exhausted), an optional **required
  tool** (§3.3), and a **respawn interval**.
- Nodes are spawned into rooms from the room **biome's node spawn
  table** (`biomes.md` §2, §5), reusing the area/spawn scheduler
  machinery (`mobs-ai-spawning.md` §3) — a node is, mechanically,
  a spawned non-mob entity with a respawn timer. A depleted node
  is removed and re-spawned after its interval, exactly as mob
  respawn works.

### 3.2 Harvesting

The `harvest` verb (canonical in §8) targets a node in the room:

1. Resolve the node by keyword (the standard target resolver,
   `commands-and-dispatch.md` §5; nodes are ordinary room
   entities for resolution).
2. Publish the cancellable `resource.gathering` event (source =
   node).
3. Enforce the **tool requirement** (§3.3) if the node declares
   one — the **only** permitted refusal (§1.1).
4. Roll the node's yield table weighted by §4, create the
   ingredient(s) in the actor's inventory, **decrement the node's
   charge count**, emit `resource.gathered`, and grant use-based
   proficiency.
5. When charges reach zero, remove the node, emit
   `node.depleted`, and schedule its respawn (§3.1).

### 3.3 Tool requirement (node-only refusal)

A node MAY declare a required tool kind (a pick for ore, an axe
for timber). Lacking it, `harvest` is **refused** with a
content-defined message. This is the single, deliberate
exception to permissive access (§1.1): a hard resource node is
not the same as field crafting, and "you need a pickaxe to mine"
is a reasonable gate. **Ambient forage never refuses** (§2) — the
exception is scoped to nodes that opt in.

### 3.4 Acceptance — nodes

- [ ] A node spawns into rooms of its biome and is targetable by
      keyword.
- [ ] `harvest` yields the node's table, decrements its charges,
      and emits `resource.gathered`.
- [ ] A node at zero charges is removed, emits `node.depleted`,
      and respawns after its interval.
- [ ] A node with a required tool refuses harvesters who lack it
      (the only allowed refusal); a node without one never
      refuses for tools.

---

## 4. The gathering skill and quality

- A single **gathering proficiency** (PD-2) governs yield and
  quality, gaining through use via the existing progression
  proficiency system (use-based gain, like crafting skill —
  `crafting-and-cooking.md` §2). No new store; it rides the
  proficiency save surface.
- **Quality computation** mirrors crafting's roll
  (`crafting-and-cooking.md` §5): a weighted score from
  proficiency + tool quality + biome/node richness produces a
  rolled **rarity tier** (PD-3) on the yielded item, clamped to
  the source's tier ceiling. Higher skill/tool raises the
  *expected* tier and the *quantity*, never the floor — friction
  lowers quality, it does not block (§1.1).
- **Tool**: an optional held tool improves the roll (and, for a
  hard node, may be required — §3.3). No tool = a worse roll for
  forage; a worse-or-refused roll for a tool-gated node.

### 4.1 Acceptance — skill/quality

- [ ] Raising gathering proficiency or tool quality raises
      expected yield tier and quantity.
- [ ] Yielded items carry a rarity-tier property like crafted
      items.
- [ ] A gather attempt grants use-based proficiency on success.
- [ ] A source's tier ceiling hard-caps the rolled tier
      regardless of skill/tool.

---

## 5. Scarcity controls (economy guardrail)

Gathering must "consume scarcity and sink/limit", not print
(`crafting-and-cooking.md` §8). Two rate limits enforce it:

- **Forage cooldown** — a per-character cooldown after a
  successful `forage`, so foraging is not every-tick spam. This
  is the primary forage limiter.
- **Optional forage depletion** — a room may carry a forage
  **richness** that decrements per harvest and regenerates on a
  tick; when depleted, the room temporarily yields nothing. Off
  by default (cooldown alone suffices for v1); content opts in
  for high-traffic rooms.
- **Node charges + respawn** — nodes are naturally finite
  (charges) and time-gated (respawn), so node scarcity needs no
  separate cooldown.

These satisfy the §8 guardrail without violating §1.1:
availability is **rate-limited**, never **denied** — you wait,
you do not get refused.

### 5.1 Acceptance — scarcity

- [ ] Foraging is bounded by the per-character cooldown.
- [ ] With depletion enabled, a heavily-foraged room yields
      nothing until it regenerates.
- [ ] A node's total output before respawn equals its charge
      count.
- [ ] No scarcity control is expressed as a hard refusal that
      could instead be a wait (per §1.1 / `crafting §8`).

---

## 6. Observable events

| Event | Fields | When | Cancellable |
|---|---|---|---|
| `resource.gathering` | actor, room, source (forage / node), biome, node? | before a forage/harvest resolves | **yes** |
| `resource.gathered` | actor, room, source, biome, items, tiers | a forage/harvest yielded items | no |
| `node.depleted` | room, node, biome | a node's last charge was consumed | no |

- `resource.gathering` follows the thin-substrate cancellable
  pattern (mirrors `recall.before` / `concealment.before`):
  content forbids gathering in protected/quest-gated spots by
  subscribing and cancelling.
- `resource.gathered` is the natural **quest** hook ("gather 10
  silverleaf") — `quests.md` advance-on-event, the same role
  `exit.discovered` plays for hidden exits.
- Node *spawn* reuses the spawn feature's events
  (`mobs-ai-spawning.md`); this feature adds only `node.depleted`.

### 6.1 Acceptance — observability

- [ ] Each table event fires with the documented payload.
- [ ] A cancelled `resource.gathering` produces no
      `resource.gathered`.
- [ ] `resource.gathered` carries the yielded items and their
      tiers for quest/economy consumers.

---

## 7. Persistence

- **Gathering proficiency** persists through the existing
  progression proficiency save surface (PD-2) — no new store.
- **Yielded item quality** persists as the item instance's
  rarity-tier property (PD-3), like any crafted/decorated item.
- **Node harvest-state** (charges remaining, respawn timing) and
  **per-room forage depletion** are **transient** (PD-4) — not
  persisted; nodes respawn fresh and rooms reset to full richness
  on restart, matching temporary exits, mob spawn tracking, and
  Tier-1 crafting stations (`crafting-and-cooking.md` §9). The
  README "NOT persisted" list gains node/forage state.

### 7.1 Acceptance — persistence

- [ ] No new player-save *gathering* field beyond the proficiency
      already saved by progression.
- [ ] After a restart, nodes are present at full charges and
      rooms at full forage richness (no persisted depletion).

---

## 8. Configuration surface

| Setting | Default | Meaning |
|---|---|---|
| `forage` / `harvest` verb names | `forage` / `harvest` | Canonical verbs; aliases (`gather`) are policy. |
| Forage cooldown | configured window | §5 per-character forage limiter. |
| Forage depletion | off | §5 optional per-room richness limiter. |
| Forage richness regen | configured rate | §5 how fast a depleted room recovers. |
| Node charge count | per-node | §3.1 harvests before exhaustion. |
| Node respawn interval | per-node | §3.1 time to respawn after depletion. |
| Quality-roll weights | per `crafting-and-cooking.md` §5 | §4 skill/tool/richness → tier. |
| Source tier ceiling | per forage table / node | §4 hard cap on rolled tier. |
| Tool-required nodes | content-declared | §3.3 the only permitted refusal. |
| Nothing-here / cooldown / depleted / no-tool messages | policy strings | actor-facing wording. |

No ingredient, biome, or node *names* are hardcoded; all are
content.

---

## 9. Open questions / future work

- **Gathering sub-skills.** v1 is one `gathering` proficiency
  (PD-2). Splitting into mining / herbalism / forestry / skinning
  (each its own proficiency and tool) is the obvious depth slice.
- **Skinning corpses.** Harvesting ingredients (hides, scales)
  from mob corpses overlaps `loot-and-corpses.md` — a node-like
  harvest on a corpse entity. Deferred; natural follow-on once
  sub-skills land.
- **Player farming.** Planting and time-growing crops
  (`crafting-and-cooking.md` §8 names it) is a larger system
  (plots, growth ticks, ownership); out of v1 scope.
- **Persisted node state.** v1 nodes are transient (PD-4). If a
  rare, slow-respawn node should survive restart, persisting node
  state is the toggle — deferred like the same question for
  crafting stations.
- **Rich depletion model.** v1 leans on the per-character
  cooldown; per-room richness is optional. A deeper shared-
  depletion economy (a forest that can be over-foraged server-
  wide) is a future tuning lever.
- **Gathering interrupts.** Whether taking damage / combat
  cancels an in-progress harvest (a cast-time gather) is a
  combat-interaction call; v1 gathers resolve instantly.

---

## Cross-references

- `biomes` — the coupled fourth cluster item: the room biome's
  forage table (§2) and node spawn table (§3) are *where*
  gathering sources from; biome decides what appears, the
  table decides what it yields (§1).
- `crafting-and-cooking` — the consumer this exists for: §8 the
  non-vendor-source + gold-sink guardrail (§1.1, §5), §1.1
  permissive access, §5 the quality roll gathering mirrors (§4),
  §9 the transient-state precedent (§7), the gather-in-wild /
  craft-in-town loop (§1).
- `mobs-ai-spawning` — §3 the spawn scheduler nodes reuse for
  placement + respawn (§3.1).
- `progression` — the proficiency system gathering skill rides
  (use-based gain, no new store — §4, §7).
- `item-decorations` — the rarity-tier currency yielded items
  carry (PD-3, §4).
- `quests` — `resource.gathered` (§6) is the advance-on-event
  hook for "gather N of X".
- `commands-and-dispatch` — §5 target resolution for `harvest`
  on a node entity (§3.2).
- `persistence` — the proficiency save surface (§7) and the
  transient node/forage state added to the NOT-persisted list.
- `docs/specs/README.md` — reading-order placement (layer 3,
  beside crafting), the events, the tick-handlers (forage-regen /
  node-respawn) and NOT-persisted surfaces.
- `BACKLOG.md` — §2 Gameplay Systems cluster, fifth item;
  designed with biomes (fourth).
