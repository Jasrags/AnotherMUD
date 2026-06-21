# Subdual Damage (nonlethal · knock-out · unconscious)

EPIC sub-epic **S1** — the **subdual damage mode**, the prerequisite the
**special-weapons** "bottomless tail" (`special-weapons.md` §2, BACKLOG §1) has
been waiting on: the consumer of the recorded-only **`subdual`** weapon field
(sap / whip / unarmed) and the gate the **whip** tail slice needs. Governed by
EPIC **Decision 0** (translate WoT onto the existing tick/chance model; no d20
rewrite). It layers on **combat** (`combat.md` §4 damage, §6 the death pipeline +
the cancellable `entity.death.check`), **conditions** (`conditions.md` — it finally
builds the first **HP-state-adjacent** condition that spec deferred,
**unconscious**), and **abilities-and-effects** (`abilities-and-effects.md` §5,
the effect substrate the condition rides). *Spec ahead of code — build pending;
sliced so each piece ships independently.*

## 1. Overview

The engine's death is **binary**: a damage application that drives a combatant's
HP to zero emits `VitalDepleted`, and the death pipeline (`combat.md` §6) makes a
corpse, credits the kill, drops loot, and disengages. There is no middle state —
no staggered, no dying, no **unconscious**. `conditions.md` calls this out: the
HP-state conditions (dying / disabled / staggered / stable / helpless /
unconscious) "need a combat HP-state machine the engine lacks" and were put out
of scope.

A **subdual** (nonlethal) weapon — the source's `§` mark on the sap, the whip,
and unarmed strikes (`docs/wot/equipment.md`) — is built "to drop a man without
killing him." It needs exactly the missing middle state: a blow that **takes a
foe out of the fight without ending its life**.

This spec ships that middle state in the **minimal** form the engine's substrate
affords, and wires the `subdual` field to it.

### The model: knock-out at zero (not a parallel damage pool)

The tabletop tracks nonlethal damage in a **separate pool**: a creature is
*staggered* when its nonlethal total equals current HP and falls *unconscious*
when it exceeds current HP, with the nonlethal total healing back far faster than
real wounds. Building that faithfully means a second persisted pool per
combatant, a two-axis damage application, and a staggered state — a parallel
HP-state machine.

Per **Decision 0**, this spec does **not** build that. Subdual damage depletes
the **same** HP pool as any blow; the **only** difference is the **finishing
blow**. When the blow that drives a victim to zero HP came from a **subdual**
source, the victim falls **unconscious** instead of dying. A lethal finishing
blow kills exactly as today.

This is the gameplay-meaningful translation — "a sap knocks you out, a sword
kills you" — within the engine's binary substrate, and it costs **no new pool,
no save bump, no persisted field**. The richer separate-pool model is recorded as
a deferred variant (§8).

### The seam: the cancellable death-check

`combat.md` §6.1 already publishes a **cancellable** `entity.death.check` before
every death is finalized, with a standing contract: *a listener that cancels MUST
restore the victim to a non-dead state.* That is precisely a knock-out hook. The
finishing blow's **subdual-ness** rides the `VitalDepleted` event into the death
pipeline; a **knock-out listener** sees a subdual finish, **cancels** the death,
**restores the victim to 1 HP**, applies the **unconscious** condition, and ends
the engagement. No corpse, no loot, no kill credit — the foe is down, not dead.

### Goals / non-goals

**Goals.** Consume the `subdual` weapon field; give the sap/whip/unarmed their
defining behavior; build the **unconscious** condition as an ordinary flagged
effect on the existing condition machinery (incapacitation + helpless
vulnerability), reusing the death-check cancel seam rather than a new HP-state
machine; keep every non-subdual fight byte-for-byte unchanged.

**Non-goals (this spec).**
- **A separate nonlethal-damage pool** + the *staggered* state (§8 deferred).
- **Coup-de-grace / finishing a helpless foe** (turning an unconscious target's
  knock-out into a kill) — the helpless/coup-de-grace family; deferred (§8).
- **"Attack to subdue" with a lethal weapon** (a per-attack choice to deal
  nonlethal damage at a to-hit penalty) — needs an action-economy / per-swing
  choice the engine lacks (Decision 0). Subdual is a **weapon property**, not a
  chosen mode (§8).
- **Command-level action gating.** Like every other condition (`conditions.md`
  §1), unconscious suppresses *combat swings* only; it does not block the
  dispatcher this slice. (An unconscious player being able to `say` is a known,
  accepted v1 wart — see §8 / open questions.)

### Slices

1. **The `unconscious` condition** (the Core-vocabulary substrate) — a new
   incapacitating + heavily-vulnerable flagged effect, recognized by the engine
   `condition` package and authored as content, exercisable through the existing
   `afflict`/`cure` inflict path. **Inert toward subdual until slice 2** (no blow
   applies it yet) but immediately useful and testable on its own. *(§3.)*
2. **Knock-out** — thread the finishing blow's `subdual` from the wielded weapon
   (`combat.Stats` → the `Hit`/`VitalDepleted` events) into the death pipeline; a
   death-check knock-out listener cancels the subdual death, restores HP, applies
   unconscious, and disengages; the victim wakes when the condition expires.
   *(§4, §5.)*
3. **Whip + content** — the `whip` special tag (subdual + reach + the source's
   "ineffective vs. armor" rule), and pointing the sap / whip / unarmed at the
   mode; a demo + a live walkthrough. *(§6.)*

## 2. What subdual is (and is not) on a swing

A subdual weapon is an **ordinary** weapon for the entire swing pipeline
(`combat.md` §4): the same to-hit roll, the same damage dice + bonuses + crit, the
same soak, the same per-swing minimum of 1. Subdual changes **nothing** about how
much damage a blow deals or how often it lands. It is a single bit — *was this
blow nonlethal?* — read live from the **wielded weapon** at swing time
(`ItemInstance.Subdual()`, already shipped), and it matters at exactly **one**
point: when the blow drives the target to zero HP.

- A subdual weapon whose blow leaves the target **above** zero HP is
  indistinguishable from any weapon (it wounds normally).
- A subdual weapon whose blow drives the target **to** zero HP triggers the
  **knock-out** path (§4) instead of death.
- Mixed sources resolve on the **finishing blow only**: prior subdual wounds do
  not "bank" toward a knock-out, and prior lethal wounds do not poison a later
  subdual finish. v1 reads only the blow that crosses zero (an open question, §9).

### Acceptance criteria

- A subdual weapon's hit deals identical damage (dice, bonus, crit, soak, the
  min-1 clamp) to the same weapon stripped of the `subdual` flag — subdual is
  inert above zero HP.
- The subdual-ness of the **finishing** blow is read live from the wielder's
  weapon at swing time (swapping to/from a subdual weapon changes the next
  finish), not latched at engage.
- An unarmed combatant's finishing blow is subdual when the engine's unarmed
  default is configured subdual (§6); a wielded lethal weapon's finish kills.

## 3. The `unconscious` condition (slice 1)

A new condition in the `conditions.md` §2 vocabulary — the first member adjacent
to the deferred HP-state family, built without the HP-state machine because it is
applied by an **external trigger** (the knock-out, §4) rather than tracked off an
HP threshold. It is an ordinary **effect** carrying the recognized flag
`condition:unconscious`, exactly like stunned/prone:

- **Incapacitating.** An unconscious combatant lands **no** combat swings — it
  feeds the existing `Incapacitated` combat hook (`conditions.md` §3, the same
  hook stunned uses). Knocked out, you do not fight.
- **Helpless (heavily vulnerable).** An unconscious combatant is far easier to
  hit — a large incoming-to-hit bonus (`UnconsciousVulnerability`), stronger than
  prone/stunned, the translation of "defenseless." (A *coup-de-grace* finish on a
  helpless foe is deferred, §8 — v1 stops at "much easier to hit.")
- **Duration-timed wake.** Unlike stunned, unconscious carries **no recurring
  shake-off save** — you do not Fortitude-save your way out of being knocked out
  each round; you **wake when the effect expires** (`abilities-and-effects.md` §5
  duration). Slice 2 owns the on-wake behavior (§5).
- **Curable.** It is a recognized `condition:` flag, so the admin `cure` verb
  clears it and `afflict <target> unconscious` applies it (the `conditions.md` §D
  inflict path) — making the condition observable and testable **before** the
  knock-out trigger that will be its real source exists (the same "observable
  ahead of its real source" posture conditions shipped with).

The magnitudes live in the engine `condition` package config
(`conditions.md` §8 — the same home as the prone/stunned magnitudes), not in
content; the content effect carries only the flag + duration.

### Acceptance criteria

- An entity bearing `condition:unconscious` is **incapacitated** (skips its combat
  swings) and **vulnerable** (incoming attackers gain `UnconsciousVulnerability`
  to hit) — resolved by the leaf `condition` fold, no host special-case.
- The condition carries a **duration** and **no recurring save**; it ends only on
  expiry (or `cure`), never on a per-tick shake-off.
- `afflict <target> unconscious` applies it and `cure <target>` clears it through
  the existing inflict path (it is a recognized condition flag).
- Inert toward subdual in slice 1: no combat blow applies it yet — only the
  admin verb does — so shipping the condition changes no fight until slice 2.

## 4. The knock-out (slice 2)

The finishing blow's subdual bit threads from the wielded weapon onto
`combat.Stats` and out on the `VitalDepleted` event (and the `Hit` event, for
renderers). The death pipeline (`combat.md` §6) reads it:

1. A subdual finishing blow emits `VitalDepleted` carrying **subdual = true** (a
   lethal finish carries false, today's behavior).
2. The **knock-out listener** runs on the cancellable `entity.death.check`
   (`combat.md` §6.1). On a **subdual** death it:
   - **Cancels** the death-check (no corpse, no `Kill`/`MobKilled`, no loot, no
     XP credit — the §6.1 cancel path).
   - **Restores** the victim to **1 HP** — discharging the §6.1 canceller
     obligation ("a listener that cancels MUST restore the victim to non-dead").
   - **Applies** the `unconscious` condition (§3) to the victim.
   - **Disengages** the victim from all combat (`DisengageAll`) — the fight is
     over for it; attackers fall out of combat as on a kill.
3. A **lethal** death-check (subdual = false) is untouched — the knock-out
   listener ignores it and the ordinary death pipeline finalizes the kill.

The knock-out rides the **existing** cancel seam — it does not add a new death
outcome to the pipeline, it is one more `entity.death.check` subscriber, ordered
so its cancel pre-empts corpse creation (the cancel short-circuits the pipeline).

### Acceptance criteria

- A subdual finishing blow leaves the victim **alive at 1 HP** with the
  `unconscious` condition, disengaged from combat — **no corpse, no loot, no kill
  credit, no death XP**.
- A lethal finishing blow kills exactly as today (the knock-out listener is a
  no-op on a non-subdual death-check).
- Works uniformly for a **player** or a **mob** victim (the condition + cancel +
  restore are entity-shape-agnostic; the §6.1 path already handles both).
- The restore-to-1-HP discharges the §6.1 canceller obligation, so the
  "cancel-left-a-corpse" operator warning never fires for a knock-out.
- Idempotent under the once-only `VitalDepleted` guarantee (`combat.md` §4 /
  `pool` crossing): a single knock-out per finishing blow, no double-apply.
- Inert without a subdual weapon: with no `subdual` weapon wielded, every death is
  lethal and the pipeline is unchanged.

## 5. Waking up (slice 2)

The `unconscious` condition is a **fixed-duration** effect; when it expires the
victim **wakes**, alive at whatever HP it has recovered (1 HP plus any regen over
the duration). Waking is the ordinary effect-expiry path (`abilities-and-effects.md`
§5) — no new tick:

- A **mob** that wakes is a live mob again; its AI resumes (it may re-aggress its
  attacker, flee, or idle per its disposition). v1 does **not** give it special
  "groggy" behavior — it simply re-enters the world awake.
- A **player** that wakes regains the ability to act in combat (the condition
  lifts → the `Incapacitated` hook clears). v1 adds no groggy after-state.

A victim **healed past a threshold** while unconscious is **not** auto-woken in
v1 (waking is duration-only); damage taken while unconscious applies normally
and **can kill** (an unconscious foe is helpless, §3 — a subsequent **lethal**
blow finishes it; this is the engine's stand-in for coup-de-grace until §8 lands
it properly).

### Acceptance criteria

- The victim wakes on `unconscious` expiry, alive, AI/agency resumed; no separate
  wake tick beyond effect expiry.
- A lethal blow on an unconscious (helpless, §3) victim **kills** it through the
  ordinary death pipeline (knock-out only intercepts *subdual* finishes) — so a
  downed foe can still be finished off with a real weapon.
- A subdual blow on an already-unconscious victim does **not** stack a second
  knock-out (it is at >0 HP after the restore; a fresh subdual finish re-applies
  unconscious, refreshing duration — no error, no double corpse).

## 6. Whip + content (slice 3)

The remaining special-weapons tail item this mode unblocks. The **`whip`**
`special:` tag (validated vocabulary, recorded-only since `special-weapons.md`
slice 1) lights up:

- **Subdual** — a whip is a nonlethal weapon (`subdual: true`), so it knocks out
  via §4.
- **Reach** — a whip strikes at the `near` band (`special-weapons.md` §3 reach),
  authored as `reach: 1`.
- **Ineffective vs. armor** — the source's "a whip deals no damage to a foe with
  armor bonus +1 or natural-armor +3" (`docs/wot/equipment.md`). Translated onto
  the existing soak model: against a sufficiently armored/natural-armored
  defender the whip's damage is fully soaked (it stings but cannot bite). The
  exact translation (a hard cutoff vs. folding into the per-type resistance the
  whip cannot overcome) is settled in the slice; recorded here as the third whip
  behavior.

Content: point the **sap** (already `subdual: true`, the existing demo weapon),
a new **whip**, and the **unarmed default** at the mode; a demo placement + a
telnet walkthrough (`internal/telnettest`) proving a sap fight ends in a
knock-out, not a corpse.

### Acceptance criteria

- A `whip` weapon is subdual (knocks out, §4) and reach-1 (near-band, §3).
- A whip's damage is nullified against a defender at/above the armor / natural-
  armor threshold (the source's anti-armor rule), and bites normally below it.
- The unarmed default knocks out rather than kills when configured subdual, so a
  bare-handed brawl ends in an unconscious loser, not a dead one.
- A telnet walkthrough demonstrates the full knock-out (down at 1 HP +
  unconscious + no corpse) and the wake.

## 7. Configuration surface

| Setting | Meaning | Default |
|---|---|---|
| `unconscious` vulnerability | Incoming to-hit bonus against an unconscious (helpless) defender. Lives with the other condition magnitudes (`conditions.md` §8 engine config), not env. | (engine default, ~4 — stronger than prone/stunned) |
| `unconscious` duration | Effect-ticks a knock-out lasts before the victim wakes. Authored on the content effect. | (content, a handful of rounds) |
| unarmed-default `subdual` | Whether the engine's unarmed strike is nonlethal (a bare-handed brawl knocks out). | (engine/pack default) |
| whip anti-armor threshold | The armor / natural-armor rating at/above which a whip's damage is nullified (§6). | (content / engine default, source: +1 armor / +3 natural) |

The knock-out itself adds **no env knob** — it is structural (a death-check
subscriber). The restore-to-HP value (1) is the §6.1 minimum-non-dead and is not
a tunable.

## 8. Deferred

- **The separate nonlethal-damage pool** (the faithful d20 model): a second pool
  tracking nonlethal damage, *staggered* at equal-to-current-HP, unconscious at
  exceeds, with fast-healing nonlethal. A larger build (a new `pool` kind, a
  two-axis damage apply, a save bump for the persisted pool, the staggered
  condition). v1's knock-out-at-zero is the Decision-0 translation; this is the
  richer variant if the staggered nuance is ever wanted.
- **Coup-de-grace** — deliberately finishing a helpless (unconscious/§3) foe with
  an auto-crit + a save-or-die. v1's stand-in is "a lethal blow on a helpless
  foe just kills normally" (§5). The helpless/coup-de-grace family rides this.
- **"Attack to subdue" with a lethal weapon** — d20's choice to deal nonlethal
  damage with any weapon at a −4 to-hit. Needs a per-swing/stance choice the
  engine lacks (no action economy, Decision 0). Subdual stays a weapon property.
- **Wake-on-heal / variable wake** — v1 wakes on a fixed duration only; healing a
  downed foe past a threshold to rouse it, or a Heal/first-aid wake, is later.
- **Groggy after-state** — a brief post-wake penalty (fatigued/dazed) is not
  modeled; the victim wakes clean.
- **Dispatcher action-gating for unconscious** — like every condition, v1
  suppresses combat swings only; an unconscious player can still `say`/`look`.
  Gating verbs at the dispatcher is the same deferred concern `conditions.md` §1
  records for the whole family.

## 9. Open questions

- **Mixed lethal/subdual finish.** v1 reads only the **finishing** blow's
  subdual-ness (§2). The stricter source reading banks nonlethal vs. lethal
  separately (the separate-pool model, §8). Lean: finishing-blow-only until the
  pool model is wanted — it is simple, predictable, and matches "what hit you
  last."
- **Knocked-out PvP exposure.** An unconscious **player** is helpless (much easier
  to hit, §3) and can be **finished with a lethal weapon** (§5). Is that the
  desired PvP stake (subdual to capture, lethal to kill), or should an unconscious
  player be protected from a lethal finish? Recorded for when PvP balance is
  tuned; v1 leaves the helpless foe finishable (the honest reading of "down but
  not dead").
- **Mob knock-out value.** What is the *point* of knocking a mob out vs. killing
  it, if there is no capture/interrogate/rob-the-helpless mechanic yet? v1 gives
  knock-out the same fight-ending result as a kill (minus loot/XP), so a subdual
  weapon is a strictly *softer* takedown. A future "subdue to capture / rob an
  unconscious foe" mechanic would give it teeth. Recorded; not built here.
- **Unconscious + AI on wake.** A woken mob resumes its disposition (§5). Should a
  freshly-woken mob re-aggress the one who knocked it out (grudge), flee
  (self-preservation), or roll normal disposition? v1: normal disposition.
  Recorded for the mob-AI slice.
