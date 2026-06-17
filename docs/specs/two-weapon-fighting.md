# Two-Weapon Fighting (off-hand attacks · dual wield)

EPIC sub-epic **S1** — increment **K** of the WoT Combat & Equipment Depth
program (`docs/themes/wot-mechanics-epic.md`,
`docs/proposals/combat-equipment-depth.md`). Governed by EPIC **Decision 0**
(translate WoT onto the existing tick model; no d20 rewrite) and layering
directly on the shipped **size-and-wielding** (`size-and-wielding.md`, increment
F) — it consumes F's **light** wield-mode classification (§4.3, the off-hand
eligibility hook F deliberately left for this increment) and F's off-hand-slot
footprint rule. *Spec ahead of code — build pending; slice 1 is the substrate,
the feats are slice 2.*

## 1. Overview

In the source material a skilled fighter can take a weapon in each hand and
strike with both — a knife-and-sword Two Rivers brawler, a Warder with paired
blades. The engine today gives every combatant a **single** wielded weapon
(`combat §4.5`): the round loop reads one weapon profile and every swing uses
it. The off-hand slot exists only as an **equipment footprint** concept
(`inventory-equipment-items §3.3`, `size-and-wielding §4.1`) — a two-handed
weapon ties it up, a shield can occupy it — but **no combat code reads it**.

This slice makes the off hand **fight**:

- A combatant may wield a **second weapon** in the off-hand slot.
- Doing so grants an **extra attack** in the round, made with the **off-hand
  weapon** (its own dice, crit, and damage type — not the main weapon's).
- Fighting with two weapons imposes a **to-hit penalty on both hands**, and the
  off-hand strike adds only a **reduced share of Strength** to its damage.
- **Feats** (a later slice) reduce or remove those penalties.

**Goals.** Honor the WoT/d20 two-weapon rules in spirit (`docs/wot/feats.md`
*Two-Weapon Fighting / Ambidexterity / Improved Two-Weapon Fighting*,
`docs/wot/equipment.md` *off-hand friendly = light*); reuse the existing
swing-count step (`combat §4.2`), the existing `HitMod` adjustment seam, the
existing Strength-to-damage step, and F's light classification rather than
adding new combat machinery; keep single-weapon and weapon-plus-shield combat
behaving exactly as today.

**Non-goals (this slice).** The two-weapon **feats** themselves — Two-Weapon
Fighting, Ambidexterity, Improved Two-Weapon Fighting — are slice 2; this slice
ships the open, un-feated baseline (anyone may dual-wield, at the full penalty)
and the seam the feats plug into. **Mob** dual-wielding (a mob striking with two
equipped weapons) is deferred — mobs equip a single weapon at spawn today; the
off-hand profile is player-only in v1. A **second** off-hand attack (Improved
Two-Weapon Fighting) is deferred with its feat. No new action economy, initiative,
or attacks-of-opportunity (Decision 0). Shield mechanics (an off-hand *shield's*
AC contribution) are an armor concern (`armor-depth`), unaffected here.

## 2. The off-hand weapon

### 2.1 Equipping a second weapon

A combatant arms the off hand by equipping a weapon to the **off-hand slot**
(`inventory-equipment-items §3.3`) — the same slot a shield uses. A weapon is
off-hand-equippable when its content declares the off-hand slot among its
**eligible slots**; the existing equip path already routes a named target slot
and already refuses an ineligible item, so no new equip verb is introduced.

- The **main hand** holds the weapon in the primary wield slot, as today.
- The **off hand** holds the second weapon. The off hand and a shield are
  mutually exclusive (one slot), and a **two-handed** main weapon already ties
  up the off hand (`size-and-wielding §4.1`), so it cannot be combined with an
  off-hand weapon — the footprint rule enforces this with no extra check.

### 2.2 Off-hand eligibility (light)

A weapon fights effectively in the off hand only when it resolves to the
**light** wield mode for its wielder (`size-and-wielding §3` — a weapon smaller
than the wielder). This is exactly the off-hand eligibility F recorded but did
not yet consume (`size-and-wielding §4.3`). A weapon that is one-handed or
larger **for this wielder** is not a valid off-hand weapon: it may still occupy
the slot for carrying/footprint purposes, but it does **not** grant the off-hand
attack (§3).

Because the wield mode is size-**relative**, off-hand eligibility is relative
too: a weapon that is one-handed for a human may be light (off-hand eligible) for
a larger wielder, and vice-versa — the same symmetry F established.

**Acceptance criteria**

- [ ] A weapon may be equipped to the off-hand slot when its content marks the
      off-hand slot eligible; the existing equip path handles routing and
      refusal with no new verb.
- [ ] An off-hand weapon and a shield cannot occupy the off hand at once (one
      slot), and a two-handed main weapon precludes an off-hand weapon (F's
      footprint rule, unchanged).
- [ ] An off-hand weapon grants the off-hand attack only when it resolves to the
      **light** wield mode for its wielder; a non-light off-hand weapon grants no
      extra attack.

## 3. The off-hand attack

When a combatant wields a valid off-hand weapon (§2.2), it makes **one extra
attack** in the round — the off-hand strike — in addition to its normal swing(s)
(`combat §4.2`). The off-hand attack:

- uses the **off-hand weapon's** dice, critical threat/multiplier, and damage
  type(s) (`weapon-identity §2, §4`), not the main weapon's;
- is resolved by the same per-swing machinery (`combat §4.3-§4.5`) — the same
  hit roll, AC comparison, crit, and soak — so nothing about how a swing lands
  changes, only *which* weapon profile feeds it and *what* penalties apply (§4).

The off-hand attack composes additively with the existing extra-attack step: a
combatant that already earns a bonus main-hand swing (`combat §4.2`, the
`extra_attack` passive seam) and also dual-wields makes its main swing(s) **and**
the off-hand strike — the two are independent.

A combatant making a **ranged** attack does not make an off-hand melee strike;
two-weapon fighting is a melee concern, and the off-hand attack is suppressed
while the main weapon is resolved as a ranged attack (`ranged-combat §2`).

**Acceptance criteria**

- [ ] A combatant wielding a valid off-hand weapon makes exactly one extra
      attack per round (the off-hand strike), additive with any bonus main-hand
      swing from the existing extra-attack step.
- [ ] The off-hand attack uses the off-hand weapon's dice, crit rule, and damage
      type(s), resolved by the same swing machinery as the main attack.
- [ ] A ranged main attack suppresses the off-hand melee strike.

## 4. Consequences of fighting with two weapons

### 4.1 Two-weapon to-hit penalty

While a combatant fights with two weapons (a valid off-hand weapon is wielded), a
**to-hit penalty** applies — a smaller penalty to the **main-hand** attack(s) and
a larger penalty to the **off-hand** attack, reflecting the difficulty of
coordinating both hands. The penalties are applied through the existing attack
adjustment seam (`combat` — the same path that stacks the darkness, armor, and
non-proficiency penalties), so they compose with every other to-hit modifier
already in play.

The penalty applies **only while dual-wielding**: a combatant with a single
weapon, or a weapon and a shield, takes no two-weapon penalty (its main attack is
exactly as today). The magnitudes live in the configuration surface (§5).

*Feats (slice 2) reduce these penalties:* the Two-Weapon Fighting feat reduces
both, and the Ambidexterity feat removes the additional off-hand-specific
penalty. This slice ships the un-feated baseline; the feat reductions plug into
the same adjustment so they are purely subtractive when they arrive.

### 4.2 Off-hand Strength on damage

The **off-hand** attack adds only a **reduced share** of the wielder's Strength
contribution to its damage — the configured **off-hand Strength factor** (the
source uses one-half), rounded per the configured policy. The **main-hand**
attack uses the full (1×) Strength contribution as today (`combat §4.5`). This
is a parameterization of the existing Strength-to-damage step — the exact
parallel of the two-handed 1.5× factor F already added (`size-and-wielding §4.2`)
— not a new damage path: only the Strength term is scaled; the base weapon dice
and a critical's dice multiplier are unchanged.

A weapon held in **both** hands cannot also be an off-hand weapon (§2.1), so the
two-handed (1.5×) and off-hand (½×) Strength factors never apply to the same
attack.

**Acceptance criteria**

- [ ] A two-weapon to-hit penalty applies to both the main and off-hand attacks
      while a valid off-hand weapon is wielded, with a larger penalty on the off
      hand; it composes with the existing to-hit modifiers.
- [ ] A combatant with a single weapon, or a weapon and a shield, takes no
      two-weapon penalty.
- [ ] The off-hand attack adds only the configured reduced share of Strength to
      damage; the main attack uses full Strength.
- [ ] The reduced-Strength factor scales only the Strength contribution, not the
      base dice or the critical dice multiplier.

## 5. Interaction with existing systems

- **Size-and-wielding** (`size-and-wielding §3, §4.3`): off-hand eligibility
  **is** the light wield mode F classified; this increment is its first consumer.
  The off-hand 1× → ½× Strength factor mirrors F's 1× → 1.5× two-handed factor.
- **Combat** (`combat §4.2, §4.5`): the off-hand strike is one more entry in the
  existing per-round swing sequence; the penalties ride the existing `HitMod`
  adjustment seam; the off-hand Strength factor parameterizes the existing
  damage step. The hit roll, AC comparison, crit dice, and soak are unchanged.
- **Equipment** (`inventory-equipment-items §3.3`): the off-hand slot gains a
  combat consumer; the slot/footprint rules (one occupant, two-handed ties it up)
  are unchanged. A weapon opts into the off hand via its eligible slots.
- **Feats** (`feats.md`): Two-Weapon Fighting / Ambidexterity / Improved
  Two-Weapon Fighting ride the existing feat system and the per-attack adjustment
  seam (the same shape as the Weapon Focus to-hit bonus); they are slice 2.
- **Weapon identity** (`weapon-identity §2, §4`): the off-hand weapon carries its
  own category / damage type / crit, read independently of the main weapon.
- **Mobs** (`mobs-ai-spawning`): unaffected this slice — a mob wields its single
  spawn weapon and takes no off-hand attack. Mob dual-wield is a later slice.

## 6. Configuration surface

| Setting | Meaning | Default |
|---|---|---|
| Main-hand two-weapon penalty | The to-hit penalty on the main attack while dual-wielding (§4.1). | the WoT value |
| Off-hand two-weapon penalty | The to-hit penalty on the off-hand attack while dual-wielding (§4.1); larger than the main-hand penalty. | the WoT value |
| Off-hand Strength factor | The multiplier on the Strength damage contribution for the off-hand attack (§4.2). | the WoT value (½×) |
| Off-hand rounding policy | How the scaled off-hand Strength contribution is rounded (§4.2). | pack policy (e.g. round down) |
| Off-hand attacks granted | How many extra off-hand attacks a valid off-hand weapon grants (§3). | one (Improved Two-Weapon Fighting raises this — slice 2+) |

All numeric magnitudes live here; the prose names behaviors, not values.

## 7. Decisions and open questions

**Decided for this slice:**

- **Open, not feat-gated.** Anyone may wield two weapons and earn the off-hand
  attack, at the full two-weapon penalty; the feats *improve* it rather than
  *unlock* it. This matches the source (the penalty exists precisely so the
  un-feated case is playable but poor) and keeps the feats as upside, not a
  paywall on the mechanic.
- **The off hand needs a light weapon.** Off-hand eligibility is F's light wield
  mode (`size-and-wielding §4.3`), resolved relative to the wielder — not a new
  flag. A non-light off-hand weapon occupies the slot but grants no attack.
- **The off-hand attack is a per-round extra swing carrying its own weapon
  profile.** Rather than overloading the weapon-agnostic `extra_attack` passive
  count, the round carries an optional off-hand weapon profile and attributes the
  off-hand swing to it. This keeps the off-hand weapon's dice/crit/type/penalty
  distinct and leaves the passive extra-attack seam untouched.
- **Substrate before feats.** Slice 1 ships the mechanic (the off-hand attack +
  baseline penalties); slice 2 adds the feats as subtractive adjustments.

**Still open (non-blocking):**

- **Mob dual-wield.** Giving a mob two equipped weapons (and the off-hand attack)
  is deferred — the spawn equip path is single-weapon today. The combat seam is
  built shared, so a later slice can light it up without reshaping the round.
- **Improved Two-Weapon Fighting (a second off-hand attack).** Deferred with its
  feat; the "off-hand attacks granted" config knob (§6) is the dial it turns.
- **Per-swing penalty visibility.** Whether the player should see the two-weapon
  penalty surfaced (a cue like the non-proficiency "clumsy" message) or infer it
  from results is a UX call left for the build.
- **Off-hand weapon proficiency / non-proficiency.** The non-proficient
  check-penalty (`weapon-identity §3`) already rides the to-hit seam; whether the
  off-hand weapon's proficiency is checked independently of the main weapon is a
  build detail (the seam supports either).

---

<!-- Spec style: narrative + acceptance criteria · Detail level: behavior only · Status: slice 1 (off-hand attack substrate) SHIPPED 2026-06-17; the feats (§4.1, Two-Weapon Fighting / Ambidexterity / Improved TWF) are slice 2 — EPIC S1 increment K -->
