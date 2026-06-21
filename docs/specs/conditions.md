# Conditions & Status Effects

EPIC sub-epic **S5** — the condition vocabulary of the WoT Mechanics program
(`docs/themes/wot-mechanics-epic.md`, row S5). Governed by EPIC **Decision 0**
(translate WoT onto the existing tick/chance model; no d20 rewrite). Builds on
the shipped **effects** system (`abilities-and-effects`) and the **saves**
primitive (`saves.md`, EPIC S6).

## 1. Overview

The WoT RPG tracks ~25 character **conditions** (`docs/wot/encounters.md`,
*Character Conditions Summary*) — prone, stunned, blinded, frightened,
fatigued, and so on. Most of the table is **tabletop-only**: action-economy
states (dazed/staggered "one action per round"), the HP-state machine
(dying/disabled/stable), and grapple maneuvers (pinned/held) presuppose
mechanics the tick engine deliberately does not have (Decision 0 / EPIC §3).

This slice translates the **combat-meaningful** conditions onto the engine's
existing seams. A condition **is an effect** — the `effect` system already
carries flags, stat modifiers, a duration, and a tick (`abilities-and-effects`
§5). The new work is:

- **A — a recognized condition vocabulary** (a small flag namespace the combat
  and save layers gate on), shipping the **Core 5**: fatigued, prone, blinded,
  frightened, stunned.
- **B — combat hooks** that read condition flags: an *incapacitated* attacker
  skips its swings; a *vulnerable* victim (prone/stunned/blinded) is easier to
  hit; condition penalties feed the existing attacker to-hit seam.
- **C — the save marriage (S6):** a condition application may be **resisted by a
  save**, and a condition may grant a **recurring save** each tick to shake off
  early.
- **D — an inflict path:** a combat **ability** that applies a condition (gated
  by a save) plus an admin **`afflict`** verb, so conditions are observable
  before the systems that will be their main sources (S2 weaves, S7 poison/fear)
  exist.

**Goals.** Reuse the effects substrate and the saves primitive rather than
building a parallel system; make the translatable conditions matter in combat;
give S2/S7 a ready condition vocabulary + the save-gated apply/shake-off
pattern.

**Non-goals (this slice).** The deferred condition families and why:
- **HP-state conditions** (dying / disabled / staggered / stable / helpless /
  unconscious) — need a combat HP-state machine the engine lacks (death is
  binary today). Out of scope. *(Exception since 2026-06-21: **unconscious**
  now exists — `subdual-damage.md` §3 — built without an HP-state machine
  because an **external trigger** (the subdual knock-out) applies it rather
  than an HP threshold; the rest of the family still awaits the machine.)*
- **Damage-over-time conditions** (bleeding / poison / disease) — DoT is **S7**;
  effects modify stats, not HP. Deferred with S7.
- **Grapple family** (grappled / pinned / held / paralyzed) — need the grapple
  maneuver system (tabletop). Deferred.
- **Initiative / morale-flee-geometry / action-economy** states (cowering,
  panicked item-drop, deafened −initiative, checked, flat-footed) — no
  initiative or action economy in the engine.
- **Command-level action gating** — the most disabling conditions do **not**
  block player verbs at the dispatcher this slice (combat-only suppression,
  decided). A stunned character can still `look` / `flee` / `say`; it just
  cannot land combat swings.

## 2. Conditions are flagged effects (A)

A condition is an ordinary **effect** (`abilities-and-effects` §5) that carries a
**condition flag** — a reserved flag name the combat and save layers recognize.
A condition effect may also carry stat modifiers and a duration like any effect;
the flag is what gives it *behavioral* meaning beyond its modifiers.

The **Core 5** and their translation:

| Condition | Flag meaning | Engine translation |
|---|---|---|
| **Fatigued** | tiredness | stat modifiers only (−Str / −Dex). No new seam — pure effect modifiers. The "free" end of the spectrum. |
| **Prone** | knocked down | the wielder's melee to-hit takes a penalty **and** incoming melee gets a bonus (you're easier to hit). Cleared by the `stand` verb (§5). |
| **Blinded** | cannot see | a large to-hit penalty (you swing nearly blind) **and** incoming attacks get a bonus. |
| **Frightened** | fear | a morale to-hit penalty **and** a save penalty (§4), and the victim is compelled to **flee** combat. |
| **Stunned** | incapacitated | the victim **cannot land combat swings** (incapacitated) **and** is easier to hit. |

Conditions compose: a character may carry several at once (each is a distinct
effect id), and their penalties/flags sum. The single-instance-per-id rule
(`abilities-and-effects` §5.2) still holds — re-applying the same condition
refreshes or is ignored per the effect's `refreshable` flag.

**Acceptance criteria**

- [ ] A condition is an effect carrying a recognized condition flag; it may also
      carry stat modifiers and a duration.
- [ ] The Core 5 (fatigued, prone, blinded, frightened, stunned) each load as
      effect content and apply their flag + any modifiers.
- [ ] Fatigued is purely stat modifiers (no behavioral hook) and works today
      with no new combat code beyond the vocabulary.
- [ ] Multiple conditions coexist on one target; their penalties and flags sum.
- [ ] An unrecognized flag on an effect is inert (no condition behavior) — the
      vocabulary is a closed set the engine reads; arbitrary effect flags are
      unaffected.

## 3. Combat hooks (B)

Three behavioral translations, all in the auto-attack pipeline (`combat §4`),
all keyed on condition flags the host resolves from the live effect set. Combat
cannot import the effect/progression layer, so the host injects predicates and
deltas (the same decoupling the darkness/proficiency penalties already use,
`combat §5.3` / `weapon-identity §3`).

- **Incapacitation (skip swing).** When the **attacker** carries an
  incapacitating condition (stunned), it lands **no combat swings** this round.
  Combat stays engaged — the character is not disengaged, takes no swing, and
  resumes swinging when the condition ends. A new attacker-keyed predicate on
  the auto-attack config; nil ⇒ never incapacitated (pre-slice behavior).
- **Vulnerability (easier to hit).** When the **defender** carries a
  vulnerability condition (prone / stunned / blinded), incoming swings get a
  to-hit **bonus**. A new *defender-keyed* to-hit adjustment — the mirror of the
  existing attacker `HitModAdjust` (which darkness + proficiency use). The host
  computes the bonus from the target's condition flags; nil ⇒ no bonus.
- **Attacker penalty (−to-hit).** When the **attacker** carries a penalty
  condition (prone / blinded / frightened), its swings take a to-hit
  **penalty**. This needs **no new seam** — it is one more contributor to the
  existing attacker `HitModAdjust`, summed before the roll alongside darkness and
  the non-proficient penalty.

Combat is **never blocked** by a condition beyond the swing-skip: a natural-20
still auto-hits, movement and flee are unaffected, and the round loop runs
normally. Conditions degrade effectiveness; they do not freeze the engine.

**Acceptance criteria**

- [ ] An incapacitated (stunned) attacker lands no swings while the condition is
      active, then resumes when it ends; it is not disengaged.
- [ ] A vulnerable (prone/stunned/blinded) defender takes incoming swings at a
      configured to-hit bonus, applied every swing while the condition holds.
- [ ] A penalty (prone/blinded/frightened) attacker swings at a configured to-hit
      penalty, summed additively with darkness and proficiency penalties into the
      effective hit modifier (`combat §4.4`).
- [ ] The vulnerability bonus and the incapacitation skip are independent — a
      stunned victim is both unable to swing (as attacker) and easier to hit (as
      defender).
- [ ] A natural-20 still auto-hits regardless of any condition; combat is never
      frozen, only degraded.
- [ ] Mobs and players flow through the same hooks (a stunned mob skips its
      swings; a prone mob is easier to hit).

## 4. The save marriage (C) — resist on apply, shake off over time

Conditions consume the saves primitive (`saves.md`) at two points:

- **Entry save (resist on apply).** A condition application may declare a
  **save** (axis + DC). When something tries to apply the condition, the target
  rolls that save (`saves §3`); **on success the condition is not applied**
  (resisted), and a `SaveResolved` event reports it. On failure the condition
  lands normally. An application with no declared save always lands (admin
  `afflict`, a guaranteed effect).
- **Recurring save (shake off).** A condition may declare a **recurring save**
  (axis). On each effect tick, the target re-rolls against the condition's DC;
  **on success the condition ends early** (the effect is removed before its
  duration expires). This is the "save each round to shake off fear / stun"
  rule. A condition with no recurring save simply runs its full duration.

The **save penalty** from a fear condition (frightened, §2) feeds these checks:
the target's effective save bonus (`saves §2`) is reduced by the morale penalty
of any active fear condition before the roll — so being frightened makes you
*worse* at shaking off the next effect, matching the source's "−morale to saves".

The save bonus for these checks is resolved from the target the same way the
massive-damage save resolves it (`saves §4`): a player composes class saves +
ability modifier; a classless mob uses the ability modifier alone. The effect
layer obtains it through an injected provider (it does not reach into session).

**Acceptance criteria**

- [ ] A condition application declaring an entry save is resisted on a made save
      (not applied) and lands on a failed one; the save resolution emits a
      `SaveResolved` event either way.
- [ ] A condition application with no entry save always lands.
- [ ] A condition declaring a recurring save re-rolls each tick and is removed
      early on a success; one with no recurring save runs its full duration.
- [ ] An active fear condition's morale penalty reduces the target's save bonus
      for both the entry and recurring checks.
- [ ] The save bonus is resolved per-target (player class saves + ability mod;
      mob ability mod alone) through an injected provider, with no new
      import cycle.

## 5. Inflict, clear, and the forced-flee path (D)

**Inflicting (v1 sources).** Two sources make conditions observable before S2 /
S7:

- **A combat ability** that applies a condition, gated by an entry save. The
  existing ability-resolution path already applies effect templates
  (`abilities-and-effects`); a condition-applying ability declares the condition
  effect + an entry save. Shipped examples: a **trip**-style ability that applies
  **prone** (Reflex save to resist) and a **bash**-style ability that applies
  **stunned** (Fortitude save to resist).
- **An admin `afflict` verb** — `afflict <target> <condition> [duration]` applies
  a condition directly (no entry save — admin force), gated on the admin role
  like the other admin verbs (`admin-verbs`). For testing and demos. A matching
  **`cure`** clears a condition (or all conditions) from a target.

**Clearing.** A condition ends by any of: its **duration** expiring (the effect
tick, existing); a **recurring-save** success (§4); the **`stand`** verb
(prone specifically — a player action to get up); **death/recovery** (effects
are dropped on death, existing); or admin `cure`.

**Forced flee (frightened).** A frightened victim is compelled to leave combat:
while the condition is active the victim **attempts to flee** each combat round,
reusing the existing flee mechanic (`combat §5.2`, the same path `wimpy` and the
`flee` verb drive). If no exit is available, the victim is stuck but still suffers
the attack/save penalties. Frightened is the one condition with a movement
consequence; the others are stationary.

**Acceptance criteria**

- [ ] A condition-applying ability rolls its entry save and applies the condition
      only on a failed save; the ability otherwise resolves normally.
- [ ] `afflict <target> <condition> [duration]` applies the condition (admin
      force, no entry save); `cure <target> [condition]` clears one or all.
- [ ] `afflict` and `cure` are admin-gated and refused for non-admins.
- [ ] `stand` clears prone (and only prone); it is a no-op (with a message) when
      the actor is not prone.
- [ ] A frightened victim attempts to flee each round via the existing flee path;
      with no exit it remains but still takes the penalties.
- [ ] Death/recovery clears all conditions (the effect drop path).

## 6. Display

Conditions surface to the player without a new framework:

- The existing **effect listing** (`affects` / the effects surface,
  `abilities-and-effects`) shows active conditions alongside other effects.
- **Combat / application messages** announce transitions: applying ("You are
  knocked prone!", "X is stunned!"), resisting ("You shrug it off!" — a made
  entry save), shaking off (a made recurring save), and clearing.
- The **`score`** sheet (or prompt) MAY show a terse active-condition marker;
  the minimum is that conditions appear in the effect listing and the combat log.

**Acceptance criteria**

- [ ] Active conditions appear in the effect/affects listing.
- [ ] Applying, resisting, shaking off, and clearing a condition each produce a
      player-visible message.

## 7. Interaction with existing systems

- **Effects** (`abilities-and-effects` §5): conditions reuse `EffectTemplate`
  (flags + modifiers + duration + refresh) and the `EffectManager` lifecycle
  (apply / tick / remove / drop-on-death). The new fields are the optional entry
  + recurring save declarations; the new manager behavior is performing those
  saves on apply and tick.
- **Saves** (`saves.md`): the entry and recurring checks call the `ResolveSave`
  primitive; the save bonus is resolved per-target via an injected provider, and
  a fear condition's morale penalty reduces it. `SaveResolved` events flow for
  every condition save.
- **Combat** (`combat §4`–`§5`): two new optional hooks (incapacitation
  predicate, defender vulnerability delta) plus the attacker-penalty contributor
  on the existing `HitModAdjust` seam. Frightened reuses the flee path
  (`combat §5.2`).
- **Abilities** (`abilities-and-effects`): the condition-applying abilities are
  ordinary abilities whose effect carries a condition flag + entry save.
- **Admin / roles** (`admin-verbs`, `roles-and-permissions`): `afflict` / `cure`
  are admin-gated verbs.
- **Light & darkness** (`light-and-darkness §5.3`): the blinded penalty and the
  darkness penalty are distinct contributors to the attacker `HitModAdjust` and
  sum; blinded does not reuse the light resolver (it is its own condition).

## 8. Configuration surface

| Setting | Meaning | Default (engine) |
|---|---|---|
| Prone melee penalty | The to-hit penalty a prone attacker's melee swings take (§3). | the WoT pack value |
| Prone vulnerability bonus | The to-hit bonus incoming melee gets against a prone defender (§3). | the WoT pack value |
| Blinded to-hit penalty | The to-hit penalty a blinded attacker takes (§3). | the WoT pack value (large) |
| Blinded vulnerability bonus | The to-hit bonus against a blinded defender (§3). | the WoT pack value |
| Stunned vulnerability bonus | The to-hit bonus against a stunned defender (§3). | the WoT pack value |
| Fear save/attack penalty | The morale penalty a fear condition applies to attack rolls and saves (§2/§4). | the WoT pack value |
| Default condition durations | The base duration (pulses) of each shipped condition when content omits one. | per-condition pack values |

All numeric magnitudes live here per spec convention; the prose names behaviors,
not values. Each condition's specific penalties are carried as the effect's stat
modifiers / the condition's config row, not hardcoded in the engine.

## 9. Decisions (resolved at slice start)

- **Vocabulary — Core 5.** Fatigued, prone, blinded, frightened, stunned — the
  combat-meaningful set that exercises every new seam. Dazed / entangled /
  exhausted and the deferred families (§1) come later if balance asks.
- **Action suppression — combat-only.** Incapacitating conditions skip combat
  swings and make the victim easier to hit; the command dispatcher is untouched
  (a stunned character can still look / flee / say). Command-level gating
  (paralyzed/held blocking verbs) is deferred — it needs a dispatcher-wide
  precondition seam and the HP-state conditions that justify it.
- **Inflict path — ability + admin verb.** A trip/bash ability (save-gated) plus
  `afflict`/`cure` admin verbs. Mob special-attack condition delivery is deferred
  to when a mob-special-attack hook is designed (S11 Shadowspawn territory).
- **Conditions are effects, not a parallel system.** Reuse `EffectTemplate` +
  `EffectManager`; add the optional save fields and the combat flag-readers. No
  new lifecycle, no new persistence (conditions are ephemeral like all active
  effects — `abilities-and-effects` non-goal on effect persistence stands).
- **Save bonus via injected provider.** The effect layer resolves a target's save
  bonus through a host-injected provider (mirroring `saves §4` FortBonus), so
  `progression` does not import `session`.

### Still open (non-blocking)

- **Effect-present check on the caster** — the ability validation fizzles a
  condition ability when the **caster** already carries the effect's id (it was
  written for self-buffs). For the shipped trip/bash this is coincidentally
  sensible (a prone fighter can't `trip`; a stunned one can't `bash`), but it is
  technically wrong for a *hostile* condition — the check should consider the
  **target**, not the caster. Fix when a condition ability needs to be castable
  while the caster is self-afflicted (gate the check on self-targeted effects).
- Whether a future **condition immunity** axis is wanted (e.g. a construct immune
  to fear) — content can approximate it with a high entry-save DC for now;
  revisit if a real immunity set is needed.
- Whether **prone** should also impose a movement penalty (the source's
  "can't stand and move freely") — v1 keeps prone stationary-until-`stand`
  without a speed model.
- **Command-level gating** for a future paralyzed/held condition — deferred with
  the grapple family; needs a dispatcher precondition seam.
- Whether conditions get a **dedicated `score`/prompt marker** beyond the effect
  listing (§6) — left to the display slice's discretion.
