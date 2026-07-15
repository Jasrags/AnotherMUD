# Shadowrun Skills — Build Plan (the ~18-skill slice + per-weapon to-hit)

**Status:** Plan (build pending) · **Date:** 2026-07-15 · **Scope:** an
SR5-flavored **active-skill** system for the `shadowrun` pack — a content
catalog of ~18 skills with linked attributes / groups / categories, an SR-style
`skills` display, untrained **defaulting**, and the one real engine change:
**per-weapon skill drives to-hit** (Pistols ≠ Automatics), gained through use in
combat · **Audience:** anyone implementing or porting this.

This is a **content + display + combat-wiring** layer on the skill substrate that
already shipped — it is **not a skill-engine rebuild**. Read `docs/specs/skills.md`
first: that spec established the model this plan reuses. This doc lives in
`docs/themes/` (a setting mechanics program, like `shadowrun-mvp.md`); the
engine-level bits it introduces (skill groups/categories, defaulting, the
weapon-skill→to-hit contract) should be back-ported into `skills.md` as
behavior sections when built.

Source of truth for the SR5 values: `ShadowMaster/data/editions/sr5/core-rulebook.json`
(`modules.skills`) and the coverage audit `docs/themes/sr5-coverage-audit.md`.

---

## 1. What already exists (reused verbatim)

From `skills.md` (WoT EPIC S3), proven end-to-end by lockpicking:

- **A skill is an ability** (`type: passive`, `category: skill`) carrying a
  **use-based proficiency (1–100)** in `ProficiencyManager`, keyed by
  `(entity, skill id)`, that **gains with use** (`RollUseGain`) and is capped/
  raised by trainers via `practice`/`train`.
- It declares a **governing stat** (`gain_stat`) — the attribute whose modifier
  feeds the check and nudges the gain roll.
- **`ResolveSkillCheck(roller, bonus, dc)`** — `d20 + bonus ≥ dc`, natural-1
  fails / natural-20 succeeds, where
  `bonus = proficiency-bonus(proficiency) + AbilityModifier(governing stat)`.
- **Untrained use:** a non-`trained_only` skill checks at proficiency-zero
  (governing-stat modifier only); a `trained_only` skill is refused.

The SR5 mapping is one-to-one:

| SR5 | Our engine (today) |
|---|---|
| Skill rating 1–6 (karma-gated, cap 6) | use-based proficiency 0–100 with a **trainer-gated cap** as the ceiling; canon 1–6 → proficiency+cap at authoring time (D1) |
| Linked attribute | governing stat (`gain_stat`) |
| Skill + attribute vs. threshold | `d20 + proficiency-bonus + attribute-mod` vs DC (Decision 0 translation) |
| Defaulting (attribute − 1) | proficiency-zero check + a small penalty (§5) |
| Improve by karma/practice | use-based gain + trainer cap (karma-buy deferred) |

So Pistols, Sneaking, First Aid, and Locksmith are all "a `skill` ability with a
linked attribute + a use-based rating, resolved by the one check primitive" —
exactly how Open Lock already works.

---

## 2. Locked decisions

- **D1 — Keep the 0–100 use-based proficiency; no 1–6 rating relabel.** The
  **trainer-gated cap is the limit** (the SR rating-ceiling analog: you grind a
  skill to its cap through use, and a trainer/`train` raises the cap — the
  find-and-pay-a-trainer gate stands in for SR's karma cost + max rating). The
  **raw 0–100 value is the progression display** — it visibly ticks up with use,
  the feedback a use-based system exists to give. Together those deliver SR's
  *limited progression* **with better feedback than a cosmetic 1–6 clamp**, which
  would only hide the tick and add a display-vs-mechanics gap (two "Pistols 4"s
  that hit differently). SR canon stat blocks (stated 1–6) translate to a
  proficiency+cap **at authoring time** (like `8P → 2d6`), separate from display,
  so no cross-reference is lost. Nothing about the scale changes for any world.
  *(If a 1–6 tag is ever wanted, "show both" — `Pistols [63/80]` plus a derived
  tier — is a trivial additive later; not now.)*
- **D2 — Per-weapon skill drives to-hit** (§6). The wielded weapon declares its
  SR skill; to-hit adds that skill's proficiency bonus, and each attack trains
  it. This is the one real engine change and the core SR combat texture.
- **D3 — Breadth: the ~18 skills with real or near-term consumers** (§4).
  Magic/Matrix/vehicle skills are authored **with their pillar**, not now (they
  gate systems that don't exist); knowledge/GM-adjudicated skills stay dropped
  per `skills.md` non-goals.
- **D4 — Gain: use-based + trainer cap** (reused). Karma-buying skill ranks is
  deferred (rides SR-M5 karma-ledger; `shadowrun-mvp.md`).
- **D5 — Groups + categories are metadata for display + future bulk-training;**
  v1 uses them only for the grouped `skills` listing (no group-buy yet).

---

## 3. The skills display

No scale change (D1). `skills` shows the actor's known skills with their **0–100
proficiency + cap** — the same numbers as today — but **grouped SR-style**: by
**category** (combat / physical / social / technical) and then **skill group**
(Firearms, Close Combat, Stealth, …), with each skill's **linked-attribute** tag.
So a runner reads e.g.:

```
Combat — Firearms
  Pistols     63/80  (AGI)
  Automatics  22/80  (AGI)
Physical — Stealth
  Sneaking    41/60  (AGI)
```

The proficiency ticks up visibly with use; the cap is the trainer-gated ceiling
(D1). The grouping is pure presentation and is **orthogonal to the scale** — a
WoT/generic world's skill display is unchanged (it never declares SR groups /
categories, so it falls back to the flat list it shows today).

**Acceptance:** `skills` lists known skills with proficiency + cap, grouped by
category then group, with the linked-attribute tag; a world that declares no
group/category metadata renders the existing flat list unchanged.

---

## 4. The ~18-skill roster

Authored as `skill` abilities with the new metadata (§5). **Consumer** = what
rolls the check: *live* (a consumer exists today), *near-term* (a small
follow-on slice, §7 D), or *pillar* (waits for magic/Matrix/rigging — **not in
this plan**, listed only to show the catalog's shape).

| Skill | Linked | Group | Category | Consumer |
|---|---|---|---|---|
| Pistols | Agility | Firearms | combat | **live** — heavy/light pistols to-hit |
| Automatics | Agility | Firearms | combat | **live** — SMG / auto to-hit |
| Longarms | Agility | Firearms | combat | **live** — rifle / shotgun to-hit |
| Heavy Weapons | Agility | — | combat | near-term — LMG/launcher content |
| Blades | Agility | Close Combat | combat | **live** — katana / sword to-hit |
| Clubs | Agility | Close Combat | combat | **live** — baton to-hit |
| Unarmed Combat | Agility | Close Combat | combat | **live** — fists to-hit |
| Throwing Weapons | Agility | — | combat | **live** — grenade / knife throw (ranged-combat) |
| Sneaking | Agility | Stealth | physical | **live** — visibility hide/sneak |
| Perception | Intuition | — | physical | **live** — visibility search/spot |
| Gymnastics | Agility | Athletics | physical | near-term — climb/dodge consumer |
| Survival | Willpower | Outdoors | physical | near-term — biome hazard / forage |
| Locksmith | Agility | — | technical | **live** — the `pick` verb (was Open Lock) |
| First Aid | Logic | Biotech | technical | near-term — a `first aid`/heal verb |
| Armorer | Logic | — | technical | near-term — weapon/armor upkeep + item-mod |
| Con | Charisma | Acting | social | near-term — deception vs disposition |
| Negotiation | Charisma | Influence | social | near-term — shop haggling (price) |
| Intimidation | Charisma | — | social | near-term — coerce vs disposition |

**Locksmith = the existing Open Lock**, re-identified to the SR name (alias or
rename); no new pick logic. The eight combat skills + Sneaking + Perception +
Locksmith (**11**) have live consumers the moment this plan's Slices A–C land;
the other seven are authored so the catalog reads coherently and each gets a
tiny consumer in Slice D.

---

## 5. Engine extension — skill catalog metadata

The `skill` ability gains optional metadata (all default to "generic/none" so
existing skills like Open Lock are unaffected):

- **`linked_attribute`** — the SR linked attribute. Defaults to `gain_stat` when
  unset (Open Lock's `dex` already serves both); a separate field only because
  SR names it distinctly and a future skill may want check-stat ≠ gain-stat.
- **`skill_group`** — one of the 15 SR groups (Firearms, Close Combat, Stealth,
  …) or none. Display grouping now; bulk-training later.
- **`skill_category`** — combat / physical / social / technical (/ magical /
  matrix / vehicle for the dormant pillars). Top-level `skills` grouping.
- **`defaultable`** (bool, default true) + **`default_penalty`** (int) — untrained
  use rules (see defaulting below).

**Defaulting.** An untrained character attempting a `defaultable` skill checks at
**proficiency-zero minus `default_penalty`** (SR5's −1 default; config). A
`trained_only` skill (e.g. a future Spellcasting) is refused untrained — the flag
already exists in `skills.md`. This subsumes and generalizes the current combat
"non-proficient" case (§6).

**Acceptance:** a skill declares linked-attribute/group/category/defaultable;
`skills` lists them grouped by category then group; an untrained defaultable
skill resolves at `−default_penalty` off the attribute-only bonus; a trained_only
skill refuses untrained.

---

## 6. Engine change — per-weapon skill drives to-hit

**Today** to-hit is binary: a weapon outside the wielder's class proficiency
*set* takes a flat `DefaultNonProficientPenalty` (−4, the WoT weapon-identity §3
rule), applied through the `AutoAttackConfig.HitModAdjust` seam; the `attack`
channel supplies the attribute term (Agility, in SR). There is **no per-skill
rating** — a character is simply proficient or not.

**The change:** a weapon declares **`weapon_skill`** (its SR skill id — `pistols`,
`automatics`, `blades`, `unarmed`, …). To-hit becomes:

```
to-hit = d20
       + attack-channel term         (unchanged — Agility, via the channel map)
       + proficiency-bonus(attacker, weapon_skill)   (NEW — the skill rating)
       − defaulting penalty          (only if untrained in a defaultable skill)
```

- **Proficiency-bonus** is the *same* 1–100 → d20 mapping the skill check uses
  (`skills.md` §3), so a rating-5 Automatics specialist lands meaningfully more
  than a rating-1 dabbler — the core SR distinction.
- **Use-gain in combat:** each resolved attack rolls `RollUseGain` on the
  weapon's skill (a hit gains full, a miss half — mirroring the pick-attempt
  gain), so **fighting trains the specific weapon skill**, bounded by the class/
  trainer cap. Firing pistols raises Pistols, not Automatics.
- **Untrained:** if the attacker has zero proficiency in the weapon's skill and
  the skill is `defaultable` (all combat skills are), apply the defaulting
  penalty (§5) rather than refuse — you *can* fire a pistol untrained, badly.
- **Coexistence with the WoT model:** both ride the existing `HitModAdjust`
  seam and are content/config-driven, so they are **per-pack**. WoT worlds keep
  the binary proficiency-set −4 (weapon-identity §3); the SR pack wires the
  per-skill path. `proficiency_tier` (the coarse simple/martial gate) is retained
  as the **class-grant + cap** mechanism (which weapon skills a class starts
  trained in, and how high); the new `weapon_skill` is the **fidelity unit** that
  feeds to-hit and gains with use. A weapon may declare both during migration.

**Acceptance:**
- A weapon with `weapon_skill: pistols` contributes `proficiency-bonus(pistols)`
  to its wielder's to-hit; a different weapon skill draws a different rating.
- Each attack trains the wielded weapon's skill through the existing use-gain
  (hit full / miss half), capped.
- An attacker untrained in a defaultable weapon skill attacks at the defaulting
  penalty, not refused; a WoT world's binary −4 path is unchanged.
- No regression for worlds that leave `weapon_skill` unset (fall back to the
  current proficiency-set behavior).

---

## 7. Build slices

- **Slice A — catalog + display.** The metadata fields (§5), the 18 skill
  ability YAMLs (§4), and the SR-grouped `skills` listing (§3). Skills are inert
  (no consumer wired yet) but visible, trainable, and gain-capable. Re-identify
  Open Lock → Locksmith.
- **Slice B — per-weapon to-hit** (§6, the engine change). `weapon_skill` on the
  item template + the SR weapon→skill map (Predator/Light Fire → `pistols`;
  Ares Alpha → `automatics`; Ingram/SMG → `automatics`; katana/sword → `blades`;
  batons → `clubs`; fists → `unarmed`; grenades/throwing knives → `throwing`);
  to-hit composition + combat use-gain; the WoT-coexistence wiring. **This is the
  slice that makes the skill system *felt*.**
- **Slice C — defaulting.** The `default_penalty` + `defaultable` gate on both
  `ResolveSkillCheck` (untrained checks) and the combat path (untrained weapon
  skill). Trained-only refusal.
- **Slice D — near-term consumers** (each a tiny follow-on, independent):
  Survival → biome-hazard/forage check; First Aid → a `first aid` heal verb;
  Con/Intimidation → a disposition check; Negotiation → shop-price haggling;
  Gymnastics → climb/dodge; Armorer → item-mod/upkeep gate. Author-order by
  which consumer we want first.

Slices A–C are the "skill system" proper; D fills in the non-combat consumers
over time. Magic/Matrix/vehicle skills join the catalog when those pillars land,
consuming the same primitive.

---

## 8. Configuration surface

| Policy | Where |
|---|---|
| Skill roster + per-skill linked attribute / group / category / defaultable | content (`content/shadowrun/skills/*` or abilities) |
| `skills` grouping (category → group) + linked-attribute tag; proficiency+cap shown as today | §3 (content metadata drives the grouping) |
| Proficiency → d20 to-hit / check bonus mapping | config (reused from `skills.md` §7) |
| `default_penalty` (untrained defaulting) | config (§5) |
| Per-weapon `weapon_skill` | content (item templates) |
| Combat use-gain rates (hit / miss) on weapon skills | config (reused `RollUseGain` params) |
| Per-class starting weapon-skill grants + caps (`proficiency_tier` retained) | content (classes/backgrounds) |

---

## 9. Open questions

- **Specializations** (SR +2 in a narrow slice — Pistols: Semi-Autos). Deferred;
  would ride a per-skill specialization tag + a situational check bonus.
- **Skill groups as a buy/raise unit.** v1 uses groups for display only; raising
  a whole group at once (SR skill-group purchase) waits for the karma-ledger
  (SR-M5) or a trainer-group verb.
- **Social-skill resolution surface.** Negotiation/Con/Intimidation need a
  *mechanical* target (a price delta, a disposition shift) — how much a check
  moves them is a per-consumer design in Slice D, and must dodge the
  "open-ended social roll" `skills.md` deliberately dropped.
- **Karma-buy vs. use-only.** This plan is use + trainer-cap; whether SR skills
  are *also* karma-purchasable rides SR-M5 (`shadowrun-mvp.md`) and is not
  decided here.
- **Knowledge / language skills.** Reuse the existing `languages` system; the SR
  knowledge-skill catalog (Arcana-as-knowledge, street knowledge) stays
  GM-adjudicated and out of scope, consistent with `skills.md` non-goals.
