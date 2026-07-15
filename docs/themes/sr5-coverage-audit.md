# Shadowrun 5th Edition — Coverage & Fidelity Audit

**Date:** 2026-07-15 · **Reference:** `ShadowMaster/data/editions/sr5/core-rulebook.json`
(SR5 Core, Catalyst Game Labs, 2013 — 29 rules modules) · **Audited against:** the
`content/shadowrun` pack + the SR-relevant engine (`internal/*`) as of this date.

This is a snapshot audit: which SR5 systems our MUD covers, and — for the ones we
built — how faithful they are to the reference. It is a **derived checklist**, not
a spec. Regenerate it after major SR work rather than editing counts by hand.

---

## How to read this

The reference is the **complete SR5 core ruleset**. We deliberately built a
**street-samurai slice** on a **ruleset-agnostic engine that translates SR5**
rather than reimplementing it. Per **Decision 0** (`wot-mechanics-epic.md`) the
engine resolves combat as a **dice + soak-channel tick model**, *not* SR5's
**DV / AP / dice-pool** system. So judge on two axes:

- **Coverage** — does the system exist at all?
- **Fidelity** — where we built it, how close are the *values* (attribute caps,
  costs) and *mechanics* to SR5?

**Headline:** the *mundane street-samurai quadrant* is built and, where it counts,
**remarkably faithful** (metatypes are exact to canon). The three **Magic**,
**Matrix**, and **Rigging** pillars plus the **skill system** are the large
untouched surface — which matches the greenfield cluster the roadmap already flags
(`shadowrun-mvp.md`: SINs, Matrix, mage).

Status legend: ✅ built · 🟡 partial / analog · ❌ absent.

---

## Coverage matrix

| SR5 module (reference count) | Status | Notes / fidelity |
|---|---|---|
| **metatypes** (5: Human/Elf/Dwarf/Ork/Troll) | ✅ all 5 | **Exact** — our stat_caps + stat_bonuses reproduce SR5 attribute min/max to the number (Troll: Body/Str max 10 & min 5 via +4, Agi 5, Rea 6, Wil 6, Log/Int 5, Cha 4, Edge 6, Essence 6.0) |
| **attributes** (8 primaries + Edge) | ✅ full | Body/Agility/Reaction/Strength/Willpower/Logic/Intuition/Charisma + Edge — the `shadowrun-primaries` attribute set |
| **advancement** (karma) | ✅ analog | Karma-as-XP (`shadowrun-mvp.md`) |
| **cyberware** (82-item catalog + grades + rules) | 🟡 ~12 implants | Wired reflexes, muscle replacement, cybereyes + vision mods (low-light / thermographic / ultrasound / magnification / vision-enh), dermal plating, reaction enhancers, cyberarm, smartlink. **Essence budget (tenths, max 6.0) + grades are present** (SR-M4). Attribute mods faithful; Essence *costs* are approximated |
| **gear / weapons** (86) | 🟡 ~12 | Ares Predator V / Alpha / Light Fire 70, Ingram Smartgun X, generic SMG; katana, sword, stun/extendable baton. **Costs match** (Predator 725¥), ammo capacity matches (15). Firearms have real depth: magazines→holders→reload, firing modes, recoil comp, AP + APDS |
| **gear / armor** (category) | 🟡 ~10 | Armored jacket, vest, full-body, lined coat, clothing + specialty (chameleon suit, coldsuit, rad-suit, urban-explorer). Folded into the **soak / mitigation** channel |
| **gear / ammunition** | 🟡 ~4 | Caseless, APDS, predator clips. The holder-fed magazine model |
| **modifications** (+ categoryModificationDefaults) | 🟡 analog | Our `item-modification`: armor capacity, weapon mount slots, cyberware clusters — a parallel to SR's modification system |
| **skills** (74 active + groups + knowledge/languages) | ❌ **architectural gap** | We use **use-based proficiency** + weapon proficiency tiers, *not* the ranked 74-skill list (Blades, Automatics, Spellcasting, Hacking, Piloting…). Deliberate (`skills.md`), but it means no skill-rank play |
| **qualities** (39 positive / 44 negative) | 🟡 partial analog | Feats + backgrounds cover *some* of the same ground; not the SR pos/neg quality catalog (Ambidextrous, Bad Luck, SINner, …) |
| **priorities / creationMethods** | 🟡 partial | Interactive creation wizard + per-pack backgrounds; **not** the A–E priority table (metatype / attributes / magic / skills / resources) |
| **actions** (combat/general/magic/matrix/social/vehicle) | 🟡 combat only | Our tick/round combat model; the magic/matrix/social/vehicle action classes are absent |
| **magic** (81 spells + traditions/rituals/mentorSpirits/foci/paths) | ❌ **none** | The entire Awakened pillar — no mage, no spellcasting |
| **adeptPowers** (31) | ❌ none | No adepts |
| **spirits** (53) | ❌ none | No summoning / spirit catalog |
| **programs** (34) + cyberdecks + complexForms + sprites + livingPersona | ❌ **none** | The entire **Matrix** pillar — no decker / technomancer |
| **vehicles** (66) | ❌ **none** | The entire **Rigging** pillar — no rigger / drones / piloting |
| **critters** (117) + critterPowers + critterWeaknesses | ❌ none | Mobs are our own authored set; no SR critter catalog |
| **bioware** (catalog) | ❌ none | No bioware track (distinct from cyberware; different essence math) |
| **gear / drugs + toxins** | 🟡 partial | Toxins ≈ our biome hazards (Glow radiation, ash toxins); no drug catalog (Kamikaze, Jazz, …) |
| **lifestyle** (4) | ❌ none | No monthly lifestyle upkeep / nuyen sink |
| **contactArchetypes / contactTemplates / favorServices** | ❌ none | No contact network / favor economy |
| **foci** (magic gear) | ❌ none | Magic-dependent |
| **socialModifiers / diceRules / gameplayLevels** | 🟡 N/A | We use our own dice model; SR's dice/limit rules don't map 1:1 |
| **availability / legality** (on every gear item) | ❌ none | We model neither the availability rating nor legality (restricted/forbidden) — no black-market gating |

---

## Fidelity — where we built it, how does it look?

**Very faithful on content *values*, deliberately loose on the *resolution system*.**

- **Metatypes — exact.** Attribute min/max and Essence base match SR5 to the
  number. This is our strongest-fidelity area; the metatype content is authored
  straight to canon.
- **Weapons — costs & capacities match, combat stats translated.** Ares Predator V:
  cost 725¥ ✅, ammo 15 ✅; but SR's `8P` / AP `-1` / Accuracy 5 → our `2d6` /
  `ap:1` / dice model. The item YAML documents the SR canon line and notes the
  translation explicitly.
- **Cyberware — mechanics faithful, Essence real, costs approximate.** Equip →
  sourced-attribute shift is real and visible on `score`; Essence is a real
  derived budget with an equip gate; grades exist. Per-implant Essence *costs* are
  approximated rather than lifted from the 82-item catalog.
- **The core divergence is intentional.** SR5's DV / AP / dice-pool / limits →
  our dice + soak channels (Decision 0). Rules-fidelity is loose *by design*;
  value-fidelity (caps, costs, capacities) is high.

**Not modeled at all on items we do have:** `availability`, `legality`,
`accuracy`, and (for cyberware) canonical Essence cost — all present on every
reference entry.

---

## The four big gaps, quantified

1. **Skills** — 0 of 74. Proficiency substitutes; a real gap for skill-gated play
   and for the magic/Matrix skills that don't exist yet.
2. **Magic** — 0 of 81 spells + 31 adept powers + 53 spirits + traditions / foci /
   mentor spirits. No Awakened characters.
3. **Matrix** — 0 of 34 programs + cyberdecks / complex forms / sprites. No decker
   or technomancer.
4. **Rigging** — 0 of 66 vehicles / drones. No rigger.

Supporting gaps: priority-based creation, the qualities catalog, lifestyle upkeep,
contacts / favors, bioware, the drug catalog, and availability / legality economy.

---

## Bonus — ready-made content in the reference to mine

The ShadowMaster tree ships more than the rulebook; these are directly usable when
we build the corresponding systems:

- `sr5/archetypes/` — street-samurai, decker, rigger, face, combat-mage, adept,
  technomancer (creation blueprints).
- `sr5/example-characters/` — 16 fully statted PCs (mob / NPC stat-block source).
- `sr5/grunt-templates/` — pr0 street-rabble → pr6 dragon-guard (tiered mob
  templates, ideal for spawn tables).
- `sr5/sample-contacts/` — ready NPCs for a future contacts system.
- `sr5/run-faster.json`, `core-errata-*.json`, `rule-reference.json` — supplements.

---

## What this tells us

The slice we shipped is **faithful where it's built** — metatypes are canon-exact,
firearms have genuine depth, cyberware/Essence are real. The gaps are not sloppy
coverage; they are the **three unbuilt character pillars** (Magic, Matrix,
Rigging) and the **skill-system divergence**, all of which are known greenfield.

Prioritization is a product call, but the reference makes the shape clear: today a
player can build and play a **mundane street samurai** end-to-end; they cannot yet
be a **mage, decker, or rigger**. Closing any one pillar (plus the skill substrate
those pillars lean on) is what widens the playable-archetype set. When we take one
on, the reference's catalog + archetypes + grunt-templates are the content source.
