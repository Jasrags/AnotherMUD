# EPIC: Shadowrun MVP — the Street Samurai slice

> **Status:** build spec (no code yet). Authored 2026-07-05.
> **Parent analysis:** `docs/themes/shadowrun-pack-plan.md` (the full subsystem map + the hard-40% archetype scorecard — read it first; this doc does *not* re-derive that analysis, it sequences the buildable slice of it).
> **Companion docs:** `docs/themes/channel-vocabulary.md` (multi-ruleset on one kernel), `docs/shadowrun/` (the SR **5th Edition** rules reference corpus — note: the `CHARACTER.md` ASCII sheet is legacy "3rd Edition" art from the source MUD; the *mechanics* corpus (`TESTS.md` limits + Matrix initiative, `ROLLS.md` hits/glitches, `Edge`) is unambiguously SR5), `docs/specs/character-identity.md` (world-locking), `docs/ENGINE-VOCABULARY.md` (content↔engine contract).
> **Posture:** *spirit, not fidelity* — inherited from `channel-vocabulary.md` §1 and the pack plan §1. We keep the `d20 + mod vs difficulty` resolution kernel and translate SR's *flavor + meaningful choices* onto it. We do **not** simulate d6 dice pools, the Rule of Six, glitches, staged wound boxes, or drain staging.

---

## 1. What the MVP is (and is not)

**The MVP is a bootable `kind: world` `shadowrun` pack in which a *Street Samurai* can be created, walk a district, and win a gunfight — end to end, on the existing tick/chance kernel.** It is the one runner role the pack-plan scorecard (§3.2) rates *near-turnkey*, because every subsystem it touches already has an engine home.

**In scope (this EPIC):**
- 5 metatypes, the SR3 attribute set, nuyen, a channel mapping, a dozen skills, a starter weapon/armor set, cyberware-as-stat-boost, a starting district with mobs, and advancement via **Option A (karma-as-XP on the existing level/track engine)**.

**Explicitly OUT (each its own arc *beyond* this EPIC — see plan §3.1/§6):**
- The Matrix / decking / Technomancers, spirits + astral space, rigging / drones, the contacts network, Lifestyle / SIN / legality gating, real initiative-pass ordering, and the karma-ledger advancement engine (Option B). Essence→Magic decay is *staged in but inert* until a mage arc (§ SR-M4 is optional).

If a request pulls toward any OUT item, it is a **new arc**, not MVP scope creep. Flag it.

---

## 2. The pinned decisions (resolve-once, recorded here)

| # | Decision | Resolution | Rationale |
|---|---|---|---|
| D1 | **Edition** | **SR5** (5th Edition) | The mechanics corpus is unambiguously SR5: `TESTS.md` Physical/Mental *limits* + Matrix (AR/VR, Data Processing) initiative, `ROLLS.md` hits/glitches/critical-glitches, `Edge`, attribute-pair tests. (The `CHARACTER.md` sheet art says "3rd Edition" — legacy, ignore it.) No mixing. |
| D2 | **Attribute set** | **8 seeded primaries**: Body, Agility, Reaction, Strength (physical); Willpower, Logic, Intuition, Charisma (mental). Plus **Edge** (special) at seed. Essence starts 6; Magic/Resonance 0 for a mundane runner. | SR5 has **8** primaries + Edge — *more* than the engine's default six, so SR-M1 genuinely must support a variable-size, differently-named set. Reaction is a **real seeded attribute** in SR5 (augmented by Wired Reflexes etc.), *not* derived. This is the strongest single argument for making the attribute set content-declared (SR-M1). |
| D3 | **Advancement** | **Option A — karma-as-XP** on the existing track/level engine. | SR5 also advances on Karma (raise at `new rating × multiplier`). Option A falls out of the MVP for ~free and validates the pack pathway; build the karma-ledger (Option B) only *after* playing A. Not speculative. (plan §7) |
| D4 | **Initiative** | **Cosmetic for v1** — global tick cadence, no per-pass ordering. `initiative` channel stays declared-but-unread. | SR5 leans on Initiative Score + multiple passes harder than WoT; real turn order is Medium Go — defer until it bites. (plan §4 optional, §10) |
| D5 | **Condition monitors** | **Two pools** (Physical + Stun), overflow = arithmetic (death vs unconscious), **not** staged wound boxes. Box counts derive SR5-style (`8 + ⌈Body/2⌉` phys, `8 + ⌈Willpower/2⌉` stun) as pool max via a channel. | Pools exist; box-staging + wound-modifier penalties are fidelity we gave up (plan §5). Stun-overflow → the existing `unconscious` condition (shipped for subdual). |
| D6 | **Skill model** | **Flat proficiency map** (use-based gain), specialization = a source-keyed `+2`. No skill-group tree. | Reuses the shipped proficiency engine; SR5 skill groups are content polish if ever wanted. (plan §10) |
| D7 | **Edge** | Map to a generalized **`pool`** (current/max, spend-to-push, slow refresh). MVP: a single "reroll/bonus" spend; the full SR5 Edge menu (pre/post-roll, seize initiative, dead-man's trigger) is flavor-trimmed. | SR5 Edge is a luck/push resource — a textbook fit for `internal/pool`. Cheap, and it's a defining SR5 feel-good knob. Optional within M3. |

---

## 3. The slices

Each slice is independently shippable and pays for itself even if the program stops there. Sizes: **Content** (no Go) · **Small Go** · **Medium Go**. Slices SR-M1→M3 are the playable core; M4/M5 are staged follow-ons.

### SR-M1 — Content-defined attribute set  · **Medium Go · the gating blocker**

> **STATUS: SHIPPED 2026-07-06** (commits `db6bc2d` registry → `5220f9e` core `classic` → `5a05f44` world-aware seed → `02efc31` data-driven `score` → `f30a0b6` trainable-from-set). All five Appendix A steps landed, each go-reviewer APPROVE, full suite `-race` green. Zero behavior change for existing worlds (they resolve to `classic` == the old hardcode). Remaining tail folded into SR-M3: the wizard's attribute point-buy step, and wiring the set's per-attribute `Cap` into race-cap enforcement (harmless today — classic `cap == DefaultRaceCap == 25`; matters once SR metatype `StatCaps` land — tracked in §7).

The one true prerequisite (plan §4.1). `progression/statblock.go` hardcodes six stat constants and the creation wizard seeds them via `DefaultPlayerBase()`; `score` display and caps assume that set. `StatBlock` is *already* a `map[StatType]int`, so storage is fine — the friction is chargen + display + caps assuming the engine's fixed six. SR5 needs **9** seeded values (8 primaries + Edge), so this is a *count-and-name* change, not just relabeling.

**Approach:** make the base attribute set a **per-world content declaration** the wizard reads, instead of calling `DefaultPlayerBase()`. The engine's `str/int/wis/dex/con/luck` becomes the *generic/WoT* declaration; `shadowrun` declares `body/agility/reaction/strength/willpower/logic/intuition/charisma` (+ `edge`).

**Acceptance criteria**
- [x] A world pack can declare its seeded attribute set (id, display name, default, per-attribute cap) as content. *(`attributes/*.yaml` → `progression.AttributeSetRegistry`; core ships `classic`.)*
- [x] The character seed comes from the *active world's* declaration, not a fixed `DefaultPlayerBase()`. *(The actor constructor resolves worldID → set; the wizard's attribute **point-buy step** is SR-M3.)*
- [x] `score` renders the active world's attribute names/values (no hardcoded six-stat layout). *(Data-driven `scAttrGrid`, grouped by category.)*
- [~] Stat caps honor the per-attribute declaration (metatype modifiers layer on top — SR-M3). *(Trainability is set-driven; wiring the set's `Cap` into race-cap enforcement is the SR-M3 tail — see §7 / STATUS above.)*
- [x] WoT + starter-world boot **unchanged** (their declaration reproduces today's six exactly — a regression gate). *(`TestCorePack_ClassicSetMatchesEngineDefaults` + `SeedBaseFromSet(classic) == DefaultPlayerBase()`.)*
- [x] A save round-trips a non-default, non-six-sized attribute set. *(The ordered pair-list save already round-trips any key set; proven at the loader/registry layer — an actual `shadowrun` save lands with the pack in SR-M3.)*

**Why first:** SR5's 8-primary + Edge set can't even be *seeded* without it, and it future-proofs *any* generic/point-buy pack — a reusable engine win independent of SR. (Note: because the set is now variable-size, the `score` sheet must render N attributes, not a fixed six-row grid — a real display change, not just relabeling.)

### SR-M2 — Typed damage: `type` + `target_pool`  · **Small Go · reusable by WoT too**

The damage struct gains two fields (plan §4.3): a damage **`type`** (feeds type-specific `mitigation` → Ballistic vs Impact) and a **`target_pool`** (which monitor an attack fills → Physical vs Stun). Physical-overflow → death; Stun-overflow → the shipped `unconscious` condition.

**Acceptance criteria**
- [ ] A weapon/attack declares a damage `type`; the `mitigation` channel can resist per-type (`armor_ballistic` vs `armor_impact`). Reuse the existing `TypedResistance`/typed-mitigation path from WoT S1.
- [ ] A weapon/attack declares a `target_pool`; damage routes to that pool (bullet → Physical, stun baton/fist → Stun).
- [ ] A `target_pool` reaching zero carries a per-pool zero-meaning: Physical → death (existing path), Stun → apply `unconscious` (the subdual seam) rather than kill.
- [ ] `VitalDepleted` carries the death-vs-KO flag so consumers branch correctly.
- [ ] Untyped / default-pool attacks behave exactly as today (WoT/starter-world regression gate).

**Why second:** WoT S1 typed damage wants this regardless, and it yields *both* Ballistic/Impact *and* Physical/Stun in one stroke — the widest engine leverage in the EPIC.

### SR-M3 — The minimal `shadowrun` world pack  · **mostly Content + small wiring**

> **STATUS: SHIPPED 2026-07-08 — all 8 acceptance criteria proven live.** The
> playable Street Samurai MVP (SR-M1 → M3) is complete: boot, creation (Street
> Samurai by default), combat (lethal Physical + stun-KO with soak), cyberware,
> karma advancement, and the nuyen shop all have live regression tests
> (`cmd/telnet-smoke/shadowrun_*_live_test.go`). Two engine fixes fell out of it
> — the class-`bound_track` primary track (`c66cea0`) and world-scoped creation
> menus (`e15d2a7`) — both of which also fixed the latent WoT equivalents.
> Post-MVP: SR-M4 (Essence pool) and SR-M5 (karma-ledger advancement) remain
> optional/deferred; the firearm+ammo mechanic and a Body-derived Physical
> monitor are the notable open tails (see `sr-m3c-deferred-fixes`).

Stand up `content/shadowrun/` (`kind: world`, depends on `tapestry-core`) and make it bootable. This is the first end-to-end exercise of the channel layer by a **non-WoT** pack.

Content inventory:
- **Metatypes** (races): human, elf, dwarf, ork, troll — SR5 attribute modifiers + augmented-max + size (`internal/size`: troll Large, dwarf Small, ork/elf/human Medium) + vision (low-light/thermographic as `effect`/racial flags).
- **Attribute-set declaration** (consumes SR-M1) — the 8 SR5 primaries + Edge + per-metatype min/max caps (SR5 metatypes set per-attribute ranges, e.g. troll Body 1–10 aug 15).
- **Channel mapping** (`channel-map/*.yaml`, later-wins override) — SR5 attribute pairings, translated to single-roll channels:
  - `attack: <skill> + agility` (SR5 firearms/melee are Agility-linked) · `defense: reaction + intuition` (SR5 defense test) · `damage_bonus: trunc(strength / 4)` (tune to SR5 STR→melee DV; firearms DV is weapon-flat) · `mitigation: body + armor_ballistic` (SR5 soak = Body + Armor; the subtract step is already live).
  - Physical/Stun monitor maxes as channels: `hp_physical: 8 + ceil(body / 2)` · `hp_stun: 8 + ceil(willpower / 2)`.
  - *(SR5 Physical/Mental/Social **limits** — caps on net hits — have no single-roll analog; dropped as flavor under the posture. Note it, don't model it.)*
- **Nuyen** — declare a currency (content-only economy primitive).
- **Edge** — a `pool` (D7), if included in this slice.
- **Skills** (~12, flat proficiency, SR5 names): Pistols, Automatics, Longarms, Blades, Clubs, Unarmed Combat, Sneaking, Perception, Athletics (Running/Gymnastics), First Aid, Negotiation, Etiquette.
- **Weapons + armor** (consumes SR-M2): a light pistol, a heavy pistol, an SMG (Automatics), a katana (Blades), a stun baton (Stun/target_pool); an armor jacket + a lined coat (SR5 single Armor rating on the `mitigation` channel; Impact vs Ballistic collapsed to one rating unless a demo wants both).
- **Cyberware as stat boosts** — a few items (wired reflexes → Reaction + Initiative, muscle replacement → Strength/Body, cybereyes → vision) sourced through the `srckey` modifier pipeline. Essence cost is **flavor text** in the MVP (real Essence decay is SR-M4).
- **World** — one district (a few rooms/an area), a fixer/contact NPC (flavor), 1–2 hostile mobs (a ganger, a lone sec-guard) to fight.
- **Splash** (`splash.txt`, required for `kind: world`) + a Street Samurai background/creation package.

**Acceptance criteria**
- [x] `ANOTHERMUD_PACKS=shadowrun ANOTHERMUD_START_ROOM=shadowrun:<room>` boots with no registry collision alongside `core` (namespaced ids; later-wins channel override). WoT boot unaffected. *(`pack.shadowrun_boot_test.go`; both live combat tests boot it.)*
- [x] A player creates a Street Samurai (metatype + attribute seed + background) through the wizard. *(the DEFAULT flow now yields a Street Samurai — `TestLive_ShadowrunKarmaAdvance` creates one via `createAndLogin` (no explicit pick) and lands on "The Long Run". Fixed by world-scoping the creation menus: a world that ships its own classes/backgrounds offers ONLY those, hiding the tapestry-core baseline `fighter`/`commoner` that leaked in via the core dependency — `worldClassFilter`/`worldBackgroundFilter`, applies to every world flow incl. WoT. A world that ships none inherits the core baseline. Unit: `TestWorldClassFilter`/`TestWorldBackgroundFilter`.)*
- [x] One combat round resolves **attack → soak → route to Physical/Stun** using only channels + SR-M2 routing (no bespoke SR combat code). *(proven live both routes: `TestLive_ShadowrunStunKnockout` (Stun) + `TestLive_ShadowrunLethalKill` (Physical).)*
- [x] A stun weapon fills the Stun monitor; overflow knocks the target unconscious rather than killing. *(`TestLive_ShadowrunStunKnockout`; overflow→Physical shipped SR-M3c-3.)*
- [x] A lethal weapon fills the Physical monitor; worn armor reduces it via `mitigation`. *(`TestLive_ShadowrunLethalKill` — katana kills the armored ganger through its body+armor soak → lootable corpse. The firearm+ammo path uses the same default route; ammo mechanic itself still unexercised.)*
- [x] Cyberware equipped/removed shifts the sourced attribute (via `srckey`), visible on `score`. *(`TestLive_ShadowrunCyberware`: wired reflexes raises Reaction 3→5 on equip, restores on unequip. PURE CONTENT — a `cyberware` slot (max 3) + three implants (wired-reflexes→Reaction, muscle-replacement→Str/Body, cybereyes→Intuition) with item `modifiers`; the standard equip → `EquipmentSourceKey` → stat-block pipeline (equipment.go) applies/removes them, `score` reads the effective attribute. Essence cost is flavor text (SR-M4). Needed a `slots:` glob in the shadowrun manifest — content globs are explicit, not directory-convention.)*
- [x] Nuyen is earned/spent at a shop. *(earn: auto-credited from looted ganger/sec-guard corpses. spend: `TestLive_ShadowrunNuyenShop` — a street fixer on the safe corner (`mobs: [fixer]`, `properties.shop.sells`) lets a runner `list` + `buy clip` for 24 nuyen (500→476), item lands in inventory. Standard shop service, no bespoke SR economy code.)*
- [x] Advancement runs on the existing engine (karma-as-XP, D3) — a kill grants karma, a track advances. *(`TestLive_ShadowrunKarmaAdvance`: a ganger kill banks 30 on the street-samurai's `street`/"The Long Run" track, and crossing 100 XP advances it to Level 2. REQUIRED an engine fix — a character's primary track (kill-XP target + `score` headline) now derives from its class `bound_track`, not the global `DefaultXPTrack="adventurer"`. Previously an SR/WoT character earned + displayed the core `adventurer` track and its own world track was inert.)*

**Why third:** with M1+M2 in place this is *mostly content*, and it's the validation gate — a genuinely playable, if simplified, Street Samurai.

### SR-M4 — Essence pool + `degrades: magic`  · **Small Go · OPTIONAL (mage prerequisite)**

`pool.Rules.Degrades` is built but used by nobody (plan §4.4). An `essence` pool whose `current` clamps a `magic` channel max is the textbook use. **Not required for a mundane Street Samurai** — sequence it when the *first mage/adept* is on the table, or as a "close the Essence-is-exotic myth" demo.

**Acceptance criteria**
- [ ] An `essence` pool declared in content, starting at 6.
- [ ] Cyberware install lowers Essence; `pool.degrades: magic` clamps the `magic` channel max at channel-resolve time (a few lines to honor `degrades`).
- [ ] A mundane character (Magic 0) is unaffected — pure regression.

### SR-M5 — Advancement fork (Option B, karma-ledger)  · **Real Go · NOT MVP**

Recorded for completeness only. `progression/manager.go` is hardwired to `XP → track → level-up`; a faithful SR spends karma à la carte at `cost = rating × multiplier`. This is the largest single engine investment and is **deliberately deferred** (D3). Build a *pluggable advancement strategy* (level-track = one impl, karma-ledger = another) **only** after the Option-A MVP has been played and its identity gap felt. Do not build speculatively.

---

## 4. Dependency order & rationale

```
SR-M1 (attribute set) ──┐
                        ├──> SR-M3 (the pack, playable) ──> [play it] ──> SR-M4 (essence, if mage) ──> SR-M5 (karma, if needed)
SR-M2 (typed damage) ───┘
```

- **M1 + M2 are engine slices** with value beyond SR (generic pack; WoT typed damage). Do them first; they are the only Go in the playable core.
- **M3 is the payoff** — mostly content, the validation gate, a shippable Street Samurai.
- **M4/M5 are staged** and gated on *actual* need (a mage; a felt advancement gap), not built ahead.
- **Engine-debt discipline** (the BACKLOG standing rule): interleave a small warm-up/debt slice if one surfaces between M1→M3.

---

## 5. Validation gate (mirror of pack-plan §8, narrowed to the MVP)

Before writing pack *content* (SR-M3), confirm M1+M2 delivered:

- [ ] Every SR subsystem an early-game Street Samurai touches has an engine home (content / small-Go), per plan §3.
- [ ] The `shadowrun` `kind: world` pack loads alongside `core` (+ `wot`) with no registry collision.
- [ ] One SR combat round resolves attack → soak → route to Physical/Stun using only channels + the M2 routing.
- [ ] SR Drain (when a mage lands) will reuse the One Power pool + `resist.backlash` seam — no second drain engine. *(Verify the seam is reachable; not exercised in the mundane MVP.)*
- [ ] Chargen seeds the SR attribute set from content, not `DefaultPlayerBase()`.

If a box fails, fix the subsystem map (plan §3) or the blocker list (plan §4) — **not the kernel.**

---

## 6. Configuration surface (additions)

| Knob | Default | Notes |
|---|---|---|
| world attribute set | engine six (`str/int/…`) | per-world content declaration consumed by chargen (SR-M1); SR = 8 primaries + Edge |
| damage `type` | untyped | feeds type-specific `mitigation` (Ballistic/Impact) (SR-M2) |
| damage `target_pool` | `hp` | which pool a damage instance fills (Physical/Stun) (SR-M2) |
| `VitalDepleted` zero-meaning | death | per-pool: Physical→death, Stun→unconscious (SR-M2) |
| advancement strategy | `level-track` | `level-track` \| `karma-ledger` — SR-M5 (Option B) only |

---

## 7. Open questions

- **SR5 Limits.** Physical/Mental/Social limits cap *net hits* per test — there's no single-roll analog under the posture (D1/plan §5). MVP drops them as flavor. Flag if playtesting says combat/social checks feel unbounded; a limit could later become a soft cap on a channel's margin.
- **Wound modifiers.** SR5 applies −1 dice per 3 boxes of damage. Our monitors are pools, not boxes (D5), so wound penalties don't fall out for free. MVP: no wound penalty (a felt loss). Could later map to a `condition` that scales an attack/defense penalty off pool depletion — a small slice if wanted.
- **Metatype attribute caps.** SR5 metatypes set per-attribute min/max/augmented-max (troll Body 1–10 aug 15, elf Agility). Race-sourced modifiers + a cap channel, or a new chargen constraint? Lean: modifiers + a per-attribute cap the wizard honors (folds into SR-M1's cap declaration).
- **Edge depth.** D7 maps Edge to a `pool` with a single spend. The full SR5 Edge menu (pre-roll extra dice / Push the Limit / post-roll reroll / seize the initiative / dead-man's trigger) is trimmed. Which one spend ships first? Lean: a post-roll reroll-a-failed-check (the most universally useful), added only if it fits M3's budget.
- **Ranged in-room vs range bands.** WoT S1 shipped range bands (far→near→melee). SR5 is gun-forward with explicit range categories (Short/Medium/Long/Extreme → dice penalties); decide whether the MVP pistol fights in-room (simplest) or reuses the shipped band model. Lean: reuse the band model — it exists and guns want it more than bows did.
- **Karma-as-XP mapping.** Under D3, what grants karma and at what rate (kill XP is the obvious seed)? A content/tuning question, not an engine one.

---

## 8. Relationship to the rest of the backlog & the shared seams

- **The owned-entity seam is now shipped.** Spirits (magic arc) and drones (rigging arc) both want an "owned, controllable entity that follows/obeys and acts on its turn" — that seam landed as **hireable mobs** (2026-06-25, per project memory). When the magic/rigging arcs start, build spirits/drones *on that seam*, not from scratch (plan §3.1 design-together note).
- **Vehicle movement** (rigging) reuses the `mounts` metered-mover seam (shipped).
- **Assensing** (magic) extends `visibility` (shipped).
- **Dual reputation** (Street Rep + Notoriety) extends `reputation.md` with a second axis — a Small Go slice that can land opportunistically.

These are all *post-MVP*. The MVP (SR-M1→M3) needs none of them.

---

## Appendix A — SR-M1 implementation plan (file-level)

Derived from a code sweep (2026-07-05). **Key finding: the storage substrate is already attribute-agnostic.** `StatType` is a typed *string* (`progression/statblock.go:42`), `StatBlock` is map-backed and reads absent keys as 0 (`statblock.go:148-188`), the save is an ordered `[]BaseEntry{Stat,Value}` pair-list (`statblock.go:579-589`, `player.go:165`), and the channel evaluator resolves any stat name via an injected `lookup`, unknown→0 (`channel/expr.go:52-58,104-111`). So this is **not** a storage refactor — it's replacing five hardcoded-six *seams* with reads from a content-declared set. No save-version bump if the core declaration reproduces the six exactly.

### The five leak sites

| # | Seam | Anchor | Fix |
|---|---|---|---|
| L1 | `DefaultPlayerBase()` seeds the six | `progression/statblock.go:102-115` | Derive attribute keys from the world's attribute-set declaration; keep engine-vital keys (`hp_max`/`movement_max`/`hit_mod`/`ac`). |
| L2 | Actor constructor injects the six *pre-merge* | `session/session.go:675` (`NewWithBase(DefaultPlayerBase())`) then `RestoreBase` at `:908` | **The correctness edge.** Because `RestoreBase` *merges* (`statblock.go:459-485`), a non-default-world save would carry *both* its set and leftover `str=10…`. Seed the constructor from the character's world set (creation → active world; login → `loaded.Player.WorldID`, available before the merge). |
| L3 | `score` renders six fixed rows | `command/score.go:57-63` (reads), `:254` (struct fields), `:386-391` (3 two-up rows) | Iterate the world attribute set in declared order, grouped by category (physical/mental/special). Data source `StatValue` is already generic (`session.go:5164-5169`). |
| L4 | Trainable gate hardcodes the six | `progression/training.go:130-146` (`DefaultTrainingConfig`), instantiated at `cmd/anothermud/main.go:3116` | Build the trainable set from the declaration's per-attribute `trainable` flag. A `SetTrainable` mutator already exists but is unwired. |
| L5 | No content stat registry exists | — (`internal/pack` has no `loadStats`/`StatRegistry`) | New content type + loader + registry (below). |

**Not leaks (already generic — leave alone):** the constant block (`statblock.go:49-66` — engine keys can stay; a world simply doesn't use `str…`), race `StatCaps` (`race.go:49-54`, map, arbitrary keys), cap *enforcement* (`training.go:504-530`, runtime string), class `StartingStats` grant (`class.go:61-68` → `ApplyStartingStats` `session.go:5097-5122`, iterates the map), the save shape, and the channel lookup. GMCP does **not** expose the six (`gmcp_charstatus.go`), so nothing to touch there.

### New content type (L5) — the attribute-set declaration

A per-pack content type (glob-enumerated, per the backgrounds-glob lesson), loaded into a `progression`-side registry keyed by set id. Proposed shape:

```yaml
# content/core/attributes/classic.yaml  (the engine six — the regression gate)
id: classic
attributes:
  - { id: str,  name: Strength,     abbrev: STR, default: 10, cap: 22, trainable: true, category: physical }
  - { id: int,  name: Intelligence, abbrev: INT, default: 10, cap: 22, trainable: true, category: mental }
  # …wis/dex/con/luck…
```

A world pack selects/declares its set; `shadowrun` (SR-M3) declares the 8 SR5 primaries + Edge with `category: physical|mental|special`. `cap` here is the *default* ceiling; race/metatype `stat_caps` still override per-race (unchanged path).

### Build order within SR-M1 — ✅ ALL SHIPPED (2026-07-06)

1. ✅ **Content type + registry + loader** (L5) — `attributes/*.yaml` → `progression.AttributeSetRegistry`. `db6bc2d`.
2. ✅ **Core declares `classic`** (the six) — the regression gate. `5220f9e`.
3. ✅ **World-aware seed** (L1+L2) — `resolveAttributeSet`/`seedBaseFor` seed the constructor from the character's world set; `RestoreBase` then merges the same keys (no "carries both sets"). `5a05f44`.
4. ✅ **`score` iterates** (L3) — `scAttrGrid` renders N attributes by category/order. `02efc31`.
5. ✅ **Trainable from declaration** (L4) — `entityTrainable` gates on the set; `DefaultTrainingConfig` is now the nil-set fallback only. `f30a0b6`.

Step 3 was the only subtle one (the L2 pre-merge ordering); it landed clean. The Shadowrun *declaration itself*, the wizard's point-buy chargen step, and cap-from-set enforcement are **SR-M3**, not M1 — M1 proved the substrate using the existing six.

### Design decisions taken (not asking)

- **Set lives in a pack `attributes/` content type**, not on the manifest — mirrors every other registry (races, feats, factions) and keeps world-locking/override semantics uniform.
- **Core declares `classic`; the seed is data-driven for every world** (no special-case fallback in Go) — makes the WoT/generic path exercise the same code as SR, so the regression gate *is* the test.
- **No save bump in M1** — the pair-list save already round-trips any key set; existing `str…` saves reload unchanged against the `classic` declaration.
