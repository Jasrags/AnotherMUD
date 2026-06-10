# EPIC: Wheel of Time Mechanics

**Status:** Program/EPIC — for alignment, pre-spec · **Type:** multi-milestone program (not a single milestone)
**Companion:** [`wot-world-plan.md`](wot-world-plan.md) (the *content/geography* track — areas, rooms, NPCs) · this doc is the *mechanics/systems* track
**Source:** the WoT RPG sourcebook extracts under [`docs/wot/`](../wot/) (classes, backgrounds, abilities, feats, skills, combat, equipment, the-one-power, encounters, heroic-characteristics, …)
**First sub-epic already drafted:** [`docs/proposals/combat-equipment-depth.md`](../proposals/combat-equipment-depth.md)

---

## 1. The framing (read this first)

The WoT RPG is **d20 / D&D 3e**. AnotherMUD's combat is a **real-time, tick-driven, chance-based** engine: no initiative order, no per-round action economy, no d20 to-hit roll, no AC/saving-throw rolls, melee-only same-room. A large fraction of what the sourcebooks describe is **tabletop scaffolding that a real-time MUD should deliberately NOT port**:

- Initiative & turn order, the action economy (standard/move/full-round/free, 5-foot steps), attacks of opportunity, grid flanking geometry, ready actions — these presuppose turn-based tactical play. The engine's tick loop replaces all of it.
- Take-10 / Take-20, GM adjudication, encounter challenge-codes, save-vs-check distinctions — pure table procedure.
- Literal d20 rolls for everything — the engine resolves hits/saves by probability, not by surfacing a die.

**The meta-decision that governs the whole program (Decision 0): ✅ DECIDED 2026-06-10 — posture A.** Keep the tick/chance model and **translate WoT mechanics' *flavor + meaningful choices* into it** — additive systems on the existing engine, **no d20 substrate rewrite.** The goal is *a fun WoT game*, not d20 fidelity. The rejected alternative (B) would have rewritten combat to a d20 round/roll/action-economy model that every other system then depended on; turn-based tactics fit a real-time MUD poorly. Every sub-epic below is therefore an **additive feature on a working engine.** (Scoped d20 *texture* — e.g. a visible to-hit number, or Fort/Reflex/Will surfaced as real resolved checks in S6 — remains a per-sub-epic choice, not a substrate swap.)

**What "translate, don't port" means in practice:** keep the *choices that matter* (which weapon, which weave, proficient-or-not, what condition you're suffering, your reputation, your channeling risk) and the *flavor*; drop the *bookkeeping that only exists to run a tabletop session*.

---

## 2. The mechanic surface, clustered into candidate systems

Each row is a sub-epic — itself theme-sized or bigger. "Engine home" names the existing seam it extends; "State" is what exists today; "Fidelity" is the recommended translation posture. Detail lives in the cited `docs/wot/` file.

| # | Sub-epic | What it is (WoT) | Engine home / state | Fidelity rec | Size | On backlog? |
|---|---|---|---|---|---|---|
| **S1** | **Weapon & Equipment Depth** | proficiency tiers, crit, damage types, ranged, armor, size-wield | `item`/`combat`/`slot`; weapons are dice+mods only, melee-only | A+B+C identity slice first; ranged & armor separate | M (then L) | yes — `proposals/combat-equipment-depth.md` |
| **S2** | **The One Power (channeling)** | saidin/saidar, weaves, daily slots, affinities, talents, overchannel, linking, madness | `progression` abilities + `effect`; **mana/resource pool unbuilt** | MUD-idiomatic: weaves = abilities w/ a Power-pool resource + cooldowns; keep slot-budget & overchannel-risk *choices*, drop d20 cast rolls | **XL** | partial — "Mana pool" §2 is the substrate |
| **S3** | **Skills system** | ~40 d20 skills (Hide, Heal, Diplomacy, Craft, …), ranks, synergy, trained-only | `progression` **proficiency** (use-based gain) is the analog; no skill list | Translate to proficiency-style skills (use-based, not point-buy); pick the subset a MUD actually resolves | L | no |
| **S4** | **Feats / traits (passive perk engine)** | binary feats w/ prereqs, multi-take, stackable; class bonus feats | `abilities-and-effects` has passives; no perk-selection engine | A passive-trait selection engine layered on the abilities/effects substrate | L | no |
| **S5** | **Conditions & status effects** | prone, stunned, dazed, bleeding, frightened, fatigued, blinded, entangled, helpless, … | `effect` system exists & is the natural home | Extend effects with a WoT condition vocabulary + their combat hooks | M | no |
| **S6** | **Saves (Fort / Reflex / Will)** | three save axes vs poison/fear/area/mind | `stats` + `combat`; no save axis today | Add three derived save values + a resolve-check primitive (poison/fear/weaves consume it) | S–M | no |
| **S7** | **Survival & environment v2** | encumbrance, thirst, fatigue/subdual, temperature, poison, disease, falling, suffocation | `economy` sustenance/rest (single pool); container caps specced | Extend sustenance → multi-pressure survival; encumbrance rides container caps | M (per pressure) | partial — "thirst split" + "container caps" on backlog |
| **S8** | **Reputation & social standing** | reputation score, fame/infamy, NPC attitude shift, followers | none; `faction` (specced §1) is the sibling architecture | Reputation as a parallel signed axis like faction; NPC attitude as a disposition hook | M | adjacent — `faction` specced |
| **S9** | **Class / background / progression rebuild** | 7 classes, 12 backgrounds, multiclass, BAB, per-class HD, ability-ups, languages | `progression` tracks/classes/races exist; no multiclass/BAB/background | Map classes→existing class content; backgrounds→starting-loadout + trait grants (S4); **skip literal d20 leveling** unless Decision 0 = B | L | no |
| **S10** | **Travel & planes** | the Ways, Portal Stones, Tel'aran'rhiod, Skimming/Gateways, fast-travel | `portal` (M15.2 temporary exits); fast-travel waypoints on backlog | Ways/Stones as special exit networks + hazards; T'A'R as an alternate room-graph realm | L | partial — "fast-travel" §2 |
| **S11** | **Shadowspawn & advanced mob mechanics** | DR, regeneration, fear aura, gaze, Myrddraal link-death, light-sensitivity, frenzy | `mob`/`ai`/`combat`; basic mobs only | Per-ability mob tags the combat pipeline switches on (content-driven) | M–L, open-ended | no |
| **S12** | **Combat-model fidelity (the d20 option)** | initiative, action economy, to-hit roll, AC, AoO | `combat` tick/chance model | **SHELVED** — Decision 0 resolved to posture A. Reopen only if the tick/chance model is ever abandoned | XL (rewrite) | no |

---

## 3. What to explicitly NOT build (tabletop-only)

Recording these so they don't sneak back in as "missing features":

- Initiative order, turn rounds, the action economy, 5-foot steps, attacks of opportunity, ready actions, grid flanking/positioning geometry (unless Decision 0 = B).
- Take-10 / Take-20, the save-vs-check distinction, GM challenge-codes, encounter-XP-÷-party-size math, adventure-length tiers.
- Literal die surfacing where the engine resolves by probability.
- GM-narrative "Lost Abilities" whose outcome is pure interpretation (Foretelling, Old Blood, Viewing) — flavor hooks at most, not systems.
- Per-square measurement, exact weights-in-stone, in-world distance units as mechanics (they're flavor; the engine has rooms, not feet).

---

## 4. Dependencies & a proposed sequence

A few primitives unlock many sub-epics; do them early:

- **S6 (saves)** is a small cross-cutting primitive that **S2 (weaves), S5 (conditions), S7 (poison/disease/fear)** all want. Cheap and foundational — a good early slice.
- **S5 (conditions)** feeds **S1 (combat depth), S2 (weaves apply conditions), S11 (mob fear/gaze)**. Mid-foundational.
- **S4 (feats/traits)** is the substrate **S9 (backgrounds/classes grant feats)** hangs off, and many **S1** weapon perks are feats.
- **S2 (The One Power)** depends on a **resource-pool substrate** (the backlog's unbuilt "Mana pool") and benefits from **S6 + S5**. It is the single most WoT-defining system and the largest — treat it as its own multi-slice arc.

**Recommended ordering (posture A):**

1. **S1 weapon-identity (A+B+C)** — small, self-contained, makes existing classes/weapons matter today. Already proposed. *Warm-up + immediate WoT texture.*
2. **S6 saves** — tiny cross-cutting primitive everything else reuses.
3. **S5 conditions** — extend the effects system; unlocks combat/weave/mob depth.
4. **S3 skills** *or* **S4 feats** — pick the one that the content track needs first (backgrounds want both; skills are the broader base).
5. **S2 The One Power** — the marquee system; its own arc once the resource pool + S5/S6 exist. This is arguably *the reason to do WoT at all* — sequence it deliberately, not last by accident.
6. Then **S7 / S8 / S11** opportunistically (survival, reputation, Shadowspawn) as content demands.
7. **S9 / S10** when the world is big enough to need multiclass depth / planar travel.
8. **S12** only if Decision 0 flips to B.

**Engine-debt discipline:** interleave a small debt/warm-up slice between the big arcs (the BACKLOG's standing rule), and keep the content track (`wot-world-plan.md`) moving in parallel at whatever fidelity the shipped systems allow.

---

## 5. How content proceeds while systems are unbuilt

The WoT *content* track does **not** block on this program. Author weapons, armor, NPCs, and areas at **today's fidelity** (dice + modifiers, melee, single AC), and they upgrade for free as each sub-epic lands (e.g. weapons gain a proficiency tier when S1 ships; channelers become real when S2 ships). The one thing to avoid is encoding a *fake* mechanic in content (a "ranged" longbow that fights in-room) — flag those as flavor in the content until the real system exists. See `wot-world-plan.md` M4: ship weapons Tier 0 now, retrofit when S1 lands.

---

## 6. Open decisions (resolve as each sub-epic starts)

- **Decision 0 (governs everything):** ✅ **RESOLVED — posture A** (translate onto tick/chance; no d20 rewrite). S12 is therefore shelved unless explicitly reopened.
- **S2 resource model:** a single "Power" pool (mana-like) vs the d20 daily-slot budget; cooldowns vs slots; how overchannel risk surfaces (a real Fort-save consequence via S6, or a flat % mishap).
- **Character model (S3/S4/S9 + S2 eligibility) — drafted:** [`docs/proposals/wot-character-model.md`](../proposals/wot-character-model.md) resolves the keystone pre-decisions (multi-track-as-multiclass; class features as `Path` grants; feat-selection deferred; skills = proficiencies; creation-time race/gender gating). Key code finding: the engine is **already multi-track**, so multiclass is ~80% content + one small `classID string → []string` + save-v18 change. **D1 (multi-track-as-multiclass) + D2 (feat-selection deferred; class features as `Path` grants) CONFIRMED 2026-06-10.**
- **S3 skills:** use-based gain (engine-idiomatic, like proficiency) vs d20 point-buy ranks; which subset of the ~40 skills the MUD actually resolves vs drops as flavor.
- **S6 saves:** real resolved checks (some d20 texture) vs folded into existing chance math.
- **S9:** adopt d20 multiclass/BAB/HD, or keep the existing track/level model and map classes onto it. (Tied to Decision 0.)
- **Gender-locked channeling, madness for men, Ogier Longing, the taint** — setting-faithful but design-sensitive (asymmetric player experience); decide how literally to enforce.

---

## 7. Relationship to the rest of the backlog

Several sub-epics overlap items already on `BACKLOG.md` — fold them in rather than duplicating:

- **S1** = `proposals/combat-equipment-depth.md` (already a §2 entry).
- **S2** consumes the unbuilt **Mana/Movement pool** substrate (§2).
- **S7** subsumes the **hunger/thirst split** and **container caps** (§1/§2).
- **S8** is a sibling of the specced **faction** (§1) — share the signed-axis architecture.
- **S10** consumes **fast-travel waypoints** (§2) and the shipped **portal** system (M15.2).
- **Visibility, hidden-exits, biomes, gathering** (specced §1) are WoT-relevant but not WoT-specific — they belong to the general backlog, not this EPIC.

---

*This is an alignment document, not a spec. Each sub-epic's first deliverable is its own `docs/specs/` slice (or the proposal it already has). Resolve Decision 0 before committing to the program's shape; resolve each sub-epic's open decisions before writing its spec.*
