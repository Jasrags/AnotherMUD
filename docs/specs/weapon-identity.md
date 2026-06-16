# Weapon Identity (categories · proficiency · critical threat)

EPIC sub-epic **S1** — the first slice of the WoT Combat & Equipment Depth
program (`docs/themes/wot-mechanics-epic.md`, `docs/proposals/combat-equipment-depth.md`,
increments **A + B + C**). Governed by EPIC **Decision 0** (translate WoT onto the
existing tick model; no d20 rewrite) and the character-model **D2**
(`docs/proposals/wot-character-model.md` — class features, incl. weapon
proficiency, are class-granted, not use-gained).

## 1. Overview

Today a weapon is a damage expression plus stat modifiers
(`inventory-equipment-items`); every weapon of the same dice is mechanically
identical, anyone may wield anything penalty-free, and a critical hit is only
the natural-maximum roll with one global multiplier (`combat §4.4`–`§4.5`).
This spec gives weapons **identity** along three axes that the combat resolver
already has the shape to honor:

- **A — category & tier.** Every weapon declares a **category** (its kind) and a
  **proficiency tier**, plus a **damage type**.
- **B — proficiency gating.** A character is **proficient** with a set of weapon
  tiers/categories (granted by class). Wielding a weapon outside that set
  imposes a **to-hit penalty**; proficient use is unpenalized.
- **C — critical threat & multiplier.** Each weapon declares a **threat range**
  (which rolls threaten a critical) and a **critical multiplier** (how the
  damage dice scale on a critical), replacing the single global policy.

**Goals.** Make weapon choice and class proficiency mechanically meaningful;
honor the WoT weapon table's tiers, threat ranges, and crit multipliers; reuse
the existing `d20 + hit-mod vs AC` resolver and its to-hit-adjust hook rather
than adding new combat machinery.

**Non-goals (this slice).** Ranged combat, ammunition, range increments (a
separate `ranged-combat` slice). Armor depth / per-damage-type AC — the
**damage type recorded here is inert** until that slice lands; a single AC still
applies (`combat §4.4`). Size-relative wielding (now its own spec,
`size-and-wielding.md`), two-weapon penalties,
masterwork grades, special-weapon handlers (later increments). Player-chosen
feats (deferred per character-model D2). No confirmation-roll step, no
initiative/action-economy (Decision 0).

## 2. Weapon categories, tiers, and damage types (A)

Every weapon declares three classifying attributes; non-weapon items declare
none and are unaffected.

- **Category** — the weapon's kind (e.g. a short-blade kind, an axe kind, a
  polearm kind). Content-defined; the engine treats it as an opaque label used
  for proficiency matching (§3) and display. Unrecognized/absent category =
  the weapon participates in no category-specific proficiency grant (it is then
  gated by tier alone).
- **Proficiency tier** — one of an ordered, content-defined tier vocabulary
  (the WoT pack uses *simple / martial / exotic*). A weapon with no declared
  tier is treated as the **lowest** tier (broadly usable) so that legacy
  weapons and minimal content keep working unpenalized.
- **Damage type** — one of a small fixed set (bludgeoning / piercing / slashing;
  a weapon may declare more than one). **Recorded only** this slice — it carries
  on the hit event for flavor/logging but does not yet change damage or AC.

A weapon's existing damage expression and stat modifiers
(`inventory-equipment-items`) are unchanged; categories/tier/type are additive.

**Acceptance criteria**

- [ ] A weapon may declare a category, a proficiency tier, and one or more
      damage types; all three are optional with the defaults above (no tier ⇒
      lowest tier; no category ⇒ tier-only gating; no type ⇒ untyped).
- [ ] Existing weapons with none of the three load and behave exactly as before
      (lowest tier ⇒ universally proficient ⇒ no penalty; §3).
- [ ] The damage type(s) are recorded on the weapon (template + instance) and do
      not alter damage, AC, or to-hit this slice. *Threading them onto the hit
      event is deferred to the armor-depth slice (E) that consumes them — an inert
      wire has no consumer until then (decided 2026-06-10).*
- [ ] Content with an unknown tier name fails validation at load (the tier
      vocabulary is pack-declared; an unlisted tier is an authoring error).

## 3. Proficiency and the non-proficient penalty (B)

A character holds a **weapon-proficiency set**: a collection of tiers and/or
specific categories they may use without penalty. The set is **granted by
class** (character-model D2 — like other class features), composed across all of
a character's classes (multiclass, character-model D1): a character is
proficient via *any* of their classes. It is **not** use-gained and does **not**
ride the ability-keyed proficiency system (`progression`); it is a static
capability set, not a 1–100 skill.

A character is **proficient with a wielded weapon** when **either** the weapon's
tier is in their granted tier set **or** the weapon's category is in their
granted category set. (A class that grants "all of tier X" covers every weapon
of that tier; a class that grants "a few specific categories" covers those
named kinds even at a higher tier — matching the WoT class proficiency lists.)

When a character attacks with a weapon they are **not** proficient with, their
to-hit is reduced by a configurable **non-proficient penalty** (§6) for the
duration of that wielding. Proficient use applies no modifier. The penalty is a
**to-hit modifier only** — it does not change damage, speed, or the ability to
equip; an out-of-tier weapon may still be wielded, just clumsily.

Because the penalty is otherwise invisible (a hit-modifier delta, no per-swing
message), **equipping a non-proficient weapon emits a one-time cue** to the
wielder so the disadvantage reads in-game. This is feedback only — it changes no
mechanic and fires only for the player who equips.

This composes with the existing per-attacker to-hit adjustment seam (the same
seam the darkness penalty uses, `combat §5.3`): the non-proficient penalty is
one more contributor to the attacker's effective hit modifier, summed before the
roll. Mob attackers, who have no class, are proficient with whatever they wield
(no penalty) unless content declares otherwise.

**Acceptance criteria**

- [ ] A character proficient with a weapon's tier OR its category attacks at no
      penalty.
- [ ] A character proficient with neither attacks at the configured
      non-proficient to-hit penalty, applied every swing while that weapon is
      wielded.
- [ ] The penalty composes additively with other to-hit adjustments (e.g.
      darkness) — both are summed into the effective hit modifier before the
      roll (`combat §4.4`).
- [ ] Proficiency is granted by class and composed across a multiclass
      character's classes (proficient if any class grants it).
- [ ] An unarmed strike and a lowest-tier weapon are never penalized (every
      character is proficient with the lowest tier).
- [ ] A mob with no class wields any weapon without penalty unless content
      states otherwise.
- [ ] The penalty changes only the to-hit roll — damage, equip eligibility, and
      attack cadence are unaffected.
- [ ] Equipping a non-proficient weapon writes a one-time cue to the wielder; a
      proficient weapon (or a non-weapon) writes none.

## 4. Critical threat range and multiplier (C)

The resolver already rolls a d20 to hit and treats the natural maximum as an
automatic critical with one global damage multiplier (`combat §4.4`–`§4.5`).
This slice makes both **weapon-specific**:

- **Threat range** — each weapon declares the set of high rolls that **threaten**
  a critical (e.g. only the maximum; the top two; the top three). A to-hit roll
  in the threat range **is** a critical hit (an automatic hit, as the maximum
  roll already is). A weapon with no declared threat range threatens only on the
  natural maximum (today's behavior). The natural-1 fumble is unchanged and is
  never a threat.
- **Critical multiplier** — each weapon declares how its damage **dice** scale on
  a confirmed critical (e.g. ×2, ×3, ×4). On a critical, the rolled weapon dice
  are multiplied by the weapon's multiplier; the strength bonus is added
  afterward and is not multiplied (preserving the existing `combat §4.5` crit
  shape). A weapon with no declared multiplier uses the configured default.

A **threatened roll is a critical** — there is **no separate confirmation roll**
(that tabletop step is dropped per Decision 0). The `isCritical` flag continues
to flow on the hit event so renderers can dramatize it (`combat §4.5`).

To make a critical deal *normal* damage, a weapon declares a multiplier of **one**
— an *undeclared* (zero/absent) multiplier means "use the configured default",
not "no bonus". The same holds for the threat range: undeclared means "natural
maximum only", so a weapon cannot widen its threat by declaring zero.

A threat range only matters on a roll that would otherwise hit or auto-hit: a
threat-range roll auto-hits and crits regardless of AC (as the maximum roll does
today); this is intentional and matches the existing natural-maximum rule.

**Acceptance criteria**

- [ ] A weapon with a wider threat range crits on more rolls; one with no
      declared range crits only on the natural maximum (unchanged default).
- [ ] A roll in the threat range is an automatic hit and a critical regardless
      of the defender's AC (mirroring the existing natural-maximum rule).
- [ ] On a critical, the weapon's own multiplier scales the dice; the strength
      bonus is added after and is not multiplied.
- [ ] A weapon with no declared multiplier crits at the configured default
      multiplier; a multiplier of one means "critical deals normal damage" (the
      `isCritical` flag still flows).
- [ ] The natural-1 fumble is unaffected and is never a threat, even for a
      weapon whose threat range is widened.
- [ ] Unarmed and weapons without C attributes behave exactly as before this
      slice.

## 5. Interaction with existing systems

- **Combat resolver** (`combat §4.4`–`§4.5`): the to-hit roll, AC comparison,
  fumble, and damage-dice/strength shape are unchanged. B adds a to-hit
  contributor; C parameterizes the threat test and the dice multiplier per
  weapon. No new combat phases, no action economy.
- **Equipment** (`inventory-equipment-items`): weapons remain ordinary equippable
  items; the new attributes are additional weapon metadata. Equip eligibility is
  unchanged — proficiency gates effectiveness, not equipping.
- **Progression / classes** (`progression`, character-model D1/D2): a class
  grants weapon-proficiency tiers/categories as a class feature; multiclass
  composes the grants. This is a static set, distinct from use-gained ability
  proficiency.
- **Light & darkness** (`light-and-darkness §5.3`): the non-proficient penalty
  and the darkness penalty share the attacker to-hit-adjust seam and sum.
- **Damage types** are recorded here and **consumed by the armor-depth slice**
  (`armor-depth.md` §4, EPIC E): they key the per-type **damage-mitigation**
  (soak) step, not a per-type AC — AC stays single (`combat §4.4`).

## 6. Configuration surface

| Setting | Meaning | Default (engine) |
|---|---|---|
| Non-proficient to-hit penalty | The to-hit modifier applied when a character wields a weapon outside their proficiency set (§3). | the WoT pack value (a single negative modifier) |
| Default critical multiplier | The dice multiplier for a critical when a weapon declares none (§4). | the existing global crit multiplier default |
| Default threat range | The rolls that threaten a critical when a weapon declares none (§4). | the natural maximum only (today's behavior) |
| Tier vocabulary | The ordered set of proficiency-tier names (§2); pack-declared. | the WoT pack tiers (simple / martial / exotic) |
| Damage-type vocabulary | The fixed set of damage types a weapon may declare (§2). | bludgeoning / piercing / slashing |

All numeric magnitudes live here per spec convention; the prose names behaviors,
not values.

## 7. Decisions (resolved 2026-06-10)

- **Proficiency storage — derive from class.** The proficiency set is read from
  the character's classes at runtime; **no save field, no version bump.** A
  persisted grant set is added only later, if a non-class source (e.g. a quest
  that teaches a proficiency) ever needs it. Matches D2 (class-granted).
- **Penalty granularity — flat.** A single configurable non-proficient to-hit
  penalty for any out-of-set weapon (the WoT "−4 to attack" rule). The tier
  ladder is ordered, so graduated penalties remain a later option.
- **Match granularity — tier + category.** A weapon is unpenalized if its tier
  **or** its category is in the character's set (§3). Per-individual-weapon
  proficiency is **not** modeled this slice (the WoT class lists don't need it).
- **Damage-type surfacing — event-only.** The damage type is carried on the hit
  event/logging only; it is **not** surfaced in player-facing text (`consider`,
  score, combat messages) until the armor-depth slice makes it affect an outcome.
- **Mob crit/threat — engine defaults for natural attacks.** A mob wielding a
  weapon with C attributes uses them; a natural-weapon mob uses the configured
  defaults (max-roll threat, default multiplier).

### Still open (non-blocking)

- Whether graduated-by-tier-distance penalties are ever wanted (the flat default
  is shippable; revisit only if balance asks for it).
