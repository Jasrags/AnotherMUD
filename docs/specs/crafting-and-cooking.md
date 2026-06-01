# Crafting & Cooking — Feature Specification

**Status:** Draft · **Scope:** The crafting system — skills, recipes,
crafting stations, the tiered quality roll, recipe acquisition — and
cooking as its food-producing specialization that feeds the existing
sustenance pool and grants quality-scaled "well-fed" effects · **Audience:**
Anyone reimplementing or porting this feature in any language.

This document describes *what* crafting and cooking must do, not *how* to
implement it. All numeric weights, tier counts, durations, and rates live
in the configuration-surface table at §10.

Adapted from a design brief written for an undecided codebase; reshaped
for AnotherMUD's actual systems. It is **setting-agnostic**: the engine
provides the regional-recipe *mechanism*; the regions and cultures that
fill it (a Wheel-of-Time world, or any other) are pack content, not part
of this spec — consistent with this project's behavior-only specs and
placeholder content namespace.

This spec **leans on systems that already exist**: output quality is a
*rarity tier* (`item-decorations.md`); crafting skill is a *proficiency*
on a progression *track* (`progression.md`); the well-fed buff is an
*effect* (`abilities-and-effects.md` + the consumable effect pipeline in
`economy-survival.md`); cooking feeds the *sustenance pool*
(`economy-survival.md` §3). Two pieces are genuinely new substrate —
**crafting stations** (§4) and **gathering/resource sources** (§8) — and
are flagged as such.

---

## 1. Overview

Crafting turns input items into an output item through a **recipe**, gated
by the crafter's **skill**, the **station** they work at, their **tools**,
and the **quality of their ingredients**. Cooking is crafting whose
outputs are food: it clears sustenance and can grant a well-fed effect.

### 1.1 The spine: access is permissive, quality is gated

This single rule resolves the realism-vs-fun tension and governs every
other section:

> A player may *attempt* almost any craft almost anywhere. What is
> restricted is the **quality ceiling** they can *achieve* — by station,
> tool, skill, and ingredients. Crafting never locks a player *out* in the
> field; it caps what they can *make* there.

Wherever a rule could either (a) refuse a craft or (b) lower its
achievable quality, this spec chooses (b). A penniless traveler can always
fry a fish over a campfire; they simply cannot forge a masterwork blade
without a forge.

### 1.2 What crafting is *not*

- Not a gate by recipe ownership. The gate to a *profession* is skill
  (§2); baseline recipes come free with the skill. Recipes restrict
  *breadth* and *ceiling*, never entry.
- Not binary success/fail. Every attempt that has its inputs produces
  *something*; the variable is the output's quality tier (§5).
- Not blind experimentation. Recipe discovery is guided (§7); there is no
  combinatorial "try every pair" loop (which only produces a wiki spoiler
  list).
- Not a closed vendor loop. Ingredients come from scarce sources
  (gathering, drops, farming), not fixed-price vendors; crafting *consumes*
  scarcity and *sinks* gold (§8).

---

## 2. Crafting skills

A crafting discipline (smithing, cooking, weaving, alchemy) is a
**proficiency** on a progression **track** (`progression.md` §3 already
names "crafting" as an anticipated side-track).

- Acquiring a discipline grants its **baseline recipes** immediately (§7).
  The entry gate is skill, never recipe ownership.
- Skill rises through **use** — the existing §3.5 use-based proficiency
  gain: a successful (or even failed) craft rolls a gain, tapering toward
  the proficiency cap, optionally scaled by a gain-stat. No separate XP
  path is invented; crafting reuses the proficiency-and-cap machinery.
- Skill is one of four inputs to the quality roll (§5); it never alone
  determines what a player can attempt.
- **Recommended progression model (open, §11): hybrid** — *recipe-unlock*
  governs breadth (which recipes you know, §7) and *use-based skillup*
  governs depth (how well you execute them, §5). This is the natural fit
  because the engine already has use-based proficiency gain; pure
  recipe-unlock would waste it, and pure use-based would lose the
  acquisition layer that drives the gold sink and travel reward.

**Acceptance — skills**

- [ ] Acquiring a discipline grants its baseline recipes and lets the
      player attempt baseline crafts immediately.
- [ ] A craft attempt rolls a use-based proficiency gain through the
      existing §3.5 path, tapering to the cap.
- [ ] Skill is necessary but not sufficient: a high skill at a low-tier
      station still cannot exceed that station's ceiling (§4).

---

## 3. Recipes

A **recipe** is content (a pack-loaded registry entry, like abilities and
item templates). It declares:

- **Inputs** — the ingredient item kinds and quantities consumed, each
  with an optional minimum/relevant quality (§5).
- **Output** — the item produced (template), and how many.
- **Discipline + skill floor** — which crafting proficiency it uses and
  the minimum skill to attempt it at all (kept low — the *ceiling*, not
  the *floor*, is the main lever per §1.1).
- **Station tier required to attempt** — the minimum station (§4). A
  recipe may be Tier-0-attemptable (poor result anywhere) while its
  *good* results need a higher tier; or it may require at least Tier 1 to
  attempt at all (you cannot smelt ore with no fire).
- **Tool** — the tool kind the recipe uses, if any (§5).
- **Time** — how long the craft occupies the player (a pulse/tick count;
  Tier 0 is fast, higher tiers slower).
- **Acquisition tier** — baseline / common / uncommon / rare / regional
  (§7), which is metadata for how the recipe is obtained, not a runtime
  gate.

Inputs are **consumed atomically**: a craft either reserves and consumes
all inputs and produces the output, or fails cleanly consuming nothing
(the same two-actor/atomic discipline as inventory transfers).

**Acceptance — recipes**

- [ ] A recipe loaded from content declares inputs, output, discipline,
      skill floor, station tier, tool, time, and acquisition tier.
- [ ] A craft consumes all inputs and produces the output atomically, or
      consumes nothing on failure.
- [ ] A recipe the player does not know cannot be crafted (breadth gate),
      but a known recipe is always *attemptable* where its station floor
      is met (§1.1).

---

## 4. Stations and the quality ceiling

**This is new substrate.** A *station* is the work surface a craft is
performed at, and its tier sets the **hard ceiling** on output quality
(§1.1). There is no furniture system today (noted in M11.4); stations are
the first real one.

Three tiers:

- **Tier 0 — anywhere.** No station. Eating, drinking, cold assembly,
  field-dressing game, lashing a splint, basic repair. Frictionless;
  lowest quality ceiling. Always available.
- **Tier 1 — improvised.** A player-built, **temporary placed station**
  (campfire, lean-to, field anvil) — modeled like a temporary placed
  entity (the M15.2 temporary-exit/portal pattern is the closest
  precedent): it is created in the room, consumes fuel/materials and time
  to build, may be **refused by terrain or weather** (no campfire
  underwater or in a downpour — read the room's terrain property and the
  weather, both already in `world-rooms-movement.md` §6), and decays.
  Unlocks real field cooking and light repair — the "on the go" tier.
- **Tier 2 — fixed.** A permanent station entity in a room (forge/anvil,
  full kitchen, alchemist bench, loom), placed by content (towns now;
  player housing later). The only path to top-quality, complex outputs.
  Its friction — being town-bound — is intentional: it makes towns matter
  and creates the **gather-in-wild / craft-in-town** loop.

A station exposes which **disciplines** it serves and its **tier**. A
craft reads the station present in the room (or the actor's own portable
tool, below) to determine the ceiling.

**Portable tools** (a traveling smith's kit) raise the field ceiling **one
tier** for their discipline, at an inventory-weight and high-gold cost —
so a dedicated traveler can approach town quality in the field, but pays
for it. A portable tool is the controlled exception that keeps Tier 2
meaningful.

**Acceptance — stations**

- [ ] A craft's achievable quality is clamped to the present station's
      tier ceiling regardless of skill, tool, or ingredients (§1.1, §5).
- [ ] Tier 0 crafts succeed anywhere with no station.
- [ ] A Tier 1 station is player-built, consumes inputs + time, may be
      refused by terrain/weather, and decays.
- [ ] A Tier 2 station is content-placed and permanent.
- [ ] A portable tool raises the field ceiling exactly one tier for its
      discipline.

---

## 5. The quality roll

Output quality is a **tiered roll** (the rarity tiers of
`item-decorations.md` — e.g. poor / normal / fine / masterwork), never
binary. Four inputs combine; the station is a hard clamp, the rest weight
a roll.

- **Station tier** sets the **hard ceiling** (§4): the roll can never
  produce a tier above what the station allows. This clamp is applied
  last and is absolute.
- **Crafter skill** is the primary weight: higher proficiency shifts the
  roll's center upward (better expected tier) and narrows variance.
- **Tool quality** is a *separate* weight from skill — a master with a
  poor hammer is not a journeyman with a masterwork hammer. Tool quality
  is read from the tool item's quality property.
- **Ingredient/material quality** weights the roll and may also set a
  *soft* ceiling (you cannot make a masterwork stew from rotten meat).
- **RNG** enters through the engine `Roller` (the same single-goroutine
  tick-context roller combat and abilities use), so a craft at fixed
  inputs still varies within its weighted band.

The behavior the formula must satisfy (exact weights → §10):

1. Compute a weighted quality score from skill, tool, and ingredient
   quality.
2. Roll within the band that score defines (RNG).
3. Clamp the rolled tier to the station ceiling (hard) and any ingredient
   soft ceiling.
4. Stamp the output item with the resulting **rarity-tier key** (so the
   item renders its quality through the existing item-decoration path) and
   any quality-derived modifiers the output template defines.

The output's quality tier is an **instance property** that persists with
the item (`item-decorations.md` §5/§6, `inventory-equipment-items.md`
instance properties).

**Acceptance — the quality roll**

- [ ] Identical inputs produce varying output tiers within a band (RNG),
      not a fixed result.
- [ ] Raising skill, tool quality, or ingredient quality raises the
      expected output tier.
- [ ] The station ceiling is absolute: no input combination exceeds it.
- [ ] Tool quality and skill are independent inputs (one cannot fully
      substitute for the other).
- [ ] The output carries its quality as a persisted rarity-tier instance
      property.

---

## 6. Cooking and the sustenance integration

Cooking is crafting whose outputs are **food** consumables. It plugs into
the **existing** sustenance pool and consumable pipeline
(`economy-survival.md` §3, §6) — it does not replace hunger; it enriches
the food that satisfies it.

- A cooked food item is a consumable: eating it **clears sustenance** like
  any food (existing behavior) **and**, when its quality warrants, applies
  a **"well-fed" effect** — a real `EffectTemplate` (the consumable
  `effect_id` pipeline, M14.2), not a new bespoke buff system.
- The well-fed effect's **tier scales with the meal's crafted quality**
  (§5): a poor field-fry gives little or nothing beyond clearing hunger; a
  fine or masterwork cooked meal grants a meaningful-duration stat bonus.
  This is what turns cooking from a chore into a **support profession**.
- **Cold rations (Tier 0)** clear sustenance but grant a weak or no
  well-fed effect; a **cooked meal (Tier 1/2)** is what grants the
  worthwhile buff. The friction of cooking (a fire, ingredients, time) is
  paid back in the buff tier — the incentive to cook rather than graze.
- **Stacking** follows the effect system's rules: re-eating refreshes or
  replaces per the effect's policy (not additive stacking of well-fed on
  well-fed), so a player cannot stack ten meals into a permanent bonus.

**Acceptance — cooking**

- [ ] Eating cooked food clears sustenance exactly as existing food does.
- [ ] A cooked meal applies a well-fed effect whose tier scales with the
      meal's crafted quality; a cold ration applies a weak/no effect.
- [ ] Well-fed does not additively stack; re-eating refreshes/replaces per
      the effect policy.
- [ ] The well-fed effect is a standard `EffectTemplate`, not a bespoke
      buff record.

---

## 7. Recipe acquisition

Recipes are acquired in **layers**, each tied to an existing system, so
that breadth rewards play and travel without ever gating entry (§1.1).

| Tier | Source | AnotherMUD hook |
|---|---|---|
| **Baseline** | known on acquiring the skill | granted with the discipline/track (§2) |
| **Common** | bought from vendors/trainers, availability by skill level | **shops** (`economy-survival.md` §3) + **training** (`progression.md`); the primary **gold sink** |
| **Uncommon** | quest rewards (carry story, slightly better output) | **quests** (`quests.md`) reward grant |
| **Rare** | mob drops, ruins, guided discovery | **mob loot** (`mobs-ai-spawning.md` §6.3), containers |
| **Regional** | learnable only in/from specific regions/peoples | content keyed to **areas/regions** (pack content; the WoT regions are this layer) |

- A learned recipe is **per-character known state** that persists like
  proficiencies/abilities (§9). Learning a known recipe is a no-op.
- **Discovery is guided, never blind.** Rare/discovered recipes are found
  through partial recipe pages, NPC clues, or quest hints that point at
  the missing inputs or station — never through blind combinatorial
  experimentation (which collapses into a spoiler wiki). A discovery hint
  is content (an item or quest fragment), not an engine search.
- **Regional sets make geography reward travel:** a recipe learnable only
  from a particular people or place gives a concrete reason to go there.
  The engine provides "this recipe is learnable only from source X"; X is
  content.

**Acceptance — acquisition**

- [ ] Baseline recipes arrive with the skill; common recipes are
      purchasable with availability tied to skill; uncommon come from
      quests; rare from drops/discovery; regional only from their region.
- [ ] A learned recipe persists across logout; re-learning is a no-op.
- [ ] No recipe is discoverable by blind input permutation — discovery
      requires a content-provided hint.

---

## 8. Economy guardrails

Called out as **anti-patterns to design against**, because crafting's
economic role is to *consume scarcity and sink gold*, not print money.

- **Ingredients must not come only from fixed-price vendors** — that is a
  money printer (buy cheap, craft, sell dear). Prefer **gathering nodes,
  mob drops, and player farming**. ⚠️ **Gathering/resource nodes are new
  substrate** (they overlap the *biomes / foraging* backlog item) — until
  they exist, ingredient sourcing leans on **mob loot** (which exists) and
  authored placement; a vendor-only ingredient is a known temporary
  compromise, not the design.
- **Crafting is a gold sink**, principally through **common-recipe
  purchase** (§7) and station/tool costs, balancing the gold that quests
  and loot inject.
- **Field-crafting must never be punishing.** Permissive access (§1.1) is
  non-negotiable: friction lowers *quality*, never *availability*. A rule
  that would refuse a field craft is wrong; the same rule expressed as a
  quality cap is right.

**Acceptance — economy**

- [ ] No recipe's full ingredient list is satisfiable purely from
      fixed-price vendor stock (flagged in content review).
- [ ] Common-recipe purchase and station/tool costs remove gold from the
      economy.
- [ ] No crafting rule refuses a field attempt that could instead be
      expressed as a quality cap.

---

## 9. Persistence

- **Known recipes** are per-character state, persisted like proficiencies
  and abilities (a list on the player save); restored on load.
- **Crafting proficiency** persists through the existing progression
  proficiency save surface (§2) — no new store.
- **Output quality** persists as the item instance's rarity-tier property
  (§5).
- **Tier 1 stations** are transient placed entities (like temporary
  exits): not persisted across restart by default (they decay), matching
  the "no temporary-exit persistence" stance. **Tier 2 stations** are
  content-placed and re-derived at boot, not saved.
- The **recipe registry** itself is content, loaded at boot; a recipe
  removed from content stops being craftable, and a known-but-now-unknown
  recipe id is ignored, never an error.

**Acceptance — persistence**

- [ ] A learned recipe survives logout; the proficiency it trained
      survives via the existing progression save.
- [ ] A crafted item's quality survives logout.
- [ ] A removed recipe id known by an old save loads cleanly (ignored).

---

## 10. Configuration surface

| Setting | Description |
|---|---|
| Quality tiers | The output tier ladder, shared with `item-decorations.md` rarity tiers (§5). |
| Quality-roll weights | How skill, tool quality, and ingredient quality weight the score; the RNG band width (§5). |
| Station tier ceilings | The max quality tier each station tier permits (§4). |
| Skill floors | Per-recipe minimum skill to attempt (kept low) (§3). |
| Craft times | Per-recipe / per-tier time cost (§3). |
| Tier 1 build cost + decay | Fuel/materials and lifetime of an improvised station (§4). |
| Portable-tool bonus | The one-tier field ceiling raise + its weight/gold cost (§4). |
| Well-fed tiers | Effect tier, stat bonus, and duration per meal quality; cold-ration baseline (§6). |
| Recipe availability curve | How common-recipe shop stock scales with skill (§7). |
| Skillup rate | The §3.5 gain parameters for crafting proficiencies (§2). |

No region, recipe, station, or ingredient *names* are hardcoded by
behavior — all are content. The engine ships the mechanism; packs ship the
world (the WoT regions and their regional recipes are this content).

---

## 11. Open questions

Surfaced with a recommendation and a default chosen so the spec is not
blocked; flagged for sign-off.

- **Load-bearing vs. flavor (recommend: cooking semi-load-bearing now,
  crafting semi-load-bearing and growing).** Because sustenance is real,
  cooking has immediate weight (the well-fed buff makes it a support
  profession, not a chore). Crafting gear can start semi-load-bearing
  (field repair, mid-tier gear) and grow toward the gear meta as Tier 2
  content lands. Tradeoff: fully load-bearing crafting raises the stakes of
  the greenfield dependencies (stations, gathering) — so grow into it.
- **Skill progression (recommend: hybrid — recipe-unlock breadth +
  use-based skillup depth).** Fits the existing proficiency gain (§2);
  decided as the working default.
- **Discovery depth (recommend: mostly purchasable certainty + guided rare
  discovery).** Common recipes are reliably bought (the gold sink); only
  rare/regional recipes require guided discovery, so the wiki-spoiler
  failure mode is avoided and certainty is the norm.
- **Greenfield dependency — stations/furniture (§4).** Tier 1/2 stations
  are the first furniture system. MVP can ship Tier 0 + a content-placed
  Tier 2 (room-tagged station, reusing room tags/properties) and **defer
  Tier 1 improvised stations** (which need the temporary-placed-entity
  build/decay machinery) to a follow-on. The plan's MVP cut reflects this.
- **Greenfield dependency — gathering/resource nodes (§8).** Overlaps the
  *biomes / foraging* backlog item. MVP sources ingredients from **mob
  loot + authored placement**; real gathering nodes are a follow-on (ideal
  to design together with biomes).
- **Setting content prerequisite.** The regional-recipe *mechanism* is in
  this spec; the *regions* are content and **no geography reference exists
  in this repo yet**. Authoring the WoT regions (via the `mud-world-builder`
  skill) is a content prerequisite for the regional layer — not a blocker
  for the engine or for the non-regional recipe tiers.

---

## Cross-references

- `item-decorations.md` — rarity tiers ARE the crafting quality tiers
  (§5); output stamps a rarity key.
- `progression.md` §3, §3.5 — the crafting track and use-based proficiency
  gain (§2).
- `abilities-and-effects.md` + `economy-survival.md` §6 — the well-fed
  effect via the consumable effect pipeline (§6).
- `economy-survival.md` §3 — the sustenance pool cooking feeds; shops for
  common recipes (§7).
- `inventory-equipment-items.md` — item instances, instance properties
  (quality), atomic consume of inputs (§3, §5).
- `world-rooms-movement.md` §6 — terrain + weather that gate Tier 1
  stations (§4).
- `mobs-ai-spawning.md` §6.3 — mob loot as a rare-recipe / ingredient
  source (§7, §8).
- `quests.md` — uncommon recipes as quest rewards (§7).
- gathering/biomes (greenfield, see `BACKLOG.md`) — the ideal ingredient
  source (§8).
