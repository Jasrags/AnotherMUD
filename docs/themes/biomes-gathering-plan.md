# Biomes + Gathering — implementation plan

**Status:** Design pass (no implementation yet). **Specs (source of
truth):** `docs/specs/biomes.md`, `docs/specs/gathering.md`,
`docs/specs/crafting-and-cooking.md` §8. This plan sequences the two
spec-only contracts into buildable milestones and closes crafting's §8
economy guardrail.

## Why this exists

Crafting §8 demands ingredients come from **gathering / mob drops / player
farming, not fixed-price vendors**, so crafting is a gold sink rather than a
money printer. Today the core pack uses a **vendor-ingredient stopgap**
(Brandr sells rusty daggers, Marta sells raw meat) — explicitly a temporary
compromise. This milestone builds the real source.

**Dependency:** Gathering reads the room **biome's** forage table + node
spawn table, so **biomes is a hard prerequisite**. Note gathering is **not**
geography-blocked — it only needs biome tables on rooms; only the *regional
recipe* tier (separate, Phase 7) needs an authored world map.

## Decisions (locked this session)

- **D1 — Full biomes first.** Build the complete `biomes.md` spec before
  gathering: registry + resource tables + ambience + the §3 shielding
  generalization (promote the hardcoded `indoors`/`underground` shielding in
  `world-rooms-movement.md` §6.4 into registered biomes). Higher up-front
  effort (touches live weather/time shielding) but leaves no biome debt.
- **D2 — Vendor policy: gather-primary, vendor stays, with two invariants.**
  Vendors MAY sell basic intermediates (iron ingot, wood plank), but:
  1. **Crafting an item is always cheaper than buying it.** Pricing is
     `value × markup` uniformly, so this reduces to a **content pricing
     discipline: an item's `value` must exceed the summed `value` of its
     craftable inputs** (the value crafting adds). Gathered inputs cost ~0
     gold (time/cooldown only), so a gather→craft path is always cheaper
     than a vendor buy.
  2. **Vendors never supply a whole recipe.** Every finished recipe's
     ingredient chain bottoms out in **≥1 gather-only or loot-only input**,
     so §8's strict line ("not satisfiable purely from fixed-price vendors")
     holds at the chain level.
  - Consequence: a **refining tier** — `gather raw (ore/logs) → refine
    intermediate (ingot/plank) → craft finished (dagger)`. Vendors sell the
    intermediate at a convenience premium; the rational, §8-compliant path
    is gather→refine→craft.
- **D3 — Optional content guardrail.** A boot/content-review check flags any
  recipe where `output.value ≤ Σ(input.value)` (a "crafting loses money"
  smell) so the D2.1 invariant can't silently rot. Cheap; worth doing.

## Milestones

Each milestone is independently committable, keeps `go test -race ./...`
green, and gets a code review before being called complete (project gate).
Commits land on `main` (no branches).

### Milestone A — Biomes (full spec) — ✅ SHIPPED (8deefa5, 7464225, a69ab5d)

The prerequisite. Backward-compatible: an unregistered `terrain` value
behaves exactly as today (`biomes.md` §2.3). A1 registry + resolution
(8deefa5); A2 shielding generalization, no-op for existing content
(7464225); A3 ambience tick (a69ab5d). Resource-table fields are carried on
the Biome struct (inert until Milestone B). Next: Milestone B.

- **A1 — Biome registry + terrain resolution.** New pack content registry
  (`biomes.md` §2), engine-scope ids unprefixed / pack-scope namespaced
  (PD-3, mirrors tags/slots/properties). Room `terrain` value → biome
  lookup; missing → default `outdoors`; unregistered → bare-string
  back-compat. No new room property (the existing `terrain` is the key).
- **A2 — Shielding generalization (§3).** `indoors`/`underground` ship as
  registered biomes carrying `weather-shielded`/`time-shielded`; the
  `world-rooms-movement.md` §6.4 eligibility check reads biome flags instead
  of the two hardcoded strings. Per-room exposed overrides still win.
  *(Riskiest slice — touches live weather/time ambience gating; lean on the
  §3.1 acceptance tests for the no-regression proof.)*
- **A3 — Biome ambience (§4).** New time-interval tick handler: pick a
  random line from a biome's ambience pool, deliver to occupied rooms of
  that biome. No bus event (pure presentation, like time ambience);
  independent of the shielding gate.
- **A4 — Resource-table fields (§2, §5).** Biome definition carries an
  (optional) forage table + node spawn table + mob spawn table. These are
  data Milestone B consumes; A only needs to load + expose them by id.
- Persistence: none (biome defs are content; ambience state ephemeral).

### Milestone B — Gathering (forage + nodes) — ✅ SHIPPED (e348237 … b038457)

B1 proficiency+roll (e348237); B2a forage-table infra (e9958c0); B2b forage
verb+events+cooldown (8d85d2a); B3a node infra (e0e4531); B3b node spawn
integration — extends the area scheduler, no mob regression (7ca1e0f); B3c+d
harvest verb + atomic TakeCharge + content (b038457). Both models live-verified.
Next: Milestone C (real biome content breadth + §8 recipe migration to
gathered/refined inputs).

Consumes Milestone A's biome tables. Both models ship together (`gathering.md`
PD-1).

- **B1 — Gathering proficiency + quality roll (§4).** A single `gathering`
  proficiency on the existing progression surface (use-based gain, no new
  store, PD-2). Quality roll mirrors `crafting §5` (proficiency + tool +
  source richness → rarity tier, clamped to source ceiling). Reuse/share the
  crafting quality-roll shape where practical.
- **B2 — Ambient forage (§2).** `forage` verb: resolve room biome → forage
  table → cancellable `resource.gathering` → cooldown gate (§5) → roll →
  create ingredient(s) in inventory → `resource.gathered` → use-gain. Never
  refused for lack of tool (§1.1); "nothing to forage here" when the biome
  has no table (absence, not refusal). Per-character forage cooldown is the
  primary limiter; optional per-room depletion off by default.
- **B3 — Resource nodes (§3).** Node template (yield table, charge count,
  optional required tool, respawn interval). Nodes spawn into biome rooms
  via the **existing area/spawn scheduler** (`mobs-ai-spawning §3`) — a node
  is a spawned non-mob entity with a respawn timer. `harvest` verb: resolve
  by keyword → cancellable event → tool gate (the ONE allowed refusal, §3.3)
  → roll → decrement charges → `resource.gathered`; at zero charges remove +
  `node.depleted` + schedule respawn.
- **B4 — Events + wiring.** `resource.gathering` (cancellable),
  `resource.gathered` (quest advance-on-event hook), `node.depleted`. Tick
  handlers: forage-richness regen (if depletion on) + node respawn (rides
  spawn scheduler). Update README event/tick/NOT-persisted tables.
- Persistence: none new — proficiency rides progression; node charges +
  forage richness are transient (PD-4, respawn fresh on restart, like
  Tier-1 craft stations / temporary exits).

### Milestone C — Content + crafting §8 closure — ✅ SHIPPED (fbd77a0 … 641bf89)

Where the economy actually changes. Mostly content + recipe rework. C1 raw +
refined items (fbd77a0); C2 wilderness biome loop + huntable boar (f9a0514);
C3 recipe re-point + ingot shop (2bfe634); C4 economy guardrail boot check
(05ee8dd) + test gap fix (641bf89). Live-verified end-to-end (see below).

**Decisions taken during build:** meat = hunt-primary + vendor backup (a
neutral huntable `wild-boar` drops raw meat via `boar-loot`; Marta still
sells meat at markup); geography = a 5-room wilderness loop (forest-edge →
deep-forest → foothills → cave-mouth → old-mine) off the meadow. The plan's
`wood-plank`/woodworking tier was **deferred** (it would need a 3rd
discipline + trainer); instead `timber-log` is `fuel`-tagged so it feeds
`build campfire` — a gathered alternative to bought firewood, no new
discipline. `rough-stone` (grassland/foothill outcrops) remains a flavor
gather with no recipe consumer yet (future alchemy/masonry); not a §8 blocker.

**Live-verified loop (clean telnet playthroughs):** learn smithing → 2
baseline recipes incl. smelt → harvest iron vein (pickaxe-gated, depletes at
3 charges) → `craft smelt-iron-ingot` (ore→ingot) → `craft reforge-short-sword`
(ingot→sword), all from gathered material at zero gold; forage forest →
wild herb + forest mushroom (quality-stamped); `kill boar` → corpse holds 2
cuts of raw meat. Forage cooldown + node depletion + biome ambience all fire.
The C4 guardrail's regression test confirms the shipped pack has no
money-losing recipe.

- **C1 — Raw + intermediate items.** Author raw gathered materials (ore,
  logs, herbs, hide) and refined intermediates (iron ingot, wood plank),
  priced per D2.1 (intermediate value > Σ raw input values).
- **C2 — Biome content.** Register wilderness biomes (forest, mountain/cave)
  with forage tables (herbs/fibers) + node spawn tables (ore vein, timber).
  Place them on existing + a few new wilderness rooms. Author tool items
  (pick, axe) for tool-gated nodes; ambience pools for flavor.
- **C3 — Refining + finished recipes.** Refining recipes (ore→ingot,
  logs→plank). Re-point finished recipes (the daggers, cooking) to consume
  gathered/refined inputs, ensuring **≥1 gather/loot-only input per chain**
  (D2.2). Keep vendor sale of basic intermediates as a priced convenience
  (D2.1 keeps craft cheaper).
- **C4 — Guardrail check (D3).** Boot/content validation flagging any recipe
  with `output.value ≤ Σ input.value`.
- Live-verify the full loop: gather ore → smelt ingot (cheaper than buying
  one) → forge dagger; forage herbs → cook; confirm no recipe is satisfiable
  purely from vendor stock.

## Risks

- **A2 shielding refactor** is the highest-risk change — it touches live
  weather/time ambience for every room. Mitigation: ship `indoors`/
  `underground` as registered biomes with identical flags; the §3.1
  acceptance tests are the no-regression proof; consider landing A1+A3+A4
  first and A2 as its own reviewed commit.
- **Spawn-scheduler reuse for nodes (B3)** assumes nodes fit the non-mob
  spawned-entity shape; verify the scheduler can place + respawn a non-mob
  entity before committing to "reuse, don't rebuild."
- **Recipe-chain completeness (C3)** — every finished recipe must remain
  craftable from gatherable inputs, or it becomes uncraftable. The C4 check
  guards pricing, not reachability; a separate content review confirms each
  chain bottoms out in something obtainable.
- **Quality-roll sharing (B1)** — reusing crafting's roll across packages
  may need a small shared helper to avoid coupling `gathering` →
  `crafting`; prefer a shared leaf (like the rarity ladder) over a direct
  import.

## Sequencing

A (full biomes) → B (gathering) → C (content + §8 closure). A and B are
engine; C is mostly content. A2 may be split out as a discrete reviewed
commit given its risk. Regional recipes (Phase 7) remain separate and
geography-blocked; this milestone does not touch them.

## Open questions (deferred, not blocking)

- Gathering sub-skills (mining/herbalism split) — v1 is one proficiency
  (`gathering §9`).
- Skinning corpses, player farming, persisted node state, gathering
  interrupts (cast-time gather) — all `gathering §9` future work.
- Biome-driven mob spawn depth — `biomes §5` capability is defined; full
  area-spawner integration is its own milestone scope.
