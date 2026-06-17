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
| **S1** | **Weapon & Equipment Depth** | proficiency tiers, crit, damage types, ranged, armor, size-wield | `item`/`combat`/`slot` | **A+B+C (`M-Weapon-Identity`) SHIPPED 2026-06-10** (category/tier/damage-type metadata, class-granted proficiency + non-proficient penalty, per-weapon crit threat/multiplier, + a demo). **F (size-wield) SPECCED 2026-06-15** (`size-and-wielding.md`, forward spec / build pending: size-relative wield mode + two-handed Str). **E (armor) + D (damage types) SPECCED 2026-06-15** (`armor-depth.md`, forward spec / build pending) — AC-model fork RESOLVED: single AC on `defense` (decompose-and-cap) + per-type resistance on the `mitigation` channel (already subtracted as a scalar; this slice makes it per-type + its first real source; the cross-ruleset soak primitive — WoT leans `defense`, Shadowrun leans `mitigation`). **H (masterwork) SHIPPED 2026-06-16** (`masterwork.md`, `internal/grade`: graded quality bonuses via existing seams, grade kept independent of the rarity decoration). **G (ranged) SHIPPED 2026-06-16** (`ranged-combat.md`) — Slice A (thrown/projectile + ammo + Strength rules + masterwork ammo) + Slice B (far→near→melee range bands, auto-close + kiting, incl. mob ranged AI) + **Model C** (cross-room targeting as an opportunistic adjacent-room action: the `shoot` verb + a shot mob's retaliation pursuit; sustained cross-room combat + multi-room LoS/pursuit deferred per §10). Remaining: build E+F, encumbrance (I), special weapons (J) | M done; L remains | spec `weapon-identity.md` + `size-and-wielding.md` + `armor-depth.md` + `masterwork.md` + `ranged-combat.md` |
| **S2** | **The One Power (channeling)** | saidin/saidar, weaves, daily slots, affinities, talents, overchannel, linking, madness | `progression` abilities + `effect`; **resource-pool substrate SHIPPED + specced** | **Proposal drafted 2026-06-11** ([`proposals/wot-the-one-power.md`](../proposals/wot-the-one-power.md)) — D1 single Power pool (not slots), D2 weaves = `spell` abilities + use-based proficiency, D3 gendered Source / defer taint, D4 one channeler class (split-seam free), D6 overchannel = Fort save (S6) → cascade (S5). Phasing: P0 Power-pool substrate (closes BACKLOG "Mana pool") → P1 channeler class + starter weaves + `channel` verb → P2 overchannel → P3+ affinities/interrupt-game/taint/angreal/linking. Research: [`research/wotmud-channeling.md`](../research/wotmud-channeling.md). **Phases 0–3 SHIPPED 2026-06-14** (save v22): pool substrate + persistence + regen + spend knobs (P0); channeler class + One Power pool + classic-four weaves + `channel` verb (P1); `overchannel` → Fort save → fatigued/stunned/stilled cascade + channel-block (P2); **affinities & the Five Powers (P3)** — `elements` field on abilities + gender at creation (v22) + gender-derived two-tier affinity (men Earth/Fire/Spirit, women Air/Water/Spirit) → soft potency scaling of weave damage/heal, weakest element governing. **P4 partially SHIPPED 2026-06-14:** affinity now also scales the EFFECT path (save-gated entry DC + recurring-save DC + buff-modifier magnitudes, via an injected `PotencyFunc` on the resolver — affinity coverage COMPLETE); and the **interrupt game is DONE** — weaves take an interruptible `cast_time` warmup (in combat rounds; `CastTracker`/`CastNotifier` + the ability-phase begin/advance/resolve state machine), and a **hit, a room change, or an incapacitating condition (stun)** all abort an in-flight weave (free refund — under spend-on-success an unresolved cast never drew the Power). **Initiate/Wilder split also DONE 2026-06-14** — the single `channeler` class replaced by `initiate` (INT-keyed pool, weak Fort) + `wilder` (WIS-keyed pool, strong Fort + bigger HP die); content-only (the divergence rides existing growth-bonus + save-progression seams, zero engine code). P4+ remaining: Talents catalogs (not ported — d20 bookkeeping), Wilder emotional Block, taint/madness, angreal, linking, stilling restore path (+ optional interrupt polish: GMCP cast-bar, cast_time tuning). **Engine substrate now SPECCED (setting-agnostic, 2026-06-15):** generalized resource pools (`progression.md` §2.6 + the player-save surface in the specs README), the cast-time/interrupt game + the reserve-gate/overexertion pattern (`abilities-and-effects.md` §4.9–§4.10), and the channel-map derived-formula seam (`combat.md` §4.4); [`wot-the-one-power.md`](../proposals/wot-the-one-power.md) is now scoped to the WoT-specific instantiation (saidin/saidar, Five Powers, taint, Initiate/Wilder). | **XL** | **P0–P3 + P4 (affinity-complete + interrupt game + Initiate/Wilder split) shipped + substrate specced; P4+ depth open** |
| **S3** | **Skills system** | ~40 d20 skills (Hide, Heal, Diplomacy, Craft, …), ranks, synergy, trained-only | `progression` **proficiency** (use-based gain) is the analog; no skill list | **SHIPPED (substrate) 2026-06-10** (`skills.md`): skill = use-based proficiency (the convention crafting already proved), the `ResolveSkillCheck` primitive (`d20 + bonus vs DC`, mirrors saves), and the first consumer — lockpicking (`pick` vs a door's pick difficulty) + Open Lock skill + a `skills` listing. Lean: hide/search/spot belong to visibility; social skills dropped | L (substrate done) | spec `skills.md` |
| **S4** | **Feats / traits (passive perk engine)** | binary feats w/ prereqs, multi-take, stackable; class bonus feats | `abilities-and-effects` has passives; no perk-selection engine | **SHIPPED 2026-06-11** (proposal [`proposals/wot-feats.md`](../proposals/wot-feats.md), 8 phases each reviewed): the full player-choice selection engine — `internal/feat` registry + prereq evaluator + banked feat-credit slots (1 at creation + 1/3 levels, save **v20**) + the `feat`/`feats` verbs + all six grant kinds (save_bonus / max_hp via `srckey.Feat` / hit_bonus / crit_threat / skill_bonus / ability) + authored background feat grants. Content: the v1 static-bonus family (saves trio / Toughness / Weapon Focus / Improved Critical / Skill Emphasis / Power Attack). Per-weapon feats bind to weapon category. Action-economy/ranged/armor/channeling/reputation/lost-ability feats deferred to their owning sub-epics; the slot engine hosts S2 channeling feats later. Open polish: creation-wizard feat-pick step, the Power Attack ability's combat effect, choose-a-feat-from-a-pool. **Engine substrate now SPECCED (setting-agnostic, 2026-06-15):** the player-choice perk engine — registry, prereq/eligibility, multi-take rules, banked credits, the grant bridge, and persistence — is `feats.md`; `wot-feats.md` retains the WoT feat catalog + content triage. | L done | spec `feats.md` (+ proposal `wot-feats.md`) |
| **S5** | **Conditions & status effects** | prone, stunned, dazed, bleeding, frightened, fatigued, blinded, entangled, helpless, … | `effect` system exists & is the natural home | **SHIPPED 2026-06-10** (`conditions.md`): the Core 5 (stunned/prone/blinded/frightened/fatigued) as flagged effects + combat hooks (incapacitation skip-swing, defender vulnerability, attacker/save penalties, forced flee), entry + per-tick shake-off saves (consumes S6), inflict via `afflict`/`cure` + save-gated `trip`/`bash` abilities, `affects` listing. HP-state/DoT/grapple families deferred | M done | spec `conditions.md` |
| **S6** | **Saves (Fort / Reflex / Will)** | three save axes vs poison/fear/area/mind | `stats` + `combat`; no save axis today | **SHIPPED 2026-06-10** (`saves.md`): three derived saves (class strong/weak base + ability mod), `ResolveSave` d20 primitive + `SaveResolved` event, first consumer = massive-damage Fortitude save, score-sheet row. Reflex/Will derived+shown, await S5/S7 consumers | S–M done | spec `saves.md` |
| **S7** | **Survival & environment v2** | encumbrance, thirst, fatigue/subdual, temperature, poison, disease, falling, suffocation | `economy` sustenance/rest (single pool); container caps specced | Extend sustenance → multi-pressure survival; encumbrance rides container caps | M (per pressure) | partial — "thirst split" + "container caps" on backlog |
| **S8** | **Reputation & social standing** | reputation score, fame/infamy, NPC attitude shift, followers | none; `faction` (specced §1) is the sibling architecture | Reputation as a parallel signed axis like faction; NPC attitude as a disposition hook | M | adjacent — `faction` specced |
| **S9** | **Class / background / progression rebuild** | 7 classes, 12 backgrounds, multiclass, BAB, per-class HD, ability-ups, languages | `progression` tracks/classes/races exist; no multiclass/BAB/background | **Multiclass seam SHIPPED 2026-06-10** (class `string → []string`, save v18 — the engine was already multi-track); **the class-set model is now SPECCED** in `progression.md` §4.7 (the keystone `wot-character-model` D1, setting-agnostic). **Backgrounds SHIPPED 2026-06-11** (`backgrounds.md`): the creation-origin starting package — skill proficiencies + items + gold, granted once at creation, save v19; core `Commoner` + 4 starter-world demo backgrounds. **Authored background feats SHIPPED 2026-06-11** (S4 Phase 5 — a background's `feats:` list grants them free at creation; the soldier grants Great Fortitude). Remaining: languages (no system), the class-skill-cap nuance, and the *choose-a-background-feat-from-a-pool* refinement; BAB/HD/d20-leveling **skipped** per Decision 0 (posture A). | L (seam + backgrounds + bg-feats done) | spec `backgrounds.md` |
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

- **S6 (saves)** is a small cross-cutting primitive that **S2 (weaves), S5 (conditions), S7 (poison/disease/fear)** all want. **SHIPPED 2026-06-10** — `combat.ResolveSave(roller, bonus, dc)` + the `SaveResolved` event are the seam those sub-epics call; `progression` derives the three saves per creature.
- **S5 (conditions)** feeds **S1 (combat depth), S2 (weaves apply conditions), S11 (mob fear/gaze)**. **SHIPPED 2026-06-10** — the Core 5 condition vocabulary + combat hooks + save-gated apply/shake-off are the seam S2/S11 reuse.
- **S4 (feats/traits)** is the substrate **S9 (backgrounds/classes grant feats)** hangs off, and many **S1** weapon perks are feats.
- **S2 (The One Power)** depends on a **resource-pool substrate** (the backlog's former "Mana pool" — now SHIPPED and specced, `progression.md` §2.6) and benefits from **S6 + S5**. It is the single most WoT-defining system and the largest — treat it as its own multi-slice arc.

**Recommended ordering (posture A):**

1. **S1 weapon-identity (A+B+C)** — ✅ **SHIPPED 2026-06-10** (spec `weapon-identity.md` + the demo). The next S1 work is the separate ranged (G) and armor (E) themes.
2. **S6 saves** — ✅ **SHIPPED 2026-06-10** (spec `saves.md`): the cross-cutting d20 save primitive (`ResolveSave`, `SaveResolved`) + three derived saves + the massive-damage Fortitude consumer. S5/S7 now have the save check to call.
3. **S5 conditions** — ✅ **SHIPPED 2026-06-10** (`conditions.md`): the Core 5 as flagged effects + combat hooks + the entry/shake-off saves (consumes S6) + both inflict paths. Unlocks combat/weave/mob depth; S2 weaves now have a condition vocabulary to apply.
4. **S3 skills** — ✅ **SHIPPED (substrate) 2026-06-10** (`skills.md`): skill = use-based proficiency + the `ResolveSkillCheck` primitive + lockpicking as the first consumer. The primitive is the seam **backgrounds (S9)** and **visibility** call; more skills are authored by their owning systems.
4b. **S4 feats** — ✅ **SHIPPED 2026-06-11** (`proposals/wot-feats.md`, 8 reviewed phases): the player-choice feat selection engine + all six grant kinds + the v1 static-bonus feat family + authored background feats (save v20). Unblocks the deferred half of S9 and hosts S2 channeling feats later.
5. **S2 The One Power** — the marquee system; its own arc once the resource pool + S5/S6 exist. This is arguably *the reason to do WoT at all* — sequence it deliberately, not last by accident.
6. Then **S7 / S8 / S11** opportunistically (survival, reputation, Shadowspawn) as content demands.
7. **S9** — multiclass seam ✅ + backgrounds ✅ shipped (2026-06-10/06-11); remaining S9 depth (background feats, languages) waits on S4 / a language system. **S10** when the world is big enough to need planar travel.
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
- **S3 skills:** ✅ **RESOLVED — use-based gain + `d20 + bonus vs DC` check** (mirrors saves); lean subset (lockpicking shipped, hide/search/spot owned by visibility, social skills dropped). Spec `skills.md`.
- **S6 saves:** real resolved checks (some d20 texture) vs folded into existing chance math.
- **S9:** ✅ **RESOLVED** (Decision 0 = posture A) — keep the existing track/level
  model and map classes onto it; the multiclass seam (class-list, save v18) +
  backgrounds (starting package, save v19, `backgrounds.md`) shipped. d20
  BAB/HD/literal leveling **skipped**. Background **feats** ✅ shipped (S4 Phase 5,
  authored fixed grant); the choose-from-a-pool refinement remains.
- **Gender-locked channeling, madness for men, Ogier Longing, the taint** — setting-faithful but design-sensitive (asymmetric player experience); decide how literally to enforce.

---

## 7. Relationship to the rest of the backlog

Several sub-epics overlap items already on `BACKLOG.md` — fold them in rather than duplicating:

- **S1** = `proposals/combat-equipment-depth.md` (already a §2 entry).
- **S2** consumes the **Mana/Movement pool** substrate — SHIPPED + specced (`progression.md` §2.6).
- **S7** subsumes the **hunger/thirst split** and **container caps** (§1/§2).
- **S8** is a sibling of the specced **faction** (§1) — share the signed-axis architecture.
- **S10** consumes **fast-travel waypoints** (§2) and the shipped **portal** system (M15.2).
- **Visibility, hidden-exits, biomes, gathering** (specced §1) are WoT-relevant but not WoT-specific — they belong to the general backlog, not this EPIC.

---

*This is an alignment document, not a spec. Each sub-epic's first deliverable is its own `docs/specs/` slice (or the proposal it already has). Resolve Decision 0 before committing to the program's shape; resolve each sub-epic's open decisions before writing its spec.*
