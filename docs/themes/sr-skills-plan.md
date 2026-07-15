# Shadowrun Skills — Content Plan (the ~18-skill roster + weapon map)

**Status:** Plan (build pending) · **Date:** 2026-07-15 · **Scope:** the
**Shadowrun-specific content** for the active-skill system — the ~18-skill
roster (linked attributes / groups / categories), the SR weapon→skill map, and
the choice to use the **weapon-skill to-hit model** · **Audience:** anyone
authoring or porting the SR pack.

**This doc is content, not engine.** The skill *engine* — the catalog metadata
(linked attribute / group / category / defaultable), the grouped `skills`
display, untrained defaulting, and the **weapon-skill→to-hit** capability — is
**setting-agnostic** and specified in [`docs/specs/skills.md`](../specs/skills.md)
(§2.1, §5, §7). It is shared by every world. What is *Shadowrun* here is only the
content below, plus one per-pack model choice.

Source of the SR5 values: `ShadowMaster/data/editions/sr5/core-rulebook.json`
(`modules.skills`); coverage context: [`sr5-coverage-audit.md`](sr5-coverage-audit.md).

---

## 1. What is actually Shadowrun-specific (vs. shared)

The skill system is almost entirely shared engine. If we build this as planned,
**Shadowrun and WoT differ in exactly two things:**

1. **Content** — their skill rosters and weapon→skill maps. SR declares
   Pistols/Automatics/Blades linked to Agility; WoT declares its own weapons and
   skills with its own links.
2. **One combat-model choice** — SR opts into the **weapon-skill to-hit model**
   (a weapon's bound-skill *rating* feeds to-hit, and combat trains it); WoT
   keeps the **binary-proficiency model** (proficient, or a flat penalty). Both
   are offered by the engine (`skills.md` §7) and chosen by content.

Everything else — the catalog metadata, the grouped display, defaulting, the
skill-check primitive, the 0–100 proficiency + trainer-gated cap — is the *same
engine* for both. There is **no bespoke SR skill path**; this plan only supplies
SR content and flips one switch.

---

## 2. Rating: no relabel (uses the shared model)

Per `skills.md` §2 / §2.1 the scale is **0–100 use-based proficiency with a
trainer-gated cap as the ceiling**, for all worlds. SR does **not** relabel to
1–6: the cap is the limit (the SR rating-ceiling analog — grind to cap, a
trainer raises it, standing in for karma cost + max rating 6), and the raw 0–100
value is the progression display (it ticks up with use). SR canon stat blocks
(stated 1–6) translate to a **proficiency + cap at authoring time** — the same
kind of translation as `8P → 2d6` for damage — never a display remap. (A 1–6
tag, if ever wanted, is a trivial additive "show both" later; not now.)

---

## 3. The ~18-skill roster

Authored as `skill` abilities with the shared catalog metadata (`skills.md`
§2.1). **Consumer** = what rolls the check: *live* (a consumer exists today),
*near-term* (a small follow-on, §6), or *pillar* (waits for magic/Matrix/rigging
— **not in this plan**, listed only to show the catalog's shape).

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
| Locksmith | Agility | — | technical | **live** — the `pick` verb (= existing Open Lock, re-identified) |
| First Aid | Logic | Biotech | technical | near-term — a `first aid`/heal verb |
| Armorer | Logic | — | technical | near-term — weapon/armor upkeep + item-mod |
| Con | Charisma | Acting | social | near-term — deception vs disposition |
| Negotiation | Charisma | Influence | social | near-term — shop haggling (price) |
| Intimidation | Charisma | — | social | near-term — coerce vs disposition |

The eight combat skills + Sneaking + Perception + Locksmith (**11**) have live
consumers the moment the engine capabilities (`skills.md` §2.1/§5/§7) land; the
other seven are authored so the catalog reads coherently, each gaining a tiny
consumer in §6. Magic/Matrix/vehicle skills join the catalog **with their
pillar**, consuming the same primitive.

---

## 4. The weapon → bound-skill map

Under the weapon-skill model (`skills.md` §7), each SR weapon binds the skill
whose rating feeds its to-hit and trains on use:

| Weapon(s) | Bound skill |
|---|---|
| Ares Predator V, Ares Light Fire 70 | Pistols |
| Ares Alpha, Ingram Smartgun X, generic SMG | Automatics |
| (rifles / shotguns, when authored) | Longarms |
| Katana, sword | Blades |
| Stun baton, extendable baton | Clubs |
| (fists / unarmed) | Unarmed Combat |
| Grenades, throwing knives | Throwing Weapons |

The class/background grants starting proficiency in some of these skills (the
existing `proficiency_tier` remains the class-access + cap mechanism); a weapon
whose skill the wielder hasn't trained defaults at the penalty (`skills.md`
§2.1/§7). Firing a pistol raises Pistols, not Automatics.

---

## 5. Build slices (SR content)

The engine work is a separate track (`skills.md` §2.1/§5/§7 → its own slices).
This plan's slices are the **SR content** that lands on top:

- **Slice A — the roster.** The ~18 skill ability YAMLs with linked attribute /
  group / category / defaultable (§3). Re-identify Open Lock → Locksmith. Grant
  the combat skills through the Street Samurai class/background.
- **Slice B — the weapon map.** Bind each SR weapon to its skill (§4) and select
  the weapon-skill to-hit model for the pack. This is the slice that makes the
  system *felt* — Pistols ≠ Automatics in a gunfight, and combat trains the
  specific skill.
- **Slice C — near-term consumers** (each a tiny, independent follow-on):
  Survival → biome-hazard/forage; First Aid → a `first aid` heal verb;
  Con/Intimidation → a disposition check; Negotiation → shop-price haggling;
  Gymnastics → climb/dodge; Armorer → item-mod/upkeep gate. Author-order by
  which consumer we want first.

Slices A–B require the engine capabilities to exist first; C is incremental.

---

## 6. Open questions (Shadowrun-specific)

- **Social-skill resolution surface.** Negotiation/Con/Intimidation need a
  *mechanical* target (a price delta, a disposition shift); how much a check
  moves it is a per-consumer design in Slice C, and must avoid the "open-ended
  social roll" `skills.md` deliberately dropped.
- **Specializations** (SR +2 in a narrow slice — Pistols: Semi-Autos). Deferred;
  an engine feature (`skills.md` open question) with SR content on top.
- **Karma-buy vs. use-only.** This plan is use + trainer-cap (the shared model);
  whether SR skills are *also* karma-purchasable rides SR-M5
  (`shadowrun-mvp.md`) and is not decided here.
- **Knowledge / language skills.** Reuse the existing `languages` system; the SR
  knowledge-skill catalog stays GM-adjudicated and out of scope, consistent with
  `skills.md` non-goals.
