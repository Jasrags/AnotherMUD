# Size and Wielding (size-relative wield mode · two-handed Strength)

EPIC sub-epic **S1** — increment **F** of the WoT Combat & Equipment Depth
program (`docs/themes/wot-mechanics-epic.md`,
`docs/proposals/combat-equipment-depth.md`). Governed by EPIC **Decision 0**
(translate WoT onto the existing tick model; no d20 rewrite) and layering on the
shipped **weapon-identity** (`weapon-identity.md`, increments A+B+C). *Spec
ahead of code — build pending.* Depends only on increment **A** (weapons already
carry classifying metadata); it does not depend on ranged (G) or armor (E), and
they do not depend on it.

## 1. Overview

A weapon's combat feel in the source material depends not just on its dice but on
**how big it is relative to the wielder**: a blade your own size is a balanced
one-handed weapon, a smaller one is a light off-hand option, and a larger one
takes both hands (and rewards the grip with extra strength behind the blow). The
engine today has only a **static** two-handed footprint — a weapon declares
`offhand` as a companion slot (`inventory-equipment-items §3.3`) regardless of
who wields it — and damage scales by a flat Strength modifier (`combat §4.5`).

This slice makes wielding **size-relative**:

- Every creature has a **size**; every weapon has a **size**.
- The **wield mode** (light / one-handed / two-handed / too-large) is derived
  from the difference *(weapon size − wielder size)*, not declared outright.
- The wield mode drives two consequences: the **equip footprint** (a two-handed
  weapon ties up the off hand; a light one does not) and a **two-handed
  Strength bonus** to melee damage.

**Goals.** Honor the WoT table's size-relative wielding (`docs/wot/equipment.md`,
*Size vs. wielder*); reuse the existing companion-slot footprint and the existing
Strength-to-damage step rather than adding new equip or combat machinery; keep
legacy content (no size data) behaving exactly as today.

**Non-goals (this slice).** Two-weapon fighting and off-hand attack penalties
(a later increment — this slice only *classifies* a weapon as off-hand-eligible,
it does not add a second attack or its penalties). Ranged Strength rules (full
on thrown, none on projectile) — those belong to the ranged slice (G).
Encumbrance and carry weight (increment I). Reach, set-vs-charge, and other
special-weapon handlers (increment J). No initiative or action economy
(Decision 0).

## 2. Size as an attribute

### 2.1 Creature size

Every combatant entity has a **size**, drawn from an ordered, content-defined
**size vocabulary** (e.g. tiny … small … medium … large …). Size is a single
ordinal attribute; the engine cares only about the **ordered distance** between
two sizes, never the names.

- A character's size defaults to the configured **baseline size** (the size most
  playable races are) unless its race declares otherwise. Race-derived size is
  read the same way other race attributes are; no per-character size choice is
  introduced this slice.
- A mob's size is declared on its template; a mob that declares none takes the
  baseline size.

### 2.2 Weapon size

Every weapon declares a **size** from the same vocabulary. A weapon that declares
no size is treated as the **baseline size**, so a weapon authored before this
slice is a one-handed weapon for a baseline-size wielder (its current behavior).
Weapon size is additive metadata alongside the weapon-identity attributes
(`weapon-identity §2`); non-weapon items declare none and are unaffected.

An **unarmed strike** is treated as a weapon two size steps **smaller** than the
wielder (so it is always *light*), matching the source rule.

**Acceptance criteria**

- [ ] Every creature resolves to a size; absent data yields the configured
      baseline size (players via race, mobs via template).
- [ ] A weapon may declare a size; absent ⇒ baseline size.
- [ ] The engine compares sizes only by ordered distance, never by name.
- [ ] Existing weapons and creatures with no size data resolve to baseline and
      behave exactly as before (one-handed for a baseline wielder).

## 3. Wield mode (the size delta)

When a creature wields a weapon, the **wield mode** is derived from the signed
step distance *d = (weapon size − wielder size)* in the size vocabulary:

| d (weapon relative to wielder) | Wield mode |
|---|---|
| smaller (`d < 0`) | **light** |
| same size (`d = 0`) | **one-handed** |
| one step larger (`d = +1`) | **two-handed** |
| two or more steps larger (`d ≥ +2`) | **too large** (unwieldy) |

- **Light** — usable in either hand and **off-hand eligible** (the two-weapon
  consequence is deferred, §4.3); confers no Strength bonus beyond one-handed.
- **One-handed** — the ordinary grip; occupies only the target slot.
- **Two-handed** — requires both hands (§4.1) and grants the two-handed Strength
  bonus (§4.2).
- **Too large** — a weapon two or more steps larger than the wielder cannot be
  wielded; the equip attempt is **refused** with a clear reason (§4.1).

The wield mode is recomputed whenever the *(weapon, wielder)* pair changes; it is
a derived property, never persisted.

**Acceptance criteria**

- [ ] Wield mode is derived from the signed size distance per the table above and
      recomputed per *(weapon, wielder)* pair.
- [ ] A weapon smaller than the wielder is light; same size is one-handed; one
      step larger is two-handed; two or more steps larger is too large.
- [ ] An unarmed strike always resolves to light (two steps smaller).

## 4. Consequences of wield mode

### 4.1 Equip footprint

The wield mode determines the weapon's **footprint** (`inventory-equipment-items
§3.3`) when it is wielded:

- **Light / one-handed** — the footprint is the target slot only (the off hand
  stays free).
- **Two-handed** — the footprint additionally occupies the off-hand slot, exactly
  as the existing static two-handed companion-slot footprint does. The off hand
  must be free (or auto-cleared by the normal equip rules) to wield it.
- **Too large** — the weapon cannot be equipped into the wield slot at all; the
  equip is refused.

**Compatibility default.** Size **derives** the footprint only when both the
weapon and the wielder carry size data. A weapon that declares no size (or a
context with no wielder size) falls back to its **statically declared**
companion-slot footprint (`inventory-equipment-items §3.3`) — so existing
two-handed weapons keep their hand-tie-up with no size data, and nothing about
legacy equipping changes. *(This is the integration default chosen for this
slice; see §7.)*

### 4.2 Two-handed Strength

When a weapon is wielded **two-handed in melee**, the wielder's Strength
contribution to damage is multiplied by a configurable **two-handed Strength
factor** (the source uses 1.5×), rounded per the configured policy; the base
weapon dice are unchanged. Light and one-handed melee use the full (1×) Strength
contribution, as today (`combat §4.5`).

This is a parameterization of the existing Strength-to-damage step, not a new one:
the rolled weapon dice are scaled by the wielder's Strength modifier; this slice
multiplies *that Strength contribution* by the two-handed factor when the wield
mode is two-handed. A critical hit's dice multiplier (`weapon-identity §4`)
continues to apply to the dice only, not to the Strength bonus.

Strength rules for **thrown** and **projectile** weapons (full Strength on
thrown, none on a bowstring) are a **ranged-combat** concern (increment G) and
are out of scope here.

### 4.3 Light and the off hand (classification only)

A light weapon is **off-hand eligible** — it is the kind of weapon that *could*
be wielded in the off hand for a second attack. This slice records that
eligibility (a light wield mode) but does **not** add an off-hand attack, the
two-weapon to-hit penalties, or any second swing — that is a later increment.
Until then, "light" affects only the footprint (§4.1, off hand stays free) and
signals off-hand eligibility for the future two-weapon slice.

**Acceptance criteria**

- [ ] A two-handed wield mode occupies the off-hand slot; a light or one-handed
      mode leaves it free.
- [ ] A too-large weapon is refused at equip with a clear reason.
- [ ] A weapon or wielder lacking size data falls back to the static
      companion-slot footprint — legacy two-handed weapons are unchanged.
- [ ] Two-handed melee multiplies the Strength contribution to damage by the
      configured factor; light/one-handed melee uses full Strength.
- [ ] The two-handed factor scales only the Strength contribution, not the base
      dice or the critical dice multiplier.
- [ ] A light weapon is marked off-hand eligible but no off-hand attack or
      two-weapon penalty is introduced this slice.

## 5. Interaction with existing systems

- **Equipment** (`inventory-equipment-items §3.3`): the wield mode feeds the
  existing footprint machinery — two-handed adds the off-hand companion slot,
  too-large blocks the equip. No new slot mechanic; size only chooses whether the
  off-hand companion is part of the footprint.
- **Combat** (`combat §4.5`): the wield mode parameterizes the existing
  Strength-to-damage step (the two-handed factor). The to-hit roll, AC
  comparison, fumble, and crit dice are unchanged.
- **Weapon identity** (`weapon-identity §2`): weapon size is additional weapon
  metadata, sitting beside category / tier / damage type; it composes
  independently of proficiency and crit.
- **Mobs** (`mobs-ai-spawning`): a mob's size rides its template; a sizeless mob
  is baseline. A large mob wielding a baseline weapon resolves it as light (and
  could one-hand a weapon that is two-handed for a player), which is the intended
  relativity.
- **Races** (`progression §3`): a race may declare its size; absent ⇒ baseline.

## 6. Configuration surface

| Setting | Meaning | Default |
|---|---|---|
| Size vocabulary | The ordered set of size names (§2). | pack-declared (e.g. tiny … large) |
| Baseline size | The size used when a creature or weapon declares none (§2). | the size of most playable races (e.g. medium) |
| Two-handed Strength factor | The multiplier on the Strength damage contribution for a two-handed melee wield (§4.2). | the WoT value (1.5×) |
| Two-handed rounding policy | How the scaled Strength contribution is rounded (§4.2). | pack policy (e.g. round down) |
| Unarmed size offset | How many steps smaller than the wielder an unarmed strike counts as (§2.2). | two steps smaller |
| Max wieldable step | The largest size distance still wieldable (above it ⇒ too large) (§3). | one step larger |

All numeric magnitudes live here; the prose names behaviors, not values.

## 7. Decisions and open questions

**Decided for this slice:**

- **Size derives the footprint; static footprint is the fallback.** When both the
  weapon and the wielder carry size data, the wield mode derives the footprint
  (§4.1); otherwise the existing statically-declared companion slots apply. This
  keeps every legacy weapon byte-identical and lets size content opt in
  per-weapon, rather than forcing a global migration of every two-handed weapon's
  metadata. *(This is the integration default the proposal left open.)*
- **Wield mode is derived and ephemeral** — never persisted, recomputed per
  *(weapon, wielder)* pair, so changing either (re-equip, a size buff) re-resolves
  it automatically.
- **Relativity is symmetric.** A weapon that is two-handed for a smaller wielder
  is one-handed (or light) for a larger one — the same weapon, different wielder,
  different mode. This is the whole point of size-relative wielding and needs no
  special-casing.

**Still open (non-blocking):**

- **Two-weapon fighting.** Off-hand attacks and their to-hit penalties are a
  separate increment; this slice only classifies a weapon as off-hand eligible.
- **A size buff/debuff effect.** Nothing here introduces a temporary size change,
  but because wield mode is derived, an effect that altered a creature's size
  would re-resolve wielding for free. Whether content wants such an effect is a
  later call.
- **Encumbrance interplay.** Size, carry weight, and armor speed penalties are
  one interacting system in the source (increment I); this slice does not touch
  weight.
- **Per-step graduated rules.** The source has finer size interactions (a Small
  creature using a Medium "spear, Aiel" without difficulty, double weapons);
  those are per-weapon special-handler content (increment J), not the general
  rule specified here.

---

<!-- Spec style: narrative + acceptance criteria · Detail level: behavior only · Status: forward spec (build pending) — EPIC S1 increment F -->
