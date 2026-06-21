# Armor Depth (armor class · damage resistance · armor proficiency)

EPIC sub-epic **S1** — increment **E** (and **D**, damage types, now consumed) of
the WoT Combat & Equipment Depth program
(`docs/themes/wot-mechanics-epic.md`, `docs/proposals/combat-equipment-depth.md`).
Governed by EPIC **Decision 0** (translate onto the existing tick model; no d20
rewrite). *Spec ahead of code — build pending.* Layers on `combat §4.4`–`§4.5`,
the **channel map** (`combat §4.4`: the `defense` and `mitigation` derived
channels), `weapon-identity` (damage types, the proficiency-penalty seam),
`inventory-equipment-items` (armor slots), and the sibling `size-and-wielding`.

## 1. Overview

Armor in the source does several distinct jobs that the engine currently
collapses into one flat AC modifier. This slice separates them across the **two
defensive channels the engine already reserves**, and that separation is the
key design decision:

- **To-hit avoidance** lives on the **`defense` channel** (armor class) — heavier
  armor makes you **harder to hit**. This stays a **single** value vs all damage
  types, faithful to the WoT armor table's single "Bonus" column. Armor
  **layers** onto the existing derived AC additively, with one new primitive: a
  **max-Dex cap** the worn armor places on the Dex contribution.
- **Damage reduction** lives on the **`mitigation` channel** (soak) — you are
  hit, but incoming damage is reduced **by damage type**. The damage roll
  already subtracts a single scalar `mitigation` (baseline zero today,
  `combat §4.5`); this slice is its **first real source** and makes it
  **per-type** — and per-type resistance (slash/pierce/bludgeon *and* elemental
  types like fire or electricity) is where damage-type differentiation belongs,
  not on AC.

The deliberate split: **per-damage-type differentiation is a damage-mitigation
concept, not an AC concept.** A fire attack is not dodged by a "fire AC"; it
lands and is *resisted*. Modeling resistance as per-type AC would be a category
error. Keeping AC single and putting per-type resistance on `mitigation` serves
both settings cleanly — **WoT leans on `defense`** (armor = harder to hit),
**a future Shadowrun-style pack leans on `mitigation`** (armor = damage
resistance, plus elemental resistances) — on one kernel, with each ruleset
choosing where its armor does its work.

This slice also **consumes the damage type (D)** recorded inertly by
`weapon-identity §2`: incoming damage now carries a type, and mitigation is keyed
by it.

**Goals.** Give armor the WoT table's depth (armor bonus, max-Dex cap, armor
check penalty, armor proficiency, don/doff timers, shields) on a **single** AC;
introduce the per-type **damage-mitigation step** as the cross-ruleset soak
primitive; reuse the `defense`/`mitigation` channels and the existing equipment
slots rather than new machinery.

**Non-goals (this slice).** Encumbrance, carry weight, and the armor **speed**
penalty (increment I — cross-referenced, not built here). Shield **bash** as an
offensive weapon and other special handlers (increment J). Two-weapon penalties
(a later increment). No initiative or action economy (Decision 0).

## 2. Armor as equipment

Armor is ordinary equippable gear (`inventory-equipment-items`) carrying armor
metadata; non-armor items declare none and are unaffected. An armor item
declares:

- An **armor bonus** — its additive contribution to AC (the `defense` channel,
  §3).
- A **max-Dex cap** — the most of the wearer's Dex modifier that counts toward AC
  while this armor is worn (§3). Absent ⇒ no cap.
- An **armor check penalty** — a penalty applied to Strength- and Dexterity-based
  skill checks while worn (§5). Absent ⇒ none.
- An **armor category / tier** — for proficiency matching (§5), drawn from a
  pack-declared armor-tier vocabulary (the WoT pack uses *light / medium /
  heavy*).
- Optional **per-type resistance** contributions (§4) — a map of *damage type →
  amount of incoming damage of that type reduced*. Absent ⇒ the armor grants no
  damage resistance (a pure to-hit armor, the WoT default).
- **Don / remove durations** and a **hastily-donned** variant (§7). Absent ⇒
  instant.

**Shields** are a distinct equippable (an off-hand item) that grants its own AC
bonus and may carry its own check penalty; a shield's bonus **stacks** with body
armor (§3). Shield offensive use (bash) is out of scope (increment J).

## 3. Armor class — the `defense` channel (single, decompose-and-cap)

AC (the `defense` channel) is a **derived value composed of named terms** rather
than one opaque number, so the max-Dex cap can apply to the Dex term alone:

```
AC = base + min(DexMod, armorMaxDex) + armorBonus + shieldBonus + misc
```

- **Base** — the configured unarmored baseline.
- **Dex term, capped** — the wearer's Dexterity modifier, **capped** by the worn
  armor's max-Dex (if it declares one). With no armor (or no cap) the full Dex
  modifier applies, exactly as today. If multiple worn pieces declare caps, the
  **most restrictive (lowest)** cap applies.
- **Armor bonus** + **shield bonus** + **misc** (deflection, natural armor,
  size, spell effects) — additive contributors that **stack** across distinct
  sources. Same-slot exclusivity (you wear one body armor) is enforced by the
  equipment slot rules (`inventory-equipment-items §3`), not here.

AC is **single** — one value compared against the attack roll regardless of the
weapon's damage type (`combat §4.4`). Damage-type differentiation is handled on
`mitigation` (§4), not here.

**Unarmored and legacy behavior is unchanged.** With no armor worn, there is no
cap and AC is `base + full Dex + any existing AC item modifiers` — byte-identical
to today. Existing AC-granting items remain additive `misc` contributors.

This composes through the existing `defense` channel formula (`combat §4.4`): the
ruleset's `defense` derivation sums these terms, and the engine reads the channel
by name. The non-armored baseline `defense` formula is unchanged; armor adds
terms and the cap.

**Acceptance criteria**

- [x] AC is composed of a base, a Dex term, an armor bonus, a shield bonus, and
      misc contributors that stack across distinct sources.
- [x] The worn armor caps the Dex term at its max-Dex; with no armor or no cap
      the full Dex modifier applies; the most restrictive cap wins if several
      apply.
- [x] AC is a single value vs all damage types (no per-type AC).
- [x] A character wearing no armor has identical AC to before this slice.
- [x] A shield's bonus stacks with body armor's bonus.

**Status (2026-06-16):** BUILT. The Dex term + max-Dex cap are scoped to a
**WoT channel-map override** (`content/wot/channel-map`: `defense: ac + dex_ac`)
— the engine's `dex_ac` synthetic input (`session.cappedDexAC`) is the Dex
modifier capped by the most restrictive worn armor (`armorDexCap`, snapshotted
on equip; full Dex when unarmored). The **fantasy/core baseline keeps `defense:
ac`** (no Dex term), so the default boot's AC is byte-identical to before — the
spec's "identical AC to before" criterion holds per-ruleset (the source assumed
Dex already fed AC; this engine had no Dex-AC term, so adding it globally was
rejected). The **armor bonus** applies as an `ac` stat modifier at equip
(`internal/command/equipment.go`), stacking across body armor + shield in every
ruleset. Item metadata (`ArmorBonus`/`ArmorMaxDex`/`ArmorTier`) lifts onto
`entities.ItemInstance`. Live-verified on `make run-wot` (AC 10→11→14 across
unarmored → light cap → heavy helm).

## 4. Damage resistance — the `mitigation` channel (per type)

The damage roll already subtracts a **single scalar** `mitigation` (soak) from
damage before the min-1 floor (`combat §4.5`; baseline maps it to zero). This
slice makes that subtraction **per damage type** — the incoming damage's type
selects which resistance applies:

```
finalDamage = rolledDamage − mitigation[incomingType]
```

It extends the existing step rather than adding a new one: the scalar
`mitigation` channel becomes a per-type lookup, and the incoming damage's type
is threaded into the subtraction.

- **Incoming damage carries a type** — from the weapon (`weapon-identity §2`'s
  recorded damage type, now consumed) or from the ability/effect that dealt it
  (e.g. a fire weave declares `fire`). Untyped damage matches no resistance and
  is unmitigated.
- **Mitigation is a per-type map** keyed over the pack-declared damage-type
  vocabulary (`weapon-identity §6`) — `mitigation[slash]`, `mitigation[fire]`,
  `mitigation[electricity]`, … Each entry is the amount of incoming damage of
  that type the defender soaks. A type with no entry gets zero mitigation (full
  damage).
- **Sources compose additively** — a defender's resistance to a type is the sum
  of contributions from worn armor, active effects, racial traits, and any other
  source, read through the `mitigation` channel — already subtracted from
  damage, this slice being its first non-zero source (`combat §4.4`–§4.5).
- **The post-mitigation floor is policy.** Whether fully-resisted damage floors
  at the existing "at least 1 on a hit" rule (`combat §4.5`) or may be reduced to
  zero (full soak) is a **configurable** per-ruleset choice (§9): WoT keeps the
  min-1 floor; a Shadowrun-style pack allows full absorption.

This is the cross-ruleset soak primitive. **WoT armor** may optionally grant
physical-type resistance (plate shrugging off a slash) but primarily works via
`defense` (§3); a **Shadowrun-style pack** leans here, with armor and gear
granting physical *and* elemental resistances. Both use the same machinery.

**Acceptance criteria**

- [ ] Incoming damage carries a damage type (from the weapon or the dealing
      ability/effect); untyped damage is unmitigated.
- [ ] Final damage is reduced by the defender's per-type mitigation for the
      incoming type; an unmatched type is reduced by zero.
- [ ] Mitigation sources (armor, effects, racial) compose additively per type
      via the `mitigation` channel.
- [ ] The post-mitigation damage floor (min-1 vs full-absorb) follows the
      configured per-ruleset policy.
- [ ] Damage types and resistance vocabularies are pack-declared and extensible
      (a pack may add elemental types without engine changes).

## 5. Armor proficiency

Mirroring weapon proficiency (`weapon-identity §3`), a character holds an
**armor-proficiency set** — the armor tiers/categories they may wear without
penalty — **granted by class** and composed across a multiclass character's
classes (`progression §4.7`). It is a static capability set, not use-gained.

Wearing armor **outside** that set imposes the **non-proficient consequence**:
the armor's check penalty (§6) is extended to the wearer's **attack rolls** (and,
per §6, applies to Str/Dex skills as it always does). Proficient wear applies no
attack penalty. As with weapons, this gates *effectiveness*, not *equipping* —
non-proficient armor may still be worn, just clumsily — and equipping
non-proficient armor emits a one-time cue to the wearer.

Mobs, having no class, wear any armor without penalty unless content declares
otherwise.

**Acceptance criteria**

- [x] The armor-proficiency set is class-granted and composed across multiclass.
- [x] Wearing non-proficient armor extends its check penalty to attack rolls;
      proficient wear adds no attack penalty.
- [x] Non-proficient armor may still be worn; equipping it emits a one-time cue.
- [x] A mob with no class wears any armor without penalty unless content states
      otherwise.

**Status (2026-06-16):** BUILT. A class declares `ArmorProficiencyTiers`
(`armor_proficiency_tiers:` YAML), unioned across a multiclass character;
`connActor.IsArmorProficient` checks the worn tiers (`armorTiers`, snapshotted
on equip) against the grants (`item.ArmorProficient`). When non-proficient, the
attacker's summed `armor_check` penalty extends to the to-hit roll via the
`HitModAdjust` seam (`cmd/anothermud/main.go`), composing additively with the
weapon/darkness/condition penalties. Equipping tiered armor the class lacks
emits a one-time clumsy-wear cue. Mobs have no class → always proficient. The
WoT channelers grant `[light]`; the demo great helm (heavy) shows the penalty +
cue. **Known simplification:** the attack penalty uses the *total* `armor_check`
(over-penalizes the rare proficient-shield + non-proficient-body mix); the
precise per-piece attribution is a later refinement.

## 6. Armor check penalty

A worn armor's **check penalty** applies to the wearer's **Strength- and
Dexterity-based skill checks** while worn (climbing, stealth, etc.) — always, not
only when non-proficient. A shield's check penalty applies likewise and stacks
with body armor's. Non-proficiency (§5) additionally extends the penalty to
attack rolls.

The check penalty composes through the same skill-check resolution the skills
feature already exposes (`skills.md`); it is one more contributor to the
effective check bonus, summed before the roll.

**Acceptance criteria**

- [ ] A worn armor's (and shield's) check penalty reduces Str/Dex skill checks
      while worn, stacking across pieces.
- [ ] The check penalty reaches attack rolls only when the wearer is
      non-proficient with the armor (§5).

**Status (2026-06-16):** the **skill-check** application is BUILT — a worn
armor's grade-reduced check penalty is summed on the `armor_check` stat
(applied at equip) and subtracted from Str/Dex skill checks (the `pick`
Open-Lock check today; gated to Str/Dex governing stats). The **attack-roll**
extension when non-proficient is now BUILT (§5, 2026-06-16).

## 7. Donning and removing

Putting on and taking off armor **takes time**, scaled by the armor's bulk (the
source's don / don-hastily / remove durations). The durations are armor metadata
(§2); the exact action model (a simple timed equip, or an interruptible timed
action) is policy (§9).

A **hastily donned** armor is worn faster at a cost: its **armor bonus and check
penalty are each one step worse** than normal until it is properly re-seated.

Armor declaring no durations dons and removes instantly (the default for minimal
content).

**Acceptance criteria**

- [x] Donning/removing armor that declares durations takes the configured time;
      armor without durations is instant. **SHIPPED 2026-06-17** as a *combat gate*
      (the chosen §9 action-model policy, Decision 0): "takes time" is translated
      to "can't be managed mid-fight" rather than a wall-clock wait. Slow armor
      (medium/heavy tier) cannot be equipped or removed while in combat; light
      armor, shields, and untiered gear stay free (`internal/command/armordon.go`).
- [x] Hastily donned armor applies a worsened armor bonus and check penalty until
      properly donned. **SHIPPED 2026-06-21** as the `hastydon <item>` verb
      (alias `quickdon`): it rides the action-economy don timer at 1/3 the time
      (`action-economy.md` §7.2) and applies −1 armor bonus / +1 check as
      degraded equipment modifiers under the item's source key, so they reverse
      on unequip and a proper re-don restores full protection (no separate
      persisted hasty state needed). Still subject to the in-combat slow-armor
      gate. `internal/command/armordon.go` + `equipment.go`.

## 8. Interaction with existing systems

- **Combat** (`combat §4.4`–`§4.5`): §3 feeds the `defense` channel (single AC,
  capped Dex); §4 adds the per-type **mitigation step** to the damage roll. The
  to-hit roll, fumble, and crit-dice shapes are otherwise unchanged.
- **Channel map** (`combat §4.4`): `defense` gains armor terms + the Dex cap;
  `mitigation` gets its first real consumer (per-type resistance). Both remain
  content-defined formulas read by name.
- **Weapon identity** (`weapon-identity §2`–§3): the recorded **damage type** is
  now consumed by §4; armor proficiency mirrors weapon proficiency (§5).
- **Progression / classes** (`progression §4.7`): armor proficiency is a
  class-granted, multiclass-composed capability set, like weapon proficiency.
- **Skills** (`skills.md`): the armor check penalty is a contributor to Str/Dex
  skill checks (§6).
- **Equipment** (`inventory-equipment-items §3`): armor and shields are ordinary
  slotted items; slot rules enforce one body armor / one shield.
- **Encumbrance** (increment I): the armor **speed** penalty and weight caps are
  that slice's concern — cross-referenced, not built here.

## 9. Configuration surface

| Setting | Meaning | Default |
|---|---|---|
| Unarmored AC base | The baseline AC with no armor (§3). | the existing AC baseline |
| Armor-tier vocabulary | The ordered set of armor tier/category names (§2, §5). | the WoT pack tiers (light / medium / heavy) |
| Non-proficient armor consequence | How wearing out-of-set armor is penalized (§5). | the armor check penalty extended to attack |
| Post-mitigation damage floor | Whether fully-resisted damage floors at 1 (on a hit) or may reach 0 (§4). | min-1 (WoT); full-absorb available per pack |
| Damage-type / resistance vocabulary | The pack-declared types mitigation is keyed over (§4). | `weapon-identity`'s set (B/P/S), extensible (e.g. fire, electricity) |
| Don / remove / hasty durations | Per-armor timing (§7). | per-armor metadata; instant when absent |
| Hastily-donned penalty step | How much worse the bonus and check penalty get (§7). | one step |

All numeric magnitudes live here; the prose names behaviors, not values.

## 10. Decisions and open questions

**Decided (resolves the proposal §7 AC-model fork):**

- **AC is single and layered (decompose-and-cap), not replaced.** Armor adds an
  armor bonus and a max-Dex cap to a decomposed `defense` value; unarmored and
  legacy AC are unchanged. "Replace the AC" was rejected — d20/WoT armor is
  additive-with-a-cap (armor + shield + Dex + misc stack), and replacement breaks
  legacy AC sources.
- **Per-damage-type differentiation lives on `mitigation`, not on AC.** Per-type
  resistance (physical *and* elemental) is the damage-reduction (soak) primitive
  on the `mitigation` channel — already subtracted from damage as a scalar
  (zero today); this slice makes it per-type and is its first real source. This serves a future
  Shadowrun-style pack's elemental resistances correctly and keeps WoT armor
  faithful to its single-bonus table. Per-type *AC* was rejected as a category
  error (you resist fire, you don't dodge a "fire AC").
- **Damage types (increment D) are consumed here.** The type recorded inertly by
  `weapon-identity §2` now keys mitigation; no separate D slice.

**Still open (non-blocking):**

- **Post-mitigation floor** — min-1 vs full-absorb is configured per ruleset
  (§9); whether any ruleset wants a partial floor (e.g. min-1 for physical,
  full-absorb for elemental) is a later refinement.
- **Don/doff action model** — a simple timer vs an interruptible timed action
  (the cast-time machinery of `abilities-and-effects §4.9` is a candidate if
  interruption is wanted). v1 may ship a simple timer.
- **Typed-bonus stacking** — d20's "same-type bonuses don't stack" (two deflection
  bonuses) is bookkeeping Decision 0 may drop; this spec stacks distinct sources
  and relies on slot exclusivity, leaving fine-grained typed-bonus rules as a
  later call.
- **Armor speed penalty / encumbrance** — increment I.
- **Shield bash** — using a shield as an off-hand weapon is increment J.

---

<!-- Spec style: narrative + acceptance criteria · Detail level: behavior only · Status: §3 (AC composition) + §5 (armor proficiency) + §6 (attack-roll extension) BUILT 2026-06-16; §4 (per-type mitigation) shipped earlier with masterwork; §7 (don/doff timers) deferred (instant default). EPIC S1 increments E + D · AC-model fork resolved: single AC on `defense` (Dex term WoT-only via channel-map override), per-type resistance on `mitigation` -->
