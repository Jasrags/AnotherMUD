# Research: How WoTMUD ships channeling (a real-time MUD's translation of the One Power)

**Why this doc exists:** scoping AnotherMUD's WoT EPIC **S2 (The One Power)**. Our source
material (`docs/wot/the-one-power.md`) is the *tabletop d20 RPG*, which the EPIC's
Decision 0 says to **translate, not port** onto our tick/chance engine. WoTMUD —
the long-running DikuMUD-derived Wheel of Time MUD — is the most relevant prior
art: a *shipped, real-time, PK-heavy* game that already made that translation. This
captures how they did it so we can borrow the parts that fit and reject the parts
driven by concerns we don't share.

**Sourcing caveat:** `wotmud.info` is JS-rendered and served only nav chrome to an
automated fetch; the authoritative mechanical detail lives on the **WoTMUD Fandom
wiki** (`wotmud.fandom.com`) and the *wotmudnewbie* blog. Numbers/commands/weave
list below are well-sourced and cross-consistent; three items are flagged
**unconfirmed** (player linking, distinct Wise One/Windfinder classes, whether male
madness is a continuous meter vs. social persecution). Primary pages cited at the
bottom.

WoTMUD is a **DikuMUD derivative**, so its channeling reads as a classic DikuMUD
spellcaster re-skinned with WoT vocabulary. That framing is itself the headline
insight: the shipped translation discards almost all d20 channeling machinery.

---

## 1. Resource model — a mana pool ("Spell Points"), one stat, tic-refilled

- **Spell Points (SP)** = the umbrella; **Saidar Points** (female) / **Saidin
  Points** (male) are the gendered labels for the *same* pool.
- **Derived from one stat — `Wil` (Willpower).** Linear formula, **frozen at level
  30** (levels past 30 don't grow it). Ceiling **163 SP at 19 Wil**; every Wil below
  19 subtracts 7 from max.
- **Regenerates only on the *tic*** (the ~60–75s DikuMUD world heartbeat), in chunks
  — *not* continuously like HP. Deliberately slow economy; you cannot spam weaves.
- **Spent on success only.** SP is deducted when the weave's cast timer *completes*.
  Interrupted or cancelled mid-weave → **no SP spent**. So the real cost of a failed
  weave is **tempo, not mana**.
- **The "2× reserve to begin" rule (cheap, clean, worth stealing):** to *start* a
  weave you must hold **twice its SP cost** in reserve (Travel costs 25 → you need 50
  banked or you get "unable to complete the weave"). Only the base cost is actually
  deducted on completion; the 2× is a headroom gate, not a double charge.
- **Holding** a maintained weave drains **11 SP/tic**.
- **No per-weave cooldowns** — pacing comes from cast-timer + tic-gated refill +
  combat-round timing (§6).
- **Angreal/sa'angreal** = external SP batteries: a fixed pool you can draw on to
  *reduce* a weave's SP cost, capped per weave. The temporary-power lever.

---

## 2. Weaves as commands — use-improved skills, not Vancian spells

Command sequence:
1. **`embrace`** (female) / **`seize`** (male) — grab the Source first (lore-faithful
   verbs: a woman submits to saidar, a man forces saidin).
2. **`channel "weave name" <target>`** — the cast (weave name quoted; target
   optional per weave).
3. **`release`** — let go; can't channel while released, mind/body rest.

Prompt carries SP as a third word-rated bar: `* R HP:Healthy SP:Bursting MV:Fresh >`.

**Weaves are use-improved `%` skills** (DikuMUD weapon-skill model), learned/raised
with **practice sessions ("pracs")**: 1 prac per weave, %-gain scaled by Intelligence.
This maps *directly* onto AnotherMUD's existing **use-based proficiency** system — a
weave is just a proficiency that climbs with use.

**Weave list (verbatim names + SP cost), abbreviated:**

| SP | Weaves |
|----|--------|
| 5  | Armor, Light Ball, Create Food/Water |
| 7  | Change Weather, Create Fog, Cure Light Wounds, Warding vs Evil |
| 8  | **Slice Weaves** (interrupt another's weave) |
| 10 | Blind, Chill, Deafen, Poison, Silence, Sleep, Slow, **Flame Strike** (fast, 9 pulses), Hailstorm, several cures, Ward Object |
| 12 | Call Lightning (outdoor only), Cure Critical/Poison, Hammer of Air, Locate Life/Object, Strength |
| 13 | Contagion |
| 15 | Earthquake, Fear, **Fireball**, Freeze, Hurricane, Ice Spikes, Sonic Boom, Create Phantom Object, Refresh |
| 20 | **Travel**, Whirlpool |
| 25 | Elemental Staff, Incinerate, Warding vs Damage |
| 30 | Sword of Flame |
| 40 | **Gate** |
| 50 | **Heal** (most expensive) |
| —  | **Shield** (cut target off from the Source) |

---

## 3. Strength-in-the-Power — trained element tiers + an org gate, not one number

Two gates, *separate from the SP pool*:
- **`Wil` sets SP capacity** (the tank).
- **Strength = how high you've trained each *element*** + organizational membership.
  "Channeling proficiency is broken into elemental knowledge and weaves; each weave
  requires proficiency handling the elemental forces it uses."
- **Hard institutional gate:** level-4+ elements/weaves require **White Tower** (or
  male equivalent) membership.
- **Specialize-vs-diversify is the build identity:** pump a few elements high to
  unlock top-tier weaves, *or* spread wide for breadth but never reach the top tier.

---

## 4. The Five Powers / affinities — modeled as the "elements" skill pool

- Each weave requires proficiency in its element(s); elements are their own trained
  tier (the "elemental knowledge" half).
- **Steep, separate leveling:** **2 pracs/level cumulative** (L1=2 … L7=14), always
  exactly one level per session.
- **Gender affinity modeled, book-faithful:** men stronger with **Fire & Earth**,
  women with **Air & Water**, **Spirit** equal-and-rare in both. Biases which weaves
  each gender reaches/excels at.

---

## 5. Gender — mechanically mirrored, socially asymmetric

- **Near-mirrored mechanically:** same weaves, same SP system; only the access verb
  differs (`seize` vs `embrace`).
- **Males are persecuted *by design*:** female channelers ("fc") progress more
  easily; male channelers ("mc") face nation-based persecution and are hunted by the
  **Red Ajah** for **gentling**. A tiny male warrior-prac discount exists but the
  wiki says it's "massively offset by the maluses males face."
- **Taint/madness — UNCONFIRMED as a player meter.** Saidin is tainted in-lore and
  there's a `tainted Channeler` mob, but I could not confirm a continuous
  player-facing madness *stat*. On WoTMUD the saidin curse seems expressed mainly as
  **social/PK persecution**, not a madness timer. (The d20 source *does* define a
  Madness rating — see §"d20 contrast".)

---

## 6. Combat channeling — timer-vs-round chess, interruption refunds the cost

This is the sharpest PvP tuning and the hardest thing to retrofit:
- **Combat rounds are 12 pulses apart;** weave cast-times are measured in **pulses**.
  The meta is fitting a weave inside the gap between rounds.
  - **Flame Strike = 9 pulses** → slips between rounds, dodges weapon-hit
    interruption → the reliable combat weave.
  - **Fireball ≈ bash timer** → trade a fireball per attempted bash. Higher-damage
    weaves (Fireball, Ice Spikes) have **longer timers = higher interrupt risk**.
    Pure risk/reward.
- **Interruption is the central counter-play, and it refunds SP** (you only pay on
  completion). Melee interrupt verbs:
  - **`kick`** interrupts any weave/charge/bash being readied; costs the kicker
    ~1.25 rounds of self-lag.
  - **`bash`** has its own startup timer that an alert foe can `kick`-interrupt.
  - **`Slice Weaves`** (8 SP) is the channeler's own weave-interrupt.
- **`Shield`** = the decisive channeler-vs-channeler control: cuts the target off
  from the Source so they **cannot channel at all** (a silence/disable).
- **Glass cannon by design:** massive fast burst, low survivability — explicit
  balance lever.
- **Environmental gating:** Call Lightning outdoor-only; Fireball max damage only in
  ideal weather.

---

## 7. Classes / guilds — one `Channeler` class, forked by organization

- **One human-only `Channeler` class.** Differentiation is primarily
  **organizational, not a separate skill tree:** membership unlocks the level-4+
  element/weave gate.
- **Female orgs:** White Tower / Aes Sedai (novice → Accepted → one of 7 Ajah; Red =
  hunts male channelers, Green = battle), plus Wilders, Kin, Dragonsworn.
- **Male orgs:** the male tower (Asha'man in lore). Males default
  persecuted/unaffiliated until they join.
- Prac costs differ slightly by gender; all channelers share the **same weave list**.
  "Guild" ≈ access tier + RP faction + PK-target status on a shared engine.
- **UNCONFIRMED:** distinct Wise One / Windfinder *mechanical* classes — appear to be
  RP flavor folded into the female track, not separate skill sets.

---

## 8. Linking / circles — UNCONFIRMED (likely not a player mechanic)

The channeling pages describe solo embrace/seize/channel/release and mention no
`link`/circle command or amplification. Element-specialization + angreal are the
documented amplification levers. Circles read as **lore-only** on WoTMUD as
documented — but the wiki has stubs, so this is "not found," not "confirmed absent."

---

## 9. Progression

- **Dual prac pools:** one normal-skill pool (warrior/hunter/rogue, same rate as
  non-channelers) + one **channeling-only pool** for elements + weaves.
- Elements: 2 pracs/level cumulative, 1 level/session. Weaves: 1 prac, %-gain by Int.
  **Practicing elements first improves weave skill-gain rates.**
- **SP grows with level only to 30**, then frozen (capped by Wil).
- **`Overchannel`** = draw beyond your limit; risks **immediate permanent stilling**
  (severed from the Source forever). **Stilling** (self-inflicted overchannel) and
  **gentling** (Red Ajah inflicts on captured men) are the permanent-loss sinks —
  real downside + a PK stake.

---

## What WoTMUD's shipped translation teaches us (vs. the d20 source)

The real-time game **threw out almost all d20 channeling machinery** and replaced it:

| d20 tabletop (`docs/wot/the-one-power.md`) | WoTMUD shipped translation |
|---|---|
| Per-level **daily weave slots** (Vancian) | **Single mana pool (SP)**, tic-refilled |
| Weaves = **known spells** | Weaves = **use-improved % skills** |
| Casting strength = **slot capacity + ability score** | **Wil → SP**, plus **trained element tiers** |
| Affinities adjust **effective slot level** | Affinities = **gender bias on element strength** |
| Initiate (Int) vs Wilder (Wis), Talents | **One Channeler class**, org-gated weave tiers |
| Overchannel → Concentration → **Fort save cascade** | Overchannel → **risk permanent stilling** |
| Madness rating (secret 1d6, +1/overchannel) | **Social/PK persecution** (madness meter unconfirmed) |
| Linking circles (Table 9-1) | **Not a player mechanic** (unconfirmed) |
| Casting time in rounds/actions | **Cast time in pulses vs the 12-pulse round** |

### The parts worth stealing for AnotherMUD
1. **Mana pool gated by one stat, refilled slowly** — validates our "Power pool"
   direction over d20 daily slots. Our engine already stubs exactly this
   (`StatResourceMax`, `DeductMana`). Decision 0 says drop the slot bookkeeping; the
   prior art agrees.
2. **Spend-on-success + 2×-reserve-to-begin** — cheap, elegant. The real cost of a
   failed cast is **tempo, not resource**. Pairs naturally with our tick loop.
3. **Weaves = use-based proficiencies** — drops straight onto our existing
   proficiency/use-gain system (the same convention crafting and skills already use).
   A weave known = a proficiency entry; no new persistence.
4. **Cast-time-vs-round + interruption** — the deepest combat lever, and the hardest
   to retrofit. *Design weave cast-times against our tick cadence from day one* so the
   interrupt/tempo game is possible later (our action-queue + combat heartbeat are the
   seam). S5 conditions already give us "stunned skips a swing"; the inverse —
   "getting hit aborts a cast" — is the channeler-side mirror.
5. **`Shield` as a disable** (cut from the Source) — the decisive channeler-vs-
   channeler control; maps to an effect that blocks the `channel` verb.
6. **Permanent sinks** (stilling via overchannel) give channeling real stakes — and
   reuse our shipped **S6 saves** + **S5 conditions** (overdraw → Fort save → cascade).

### The parts to *reject* or defer (driven by WoTMUD concerns we don't share)
- **PK-balance tuning as the primary design driver** — glass-cannon balance, male
  persecution-as-balance, Red Ajah gentling. AnotherMUD isn't PK-first; these are
  flavor we can add later, not load-bearing balance.
- **Org-membership as the weave-tier gate** — our analog is class + trainer tiers +
  proficiency caps; we don't need a White-Tower-membership gate in v1.
- **Level-30 SP freeze, exact 7-per-Wil numbers** — WoTMUD-specific tuning; our pool
  formula rides our own stat model.

---

## Open questions this research surfaces for our S2 decisions
- **Resource model:** WoTMUD's shipped choice (mana pool) backs our recommended
  "single Power pool" over d20 slots. ✔ leans decided.
- **Gender & taint:** WoTMUD mirrors the mechanics and expresses the saidin curse
  *socially*, deferring/omitting a madness meter. Backs our "gendered Source, defer
  taint to its own slice" recommendation.
- **Combat depth:** the cast-time-vs-round interrupt game is WoTMUD's best idea and
  our biggest future-proofing concern — even if v1 weaves resolve simply, pick weave
  cast-times now with the interrupt game in mind.
- **Linking & elements-as-separate-tier:** WoTMUD treats elements as a *second*
  trained axis (specialize-vs-diversify) and likely skips player linking. We can fold
  affinities into the weave-eligibility check (Phase 3) and treat linking as a far
  later slice.

---

### Primary sources
WoTMUD Fandom wiki: [Spell points](https://wotmud.fandom.com/wiki/Spell_points) ·
[Weaves](https://wotmud.fandom.com/wiki/Weaves) · [Channel](https://wotmud.fandom.com/wiki/Channel) ·
[Channeler](https://wotmud.fandom.com/wiki/Channeler) ·
[Embrace](https://wotmud.fandom.com/wiki/Embrace)/[Seize](https://wotmud.fandom.com/wiki/Seize)/[Release](https://wotmud.fandom.com/wiki/Release) ·
[Shield (weave)](https://wotmud.fandom.com/wiki/Shield_(weave)) ·
[Flame Strike](https://wotmud.fandom.com/wiki/Flame_Strike) · [Fireball](https://wotmud.fandom.com/wiki/Fireball) ·
[Overchannel](https://wotmud.fandom.com/wiki/Overchannel) · [White Tower](https://wotmud.fandom.com/wiki/White_Tower) ·
[Angreal](https://wotmud.fandom.com/wiki/Angreal).
Blog: [wotmudnewbie — Weaves](http://wotmudnewbie.blogspot.com/2012/07/weaves.html).
Site: [wotmud.info — player classes](https://wotmud.info/player-classes-in-wotmud/).

*Captured 2026-06-11 for S2 scoping. Companion to `docs/wot/the-one-power.md` (the
d20 source) and `docs/themes/wot-mechanics-epic.md` §2 row S2.*
