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
- [x] Stat caps honor the per-attribute declaration (metatype modifiers layer on top). *(Every SR metatype declares per-attribute `stat_caps` for all 9 attributes (human 6/edge 7 … troll body 10), and training enforces the race `StatCaps` at raise time (`training.go:544`), so an SR character caps correctly. Residual (LOW, moot for the MVP): the attribute-set's own default `cap` is NOT threaded into that path as the fallback for a race that omits a cap — it falls back to `DefaultRaceCap` 25 — but no shipped SR metatype omits a cap, so it never triggers. Fix-by: a future world whose races don't fully declare `stat_caps`.)*
- [x] WoT + starter-world boot **unchanged** (their declaration reproduces today's six exactly — a regression gate). *(`TestCorePack_ClassicSetMatchesEngineDefaults` + `SeedBaseFromSet(classic) == DefaultPlayerBase()`.)*
- [x] A save round-trips a non-default, non-six-sized attribute set. *(The ordered pair-list save already round-trips any key set; proven at the loader/registry layer — an actual `shadowrun` save lands with the pack in SR-M3.)*

**Why first:** SR5's 8-primary + Edge set can't even be *seeded* without it, and it future-proofs *any* generic/point-buy pack — a reusable engine win independent of SR. (Note: because the set is now variable-size, the `score` sheet must render N attributes, not a fixed six-row grid — a real display change, not just relabeling.)

### SR-M2 — Typed damage: `type` + `target_pool`  · **Small Go · reusable by WoT too**

The damage struct gains two fields (plan §4.3): a damage **`type`** (feeds type-specific `mitigation` → Ballistic vs Impact) and a **`target_pool`** (which monitor an attack fills → Physical vs Stun). Physical-overflow → death; Stun-overflow → the shipped `unconscious` condition.

> **STATUS: SHIPPED 2026-07-06** (`45ef447` typed-damage `target_pool` routing → `6ea7d5d` weapon `target_pool` field). Proven live end to end by the SR-M3 combat tests — `TestLive_ShadowrunLethalKill` (Physical route + soak) and `TestLive_ShadowrunStunKnockout` (Stun route → KO). Stun-overflow→Physical shipped as SR-M3c-3. Per-type Ballistic/Impact mitigation reuses the WoT S1 typed-resistance path; the SR pack deliberately collapses armor to a single rating (§3), so the per-type *split* isn't exercised in SR content, but the capability is present and SR weapons do carry a `damage_types` (piercing/slashing/…). See `sr-m2-deferred-fixes` for the one open MED (non-atomic named-pool liveness, fine under serial dispatch).

**Acceptance criteria**
- [x] A weapon/attack declares a damage `type`; the `mitigation` channel can resist per-type (`armor_ballistic` vs `armor_impact`). Reuse the existing `TypedResistance`/typed-mitigation path from WoT S1. *(mechanism = the reused WoT typed-mitigation path; SR content collapses to one armor rating so the split is unexercised in SR — capability present.)*
- [x] A weapon/attack declares a `target_pool`; damage routes to that pool (bullet → Physical, stun baton/fist → Stun). *(`45ef447`; both routes proven live.)*
- [x] A `target_pool` reaching zero carries a per-pool zero-meaning: Physical → death (existing path), Stun → apply `unconscious` (the subdual seam) rather than kill. *(`TestLive_ShadowrunStunKnockout`.)*
- [x] `VitalDepleted` carries the death-vs-KO flag so consumers branch correctly. *(`Crossing.Nonlethal`; per-crossing KO/death in `internal/combat`.)*
- [x] Untyped / default-pool attacks behave exactly as today (WoT/starter-world regression gate). *(full `-race` suite green; WoT/starter live tests pass.)*

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
- [x] A lethal weapon fills the Physical monitor; worn armor reduces it via `mitigation`. *(`TestLive_ShadowrunLethalKill` — katana kills the armored ganger through its body+armor soak → lootable corpse. The firearm+ammo path is now proven separately — see SR-M3d / `TestLive_ShadowrunFirearm`.)*
- [x] Cyberware equipped/removed shifts the sourced attribute (via `srckey`), visible on `score`. *(`TestLive_ShadowrunCyberware`: wired reflexes raises Reaction 3→5 on equip, restores on unequip. PURE CONTENT — a `cyberware` slot (max 3) + three implants (wired-reflexes→Reaction, muscle-replacement→Str/Body, cybereyes→Intuition) with item `modifiers`; the standard equip → `EquipmentSourceKey` → stat-block pipeline (equipment.go) applies/removes them, `score` reads the effective attribute. Essence cost is flavor text (SR-M4). Needed a `slots:` glob in the shadowrun manifest — content globs are explicit, not directory-convention.)*
- [x] Nuyen is earned/spent at a shop. *(earn: auto-credited from looted ganger/sec-guard corpses. spend: `TestLive_ShadowrunNuyenShop` — a street fixer on the safe corner (`mobs: [fixer]`, `properties.shop.sells`) lets a runner `list` + `buy clip` for 24 nuyen (500→476), item lands in inventory. Standard shop service, no bespoke SR economy code.)*
- [x] Advancement runs on the existing engine (karma-as-XP, D3) — a kill grants karma, a track advances. *(`TestLive_ShadowrunKarmaAdvance`: a ganger kill banks 30 on the street-samurai's `street`/"The Long Run" track, and crossing 100 XP advances it to Level 2. REQUIRED an engine fix — a character's primary track (kill-XP target + `score` headline) now derives from its class `bound_track`, not the global `DefaultXPTrack="adventurer"`. Previously an SR/WoT character earned + displayed the core `adventurer` track and its own world track was inert.)*

**Why third:** with M1+M2 in place this is *mostly content*, and it's the validation gate — a genuinely playable, if simplified, Street Samurai.

### SR-M3d — Firearm + ammo: the in-room firefight tail  · **Small (mostly proof + one content call)**

> **STATUS: SHIPPED 2026-07-08.** As scoped, this was finish-and-prove, not greenfield: the engine's ranged/ammo system already shipped and was fully wired (the `AmmoFor` round-loop hook `main.go:2064` → `session.AmmoConsumer` spends one ammo unit per projectile swing and skips the swing dry via `OnRangedDry`; range bands + melee-band penalty; `throw`/`shoot`/`load` verbs), and the SR firearms were already authored (`heavy-pistol`/`smg` `ranged_class: projectile` + `ammo_kind: bullet`). `TestLive_ShadowrunFirearm` proves it live — an empty heavy pistol clicks dry in melee ("no bullet left to shoot"), then fed with rounds it fires point-blank and hits the ganger on the Physical monitor (soak applied, no target_pool). The clip-vs-round call resolved **Option A** (per-shot round): `caseless-round` renamed to "a caseless round" (each stackable unit = one shot, the arrow model; id kept so shop/room refs hold). The **magazine model (Option B)** shipped as its own slice — **SR-M3e** below; SMG burst-fire and SR cross-room `shoot` remain deferred.

**The slice:**
1. **Live proof (the deliverable).** `TestLive_ShadowrunFirearm`: a Street Samurai wields the heavy-pistol, holds a stack of `bullet` ammo, and kills the ganger IN-ROOM (single district = melee band, so firing takes the melee-band penalty — SR5 allows a gun in melee at a penalty). Assert: (a) the ammo count decrements per shot; (b) lethal fire lands on the Physical monitor (default route) through the target's soak — the katana proof, now via a firearm; (c) with ammo exhausted the next shot clicks empty (`RangedDry` → swing skipped + dry-fire narration). Fix whatever it surfaces (e.g. the AmmoConsumer not resolving the SR clip, or the melee-band penalty stalling the fight — buff/`restore` as the other SR live tests do).

2. **The one content decision — clip vs round.** Today the AmmoConsumer spends ONE `caseless-round` ITEM per shot, so a "clip" reads as a single bullet.
   - **A — per-shot round (MVP lean):** treat each stackable ammo unit as one round; rename `caseless-round` → "a box of caseless rounds" so a stack = rounds, one burned per shot. Reuses the bow/arrow model exactly. **Zero engine change** — a content rename + the test.
   - **B — magazine capacity (post-MVP, own slice):** a clip = N loaded rounds; a `reload` flow consumes a clip and loads N onto a per-weapon loaded-count (distinct from the crossbow's single-shot `ReloadTicks`); each shot decrements; empty → reload. The authentic SR model, but it needs a new weapon-state field + a reload verb/flow — **Medium Go.**
   Recommend **A** for the MVP firefight; defer **B**.

3. **Verify melee-band firing** (likely already works): a pistol at the melee band fires with `MeleeBandPenalty`; confirm the SR firefight resolves rather than stalling on the penalty.

**Deferred to their own slices (post-MVP polish):** magazine capacity + reload flow (B above); SMG **burst / full-auto** (multiple shots per action, proportionally more ammo — a new action shape); the `shoot` **cross-room** model for SR (corp-plaza sniping across the district — the range bands + Model C already support it; SR content just doesn't place a cross-room engagement yet).

**Acceptance criteria**
- [x] A Street Samurai wields the heavy-pistol and fires it at the ganger in-room; each shot spends one `bullet` unit. *(`TestLive_ShadowrunFirearm`; consumption proven by the empty click after depletion.)*
- [x] With ammo exhausted, the shot clicks empty (`RangedDry`) and is skipped, narrated to the player ("no bullet left to shoot").
- [x] Lethal fire lands on the Physical monitor (default route) with the target's soak applied — a firearm hit deals damage like the katana proof. *(the pistol has no target_pool → Physical; hits observed through the ganger's body+armor soak.)*
- [x] Clip-vs-round semantics resolved — **Option A**, per-shot round (`caseless-round` → "a caseless round").

### SR-M3e — Magazine model + reload (Option B)  · **Medium Go · SHIPPED 2026-07-08**

> **STATUS: SHIPPED.** The Option-B magazine model deferred by SR-M3d, built as its own slice. A firearm now carries a **magazine capacity** (`magazine:` on the weapon; the Predator V = 15, SR5 "15 (c)") and a `reload_method:` (only `clip` consumed so far). The loaded-round count lives as a mutable **instance property** (`loaded`) on the weapon: firing decrements it via the session ammo hook (`ConsumeAmmo` is magazine-aware — combat is untouched), an empty magazine is a dry click, and the player-facing **`reload`** verb refills it from carried rounds. The count **persists** across relog as `EquippedItem.Loaded`/`InventoryEntry.Loaded` (additive `omitempty` pointer — no save-version bump, an old save reads lazy-empty). The admin script-reload verb was renamed **`reloadscripts`** to free `reload` for the weapon; `load` still chambers a crossbow. This slice also renamed the demo firearm to the canonical **Ares Predator V** (content). Proven live: `TestLive_ShadowrunFirearm` (empty → dry → reload → fire+hit) and `TestLive_ShadowrunMagazinePersist` (a full 15/15 magazine survives quit + relogin).

**Content:** `magazine` + `reload_method` weapon fields; `content/shadowrun/items/heavy-pistol.yaml` → `ares-predator-v.yaml` (`magazine: 15`, `reload_method: clip`).

**Deferred (own slices):** the reload **action cost** (SR5 Simple/Complex per the reloading table — reload is instant this slice); **typed rounds in a magazine** (masterwork/special ammo — the magazine holds an abstract count, not graded rounds); **real clip items** (spare pre-loaded clips vs. the abstract fill-from-loose-rounds model); SMG **burst/full-auto** (multi-round shots); SR **cross-room `shoot`**.

**Acceptance criteria**
- [x] A magazine weapon holds a loaded-round count on the item instance; firing decrements it and an empty magazine clicks dry. *(`TestLive_ShadowrunFirearm`.)*
- [x] `reload` refills the magazine from carried ammo of the weapon's kind (partial fill when short); reports the loaded/capacity count, "already fully loaded", or "no rounds". *(reload_weapon.go + `ReloadWieldedMagazine`.)*
- [x] The loaded count persists across relog (a loaded gun stays loaded). *(`TestLive_ShadowrunMagazinePersist`; `EquippedItem.Loaded`.)*
- [x] The player `reload` verb doesn't collide with the admin script-reload (renamed `reloadscripts`); `load` still serves the crossbow. *(builtins.go.)*

**Size: Small** — landed as a live test + a content rename. The magazine model (B), burst-fire, and SR cross-room are separate slices.

### SR-M3f — Ammo holders + the unified reload (Tier B-lite)  · **Medium–Large Go · COMPLETE**

> **STATUS: SR-M3f COMPLETE 2026-07-09 — SR-M3f-1 (holder model) + SR-M3f-2 (shop
> SKUs, ejected-clip decay, grade-through-holder) all SHIPPED.** SR-M3f-1 landed the
> holder model live: the Ares Predator V is holder-fed (`accepts_holder:
> heavy-pistol`), a new `predator-clip` holds rounds, the unified `reload`
> fills a clip (`reload clip`) / inserts a clip (`reload`) / ejects the spent
> clip to the ground, firing draws from the inserted clip, and the inserted clip
> persists across relog (`EquippedItem.Holder`, no save bump). Loose ammo can no
> longer be consumed as a holder or vice-versa (`HolderFits` guard). Proven live:
> `TestLive_ShadowrunFirearm` (clipless→dry→fill→insert→fire), `TestLive_
> ShadowrunMagazinePersist` (inserted clip survives relogin → fires),
> `TestLive_ShadowrunReloadPlaytest` (fill/insert/eject/persist walkthrough).
> **SR-M3f-2** (below) — ejected-clip decay, shop SKUs
> (loaded/empty clips), grade-through-holder — is the remaining slice. Behavior
> contract:
> `docs/specs/ammo-and-reloading.md`. This makes ammunition physical: rounds live
> in removable **holders** (clip/magazine/belt), holders go *into* the weapon,
> firing draws from the inserted holder, and `reload` swaps holders — the spent
> one **ejects** to the ground (recoverable, then decays). It is partly a
> **refactor of SR-M3e**: the loaded-round count moves off the gun instance and
> onto a holder item. Internally-fed weapons (a revolver's cylinder) keep the
> SR-M3e loose-round model unchanged. Design decisions are settled in the spec
> (Tier B-lite; recoverable-then-decay; the unified `reload`; homogeneous
> holders).

**The two slices.**

**SR-M3f-1 — Holders as items + insert + fire-from-holder (the refactor).**
- **Holder item** — a constrained container (`inventory-equipment-items`): content
  fields for capacity, accepted round kind, and the weapon **family it fits**;
  a loaded-round count on the instance (generalize SR-M3e's `loaded` property +
  `MagazineLoaded`/`SetMagazineLoaded` from "the gun" to "the holder").
- **Holder-fed weapon** — declares it accepts a holder family (e.g. `accepts_holder`),
  distinct from an internally-fed weapon's own `magazine`. `combat.Stats` /
  `weaponInfo` carry it as SR-M3e did the magazine fields.
- **The inserted-holder relationship** — a holder-fed weapon references its
  inserted holder (a gun→holder link; either a dedicated field on the weapon
  instance or the `Contents` nesting, spec §9). Firing (`ConsumeAmmo`) draws from
  the inserted holder's count instead of the gun's own; empty inserted holder →
  the existing dry path.
- **`reload` unification** (spec §3): no-arg on a holder-fed weapon selects a
  compatible loaded holder from inventory, inserts it, **ejects** the prior
  holder to the room (basic eject here; decay in -2); `reload <holder>` fills a
  named holder from loose rounds; internally-fed weapons keep the SR-M3e fill.
- **Persistence** — the hard part beyond SR-M3e: a holder's load persists whether
  it's a loose inventory item (extend the `InventoryEntry.Loaded` path) OR
  **inserted in a weapon** (the `EquippedItem` must carry its inserted holder's
  template + load, or the inserted holder is its own record). No save-version bump
  if it stays additive/`omitempty` like SR-M3e.

**SR-M3f-2 — Ejection decay + shop SKUs + grade-through-holder.**
- **Shop SKUs — SHIPPED 2026-07-09.** A `preload` holder field seeds a holder's
  rounds at spawn (content → loader (clamped to capacity) → template →
  instance-construction). New `predator-clip-loaded` (preload 15); the fixer now
  stocks a **loaded clip** (primary buy, arrives full), **loose rounds** (refills),
  and an **empty clip** (cheap spare) — distinct keywords so `buy loaded` vs
  `buy clip` disambiguate. Live: `TestLive_ShadowrunLoadedClipShop` (buy loaded →
  insert → 15/15, no fill).
- **Decay — SHIPPED 2026-07-09.** An ejected clip lingers recoverable, then a
  sweep removes it. New `internal/scrap` (a `TagScrap` runtime tag + drop-tick
  property + `Sweep`); `ItemInstance.AddTag`/`HasTag` added; the eject marks the
  clip (`scrap.Mark`); a `scrap-decay` tick handler (shares the corpse-decay
  cadence) + `ANOTHERMUD_EJECTED_HOLDER_LIFETIME` (default 3m). Unit tests
  `internal/scrap` (expiry / picked-up-skip / clock-skew) + live
  `TestLive_ShadowrunClipDecay` (eject in the back alley → lingers → decays). A
  picked-up clip is skipped by the sweep (Placement.Remove single-winner); the
  stale-tag-on-re-drop edge is a recorded LOW.
- **Grade-through-holder — SHIPPED 2026-07-09.** A homogeneous holder captures
  its rounds' grade at fill and applies it per shot, **retiring the deferred
  "typed/masterwork ammo in a magazine" tail.** Added SR grade content
  (`content/shadowrun/grades/` — `match` +1 / `apds` +2 to-hit; a `grades:` glob
  loading before items) + a graded `apds-round` (the fixer sells it). Grade path:
  `pullAmmoLocked` returns the pulled rounds' grade → `FillHolder` sets it on the
  holder (`HolderAmmoGrade`) → `InsertHolder` moves it onto the inserted-holder
  state → `ConsumeAmmo` returns it → the round loop maps it to a to-hit bonus.
  Persists on both the inserted clip (`EquippedHolder.Grade`) and a loose clip
  (`InventoryEntry.Grade`); an ejected clip keeps its grade. Simplification: the
  holder is homogeneous — the grade is set at fill-from-empty and kept on a top-up
  (mixing a different grade keeps the first, a known edge). Deterministic
  `TestGradeThroughHolder` (fill APDS clip → insert → `ConsumeAmmo` returns
  "apds"). **SR-M3f-2 complete.**

**Follow-ons:** reload as a **timed action** — **SHIPPED 2026-07-09** (a busy
action via `action-economy`, `ANOTHERMUD_RELOAD_DURATION` default 1s / 0=instant;
begin → async-complete, a mid-reload action refused as busy; `TestLive_
ShadowrunReloadTimed`). Still deferred (own slices, spec §11): reload
**per-method** Simple/Complex differentiation; **mixed-ammo** holders (homogeneous
assumed); **speed-loaders / belts** as holder sub-behaviors; compatibility
strictness (weapon-family vs exact-id); auto-select order among several loaded
holders.

**Acceptance criteria** (from `ammo-and-reloading`)
- [ ] A holder-fed weapon fires from an inserted holder; `reload` inserts a
      compatible loaded holder and ejects the prior one (with its remaining rounds).
- [ ] `reload <holder>` fills a named holder from loose rounds; a holder rejects a
      round kind different from what it holds.
- [ ] An ejected holder lands in the room recoverable, then decays.
- [ ] A holder's load (and grade) persists across relog, carried or inserted.
- [ ] Masterwork/special rounds confer their grade when fired from a holder.
- [ ] Internally-fed weapons (the revolver path) are unchanged from SR-M3e.

### SR-M4 — Essence pool + `degrades: magic`  · **Small Go · SHIPPED 2026-07-13**

`pool.Rules.Degrades` is built but used by nobody (plan §4.4). An `essence` pool whose `current` clamps a `magic` channel max is the textbook use. **Not required for a mundane Street Samurai** — sequenced now because the cyberware-as-stat-boost content (SR-M3) already exists and this turns its flavor-text Essence cost into a real, bounded budget.

**Implementation notes (as built)**
- Essence is stored in **tenths** (max 60 == 6.0) so the integer `pool.Pool` can hold the SR decimal. `item.essence_cost` is authored as the decimal (2.0, 0.2) and converted ×10 at load; `score` renders it back as `X.X`.
- Essence is **derived, not spent**: `current = max − Σ(essence_cost of installed cyberware)`, recomputed in `connActor.recomputeWeaponLocked` → `setEssenceInstalledLocked` on every equip/unequip. Installing chrome lowers it; removing chrome restores it (symmetric with the reversible cyberware slot). No spend/regen tick touches it.
- The **equip gate** refuses an install that would exceed the remaining budget (`internal/command/equipment.go`), only where an essence pool exists — `essence_cost` stays inert in any non-Shadowrun world.
- `degrades: magic` is honored (`applyEssenceDegradesLocked`) by capping a same-Set `magic` pool's max at Essence current (a ratchet). **Inert for a mundane runner** (no magic pool seeded → no target). The Awakened `magic` pool, its units/rounding, and full essence-hole-vs-restore semantics are the mage arc's to settle; the honoring hook + its regression test (a synthetic magic pool) prove the mechanism.

**Acceptance criteria**
- [x] An `essence` pool declared in content, starting at 6 (`content/shadowrun/pools/essence.yaml`, max_formula "60").
- [x] Cyberware install lowers Essence; `pool.degrades: magic` caps the `magic` pool max at recompute time (honoring hook, tested with a synthetic magic pool).
- [x] A mundane character (Magic 0) is unaffected — pure regression (no magic pool → the degrades hook is a no-op; `TestEssence_*` + live `TestLive_ShadowrunEssence`).

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
