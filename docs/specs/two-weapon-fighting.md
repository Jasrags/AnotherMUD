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

**Slices.** Slice 1 (SHIPPED) is the off-hand-attack substrate — the open,
un-feated baseline (anyone may dual-wield, at the full penalty) and the seam the
feats plug into. Slice 2 (SHIPPED) is the penalty-reducing feats — Two-Weapon
Fighting and Ambidexterity. Slice 3 (SHIPPED) is **Improved Two-Weapon
Fighting** — a *second* off-hand attack (§3.1), gated behind the other two feats,
made at a further accuracy penalty (§4.3). Slice 4 (SCOPED, build pending) is
**mob dual-wield** — a mob striking with two equipped weapons (§2.3), un-feated.

**Non-goals.** No new action economy, initiative, or attacks-of-opportunity
(Decision 0). Shield mechanics (an off-hand *shield's* AC contribution) are an
armor concern (`armor-depth`), unaffected here. The engine does **not** apply an
iterative accuracy penalty to *main-hand* extra attacks (`combat §4.2` — the
`extra_attack` passive swings all fire at the same hit modifier); slice 3's
secondary off-hand penalty (§4.3) is the first iterative-attack penalty modeled,
and reworking the main-hand seam to match is out of scope.

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

### 2.3 Mob off-hand weapons — slice 4

A **mob** arms its off hand the same way it arms its main hand: through its
spawn-time equipment list (`mobs-ai-spawning §3.3`), not an equip verb. When a
mob's equipment includes a **second weapon** that fits the **off-hand slot** and
resolves the **light** wield mode for the mob's own size (§2.2 — relative to the
mob, exactly as for a player), that weapon becomes the mob's off-hand weapon and
the mob makes the off-hand attack (§3). This is **content-driven**: a mob opts in
by carrying two qualifying weapons — no template flag and no new spawn field. The
main weapon is the mob's first equipped weapon as today; the off-hand weapon is
the one that lands in the off-hand slot. A two-handed main weapon ties up the off
hand (F's footprint rule), so it precludes a mob off-hand weapon with no extra
check, and a **ranged** main suppresses the mob off-hand strike (§3) exactly as
for a player.

A mob holds **no feats** (the feat system is player-only), so a dual-wielding mob
always fights at the full un-feated two-weapon penalty (§4.1) with the reduced
off-hand Strength (§4.2) and makes exactly **one** off-hand strike — the
penalty-reducing feats (§4.1) and the second strike (§3.1) never apply to a mob.
The combat round loop is unchanged: it already resolves any attacker's off-hand
profile, so lighting up mob dual-wield is entirely a matter of the mob producing
an off-hand profile from its `Stats()`, the mirror of the player's producer.

**Acceptance criteria**

- [ ] A mob whose spawn equipment includes a second weapon that fits the off-hand
      slot and is light for the mob makes the off-hand attack, at the full
      un-feated two-weapon penalty and reduced off-hand Strength.
- [ ] A mob's off-hand weapon is the weapon that occupies the off-hand slot; its
      first weapon remains the main weapon. A non-light or non-fitting second
      weapon is carried (loot) but grants no off-hand attack.
- [ ] A mob never benefits from the two-weapon feats and never makes a second
      off-hand strike (mobs hold no feats).

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

### 3.1 A second off-hand attack (Improved Two-Weapon Fighting) — slice 3

A combatant holding the **Improved Two-Weapon Fighting** feat makes a **second**
off-hand attack in the round, for **two** off-hand strikes total. The number of
off-hand attacks a valid off-hand weapon grants is the configured **off-hand
attacks granted** count (§6) — **one** by default, raised by **one** per
Improved Two-Weapon Fighting (a single-take feat in v1, so the cap is two). The
feat is gated behind both penalty-reducing feats: it requires **Two-Weapon
Fighting** and **Ambidexterity** (the same prerequisite chain the source uses;
the source's BAB floor maps to a character-level prerequisite, deferred for the
v1 demo so the feat is reachable, mirroring the slice-2 demo feats).

The additional off-hand strike(s) use the **same off-hand weapon profile** (dice,
crit, type) as the first, resolved by the same per-swing machinery, but at a
**reduced accuracy** (§4.3). Each off-hand strike is independent: a strike that
**kills** the target ends the round — no further off-hand strikes are made
(exactly as a killing main swing stops the remaining main swings, `combat §4.3`).

**Acceptance criteria**

- [ ] A combatant wielding a valid off-hand weapon makes the **off-hand-attacks-
      granted** number of off-hand strikes per round (one by default), additive
      with any bonus main-hand swing from the existing extra-attack step.
- [ ] Improved Two-Weapon Fighting raises the off-hand-attacks-granted count by
      one (to two), and requires the Two-Weapon Fighting and Ambidexterity feats.
- [ ] Every off-hand strike uses the off-hand weapon's dice, crit rule, and
      damage type(s), resolved by the same swing machinery as the main attack.
- [ ] A ranged main attack suppresses **all** off-hand melee strikes.
- [ ] An off-hand strike that kills the target ends the round; no further
      off-hand strikes are made.

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

### 4.3 Secondary off-hand penalty (the second strike) — slice 3

When Improved Two-Weapon Fighting grants more than one off-hand strike (§3.1),
each off-hand strike **after the first** takes an additional **secondary off-hand
penalty** to hit, on top of the off-hand two-weapon penalty (§4.1) it already
carries. The penalty is **cumulative** by strike order — the second strike takes
it once, a (future) third strike twice — so accuracy falls off with each extra
off-hand swing, the source's "second off-hand attack at −5" rendered as a
configurable per-extra-strike step (§6). It rides the same attack-adjustment seam
as every other to-hit modifier. The **first** off-hand strike is unaffected (it
keeps exactly the §4.1 off-hand penalty), and the **damage** of every off-hand
strike is unchanged — only later strikes' *accuracy* falls.

This is the **first** iterative-attack accuracy penalty the engine models; the
main-hand extra-attack seam (`combat §4.2`) is intentionally left flat (non-goal,
§1).

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
- [ ] With more than one off-hand strike, each strike after the first takes the
      cumulative secondary off-hand penalty to hit; the first strike does not, and
      no off-hand strike's damage changes.

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
- **Mobs** (`mobs-ai-spawning`): slice 4 gives the off-hand slot a mob combat
  consumer. A mob that spawns with a second light off-hand-eligible weapon makes
  the off-hand attack (§2.3) — un-feated, one strike. The spawn equip path gains
  off-hand-weapon detection; the round loop and the player path are unchanged.

## 6. Configuration surface

| Setting | Meaning | Default |
|---|---|---|
| Main-hand two-weapon penalty | The to-hit penalty on the main attack while dual-wielding (§4.1). | the WoT value |
| Off-hand two-weapon penalty | The to-hit penalty on the off-hand attack while dual-wielding (§4.1); larger than the main-hand penalty. | the WoT value |
| Off-hand Strength factor | The multiplier on the Strength damage contribution for the off-hand attack (§4.2). | the WoT value (½×) |
| Off-hand rounding policy | How the scaled off-hand Strength contribution is rounded (§4.2). | pack policy (e.g. round down) |
| Off-hand attacks granted | How many off-hand attacks a valid off-hand weapon grants (§3, §3.1). | one; Improved Two-Weapon Fighting raises it by one (to two) |
| Secondary off-hand penalty | The cumulative to-hit penalty on each off-hand strike after the first (§4.3). | the WoT value (5) |

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
- **Improved Two-Weapon Fighting is a second off-hand strike at a penalty
  (slice 3, decided).** The "off-hand attacks granted" count (§6) is the dial
  the feat turns (+1, to two); the **second** strike takes a cumulative secondary
  off-hand penalty (§4.3, default the source's −5) rather than firing at full
  off-hand accuracy. Chosen over a flat (no-penalty) second swing to honor the
  source ("second off-hand attack at −5") and keep the feat upside-with-tradeoff;
  this is the first iterative-attack penalty the engine models (the main-hand
  extra-attack seam stays flat — non-goal, §1). The off-hand profile carries the
  attack **count**; the round loop loops the off-hand strike and subtracts the
  cumulative penalty per extra strike. The feat is gated behind Two-Weapon
  Fighting + Ambidexterity; the source's BAB floor maps to a level prerequisite,
  omitted for the v1 demo (reachability, mirroring the slice-2 demo feats).

- **Mob dual-wield is content-driven, un-feated, one strike (slice 4, decided).**
  A mob opts into the off-hand attack by spawning with a second weapon that fits
  the off-hand slot and is light for its size (§2.3) — no template flag, mirroring
  the player's "carry two qualifying weapons" path. The spawn equip path
  (`EquipMobAtSpawn`) gains off-hand-weapon detection (which equipped weapon
  occupies the off-hand slot); `MobInstance.Stats()` builds the off-hand profile
  the same way `connActor.Stats()` does, minus the feat-cache reads (mobs hold no
  feats → full penalty, ½× Strength, one strike). The round loop is untouched (it
  already swings any attacker's off-hand profile). Chosen over a `dual_wield:`
  template flag to keep the mechanic uniform with the player and let any mob with
  the right gear dual-wield.

**Still open (non-blocking):**

- **Per-swing penalty visibility.** Whether the player should see the two-weapon
  penalty surfaced (a cue like the non-proficiency "clumsy" message) or infer it
  from results is a UX call left for the build.
- **Off-hand weapon proficiency / non-proficiency.** The non-proficient
  check-penalty (`weapon-identity §3`) already rides the to-hit seam; whether the
  off-hand weapon's proficiency is checked independently of the main weapon is a
  build detail (the seam supports either).

---

<!-- Spec style: narrative + acceptance criteria · Detail level: behavior only · Status: slice 1 (off-hand attack substrate) SHIPPED 2026-06-17; slice 2 (Two-Weapon Fighting + Ambidexterity feats — §4.1) SHIPPED 2026-06-17; slice 3 (Improved Two-Weapon Fighting — a second off-hand strike at a cumulative secondary penalty, §3.1/§4.3) SHIPPED 2026-06-17; slice 4 (mob dual-wield — content-driven, un-feated, one strike, §2.3) SCOPED 2026-06-17, build pending. EPIC S1 increment K. -->
