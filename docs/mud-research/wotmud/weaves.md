# WoTMUD — Channeler Class & the In-Game Weave List

Source: https://wotmud.info/channeler-class-wheel-of-time-online-game/ and https://wotmud.info/weaves/ (both embed the Fandom wiki via `data-resource`; authoritative source: https://wotmud.fandom.com/wiki/Channeler and https://wotmud.fandom.com/wiki/Weaves)
Fetched: 2026-06-18

> This file captures the **channeler class page + the verbatim in-game weave
> table** (the data). It **complements** the sibling `channeling.md`, which is a
> design *analysis* of how WoTMUD translated the One Power for AnotherMUD's S2
> scoping. Numbers here are from the Fandom weave table and supersede the
> abbreviated/round-number list quoted in `channeling.md`.

---

## Channeler class

> "Channelers are a class who can Channel the One Power, a metaphysical energy
> source that functions the same as 'magic'. Using the One Power, Channelers can
> utilize abilities called Weaves… a wide variety of effects including healing
> and damage-dealing."

> "Gameplay is radically different between male and female channelers due to
> fundamental Wheel of Time lore. Though game and roleplay elements vary
> tremendously because of this, **mechanically speaking channeling as a male or
> female is a similar experience** and pracs are used in much the same way."

### Two prac pools

> "Unlike the other classes, channelers have two prac pools. One is for normal
> skills, which increase at the same rate as a non-channeling character. The
> other pool is for channeling-related skills trainable only by this class."

Channelers are **prac-handicapped in every base-class area** because of their
damage ability (this is the balance lever), guaranteeing they "can never become
as diverse in their skills [as] the other base classes, thereby forcing them to
rely more often on weaves as they progress."

### Stat modifiers

| Str | Int | Wil | Dex | Con |
|:---:|:---:|:---:|:---:|:---:|
| -2 | +3 | +3 | -2 | -3 |

### Base-class practice costs

| | Warrior | Hunter | Rogue | Channeler |
|--|:---:|:---:|:---:|:---:|
| **Male Channeler**   | 3 | 2 | 3 | 1 |
| **Female Channeler** | 4 | 2 | 3 | 1 |

> "MCS have a slight advantage in warrior training but this is massively offset
> by the large number of maluses males face in gameplay."

### Channeler practices per level (the channeling prac pool)

| Levels | Pracs per level |
|--------|:---:|
| 1–10  | 3 |
| 11–15 | 5 |
| 16–25 | 7 |
| 26–40 | 8 |
| 41–51 | 2 |

### Trainers

- **Female channelers** (pre-clan): practice at **Selaana**, in Whitebridge.
- **Male channelers**: practice at **Mazrim Taim**, *or* `tell guardian pracs`
  to teleport to a practice area in the Circle of Light — but only if **(1)**
  healthy, **(2)** not NO QUIT, and **(3)** have not already practiced any
  elements or weaves.

### Weave success & range

> "The practice percentage of the weave determines its pass/fail success. For a
> few weaves, such as Locate Life, Locate Object, Travel, and Gate, the practice
> percentage also determines the **effective range** of the weave. **A failed
> weave still costs the same amount of sps even if it fails.**"

> Channeling proficiency splits into **elemental knowledge** and **weaves**;
> each weave requires proficiency handling the elemental forces it uses. The core
> build choice: **specialize** in high-level weaves on a few elements, or
> **diversify** across elements/weaves while sacrificing access to higher-tier
> weaves.

---

## Elements

The five basic forces. **Elements cost 2 pracs per level**, always advance
**one level per session**, and the percentage per session is **fixed (not
stat-influenced)**.

The five elements: **Earth, Air, Fire, Water, Spirit.**

### Total element prac cost (cumulative)

| Element level | Prac cost | Total pracs |
|:---:|:---:|:---:|
| Level 1 | 2 | 2 |
| Level 2 | 4 | 6 |
| Level 3 | 6 | 12 |
| Level 4 | 8 | 20 |
| Level 5 | 10 | 30 |
| Level 6 | 12 | 42 |
| Level 7 | 14 | 56 |

(Example from the wiki: pracing Fire to level 4 = 2 + 4 + 6 + 8 = **20 prac
sessions**.)

---

## Weaves

Each weave costs **1 practice session**; the percentage per prac is **based only
on Intelligence**. Using a weave spends **Spell points (SPs)** — see
`channeling.md` for the SP pool model.

**Column legend:** *Clan* = requires clanning/org membership · *Elements* =
required element levels (e.g. `4F` = Fire 4; `2A 3W` = Air 2 + Water 3) ·
*SPs* = spell-point cost · *Pulses* = cast-time/execution timer in pulses ·
*Duration* = effect duration in tics · *On Self / On Engaged / While Engaged* =
usage contexts.

### Full weave table (verbatim from the Fandom wiki)

| Weave | Clan | Elements | SPs | Pulses | Duration | On Self | On Engaged | While Engaged | Effect |
|-------|:----:|----------|:---:|:------:|----------|:-------:|:----------:|:-------------:|--------|
| **Armor** | No | 2S | 5 | 5 | 1 tic per Level | Yes | Yes | No | Adds **10 DB** to target. |
| **Blind** | No | 1E 1F 1S | 10 | 10 | 3 tics | Yes | Yes | No | Inflicts *Blind*: reduces OB & PB by 50%, DB by 20%. |
| **Call Lightning** | No | 3A 1F 2W | 12 | 10 | — | No | Yes | Yes | High damage. **Only outdoors during a lightning storm.** |
| **Change Weather** | No | 2A 3W | 7 | 10 | 1 tic (stacks) | — | — | Yes | Makes weather hotter/colder, or causes/removes inclement weather. |
| **Chill** | No | 1W | 10 | 5 | 6 tics | Yes | Yes | Yes | Reduces target OB, DB, PB by 10% each. |
| **Contagion** | Yes | 4E 3S | 13 | 15 | 3 tics | Yes | Yes | Yes | Inflicts *Contagion*: reduces OB & PB by 50%, DB by 20%. |
| **Create Fog** | No | 2A 3W | 7 | 10 | 1 tic (stacks) | — | — | Yes | Fogs an entire zone after weaving multiple times. |
| **Create Food** | No | 1E | 5 | 5 | — | — | — | Yes | Adds *a strange fruit* to inventory. |
| **Create Phantom Object** | No | 3E | 15 | 10 | 15 tics | — | — | No | Creates a non-removable object in the room. |
| **Create Water** | No | 1W | 5 | 5 | — | — | — | Yes | Fills the targeted container with water. |
| **Cure Blindness** | No | 1F 3S | 10 | 10 | — | Yes | — | — | Removes *Blind* status from target. |
| **Cure Critical Wounds** | Yes | 1E 5W 2S | 12 | 25 | — | No | — | — | Heals ~**23–67 HP**. |
| **Cure Fear** | Yes | 4S | 10 | 10 | — | No | — | — | Removes *Fear* status. |
| **Cure Light Wounds** | No | 1E 1W 2S | 7 | 5 | — | No | No | Yes | Heals ~**6–22 HP**. |
| **Cure Poison** | Yes | 4E 3W | 12 | 20 | — | Yes | — | No | Removes *Poison* status. |
| **Cure Serious Wounds** | Yes | 1E 4W 2S | 10 | 15 | — | No | — | — | Heals ~**9–52 HP**. |
| **Deafen** | No | 2E 1S | 10 | 5 | — | Yes | Yes | No | Target can no longer hear tells/chats/narrates/says (can still see emotes). |
| **Earthquake** | No | 4E | 15 | 8 | — | No | Yes | Yes | Engages all targets in room; knocks all riders off mounts into sitting. |
| **Elemental Staff** | Yes | 4A 3F 5W | 25 | ? | 3 tics | — | — | — | Creates & wields a two-handed woven staff for the duration. |
| **Fear** | No | 2S | 15 | 13 | 3 tics | No | Yes | Yes | Inflicts *Fear* status. |
| **Fireball** | Yes | 7F | 15 | 13 | — | No | Yes | Yes | High damage depending on temperature. |
| **Flame Strike** | Yes | 4F | 10 | **9** | — | No | Yes | Yes | Low damage depending on temperature. (9-pulse timer = the reliable combat weave.) |
| **Freeze** | Yes | 3A 5S | 15 | 10 | 3 tics | No | Yes | No | Encases target in ice — unable to move or enter commands. |
| **Gate** | Yes | 7E 4S | 40 | 50 | — | — | — | — | (Long-range teleport; range scales with practice %.) |
| **Hailstorm** | Yes | 1A 4W | 10 | 9 | — | No | Yes | Yes | Low damage depending on temperature. |
| **Hammer of Air** | Yes | 4A | 12 | 9 | — | No | Yes | Yes | Low damage with a small chance to Bash. |
| **Heal** | Yes | 1E 6W 2S | **50** | 50 | — | No | — | — | Heals ~**60–150 HP** (most expensive weave). |
| **Hurricane** | Yes | 6A | 15 | 15 | — | No | — | — | — |
| **Ice Spikes** | No | 3A 3W | 15 | 13 | — | Yes | Yes | Yes | High damage depending on temperature. |
| **Incinerate** | Yes | 7E 7F 7S | 25 | 25 | — | — | — | — | (Top-tier; requires Earth 7, Fire 7, Spirit 7.) |
| **Light Ball** | No | 1A 1F | 5 | 5 | 1 tic per Level | — | — | Yes | Creates a light source for the caster. Cannot be darkened. |
| **Locate Life** | Yes | 2E 2A 4W 2S | 12 | 15 | — | — | — | No | Shows the general location (by zone) of a target. Range scales with %. |
| **Locate Object** | Yes | 4E 2A 2S | 12 | 15 | — | — | — | No | Shows the general location (by zone) of an object. Range scales with %. |
| **Poison** | Yes | 4E 3W | 10 | 15 | 3 tics | Yes | — | — | Inflicts *Poison* status. |
| **Refresh** | No | 2E 2W 3S | 15 | 20 | — | No | — | No | Recovers a high amount of MVs for players & mounts following the channeler. |
| **Remove Contagion** | No | 2E 2S | 10 | 20 | — | Yes | — | — | Removes *Contagion* status. |
| **Remove Warding** | No | 1E 1A 3S | 10 | 20 | — | — | — | — | Removes wards created by Ward Object. |
| **Sense Warding** | No | 2S | 10 | 5 | — | — | — | — | — |
| **Shield** | Yes | 5S | — | — | — | — | — | — | Renders target unable to sense the True Source while shielded (the channeler-vs-channeler disable). |
| **Silence** | No | 2E 1S | 10 | 2 | 6 tics | Yes | — | — | Renders target unable to use channels. |
| **Sleep** | Yes | 3A 5S | 10 | 25 | — | Yes | — | — | — |
| **Slice Weaves** | Yes | 4F 4S | 8 | 7 | — | — | — | — | **Interrupts the target's weaves.** |
| **Slow** | Yes | 3A 5S | 10 | 13 | — | Yes | — | — | Inflicts loss of half the target's movement points. |
| **Sonic Boom** | No | 3A 1F 2W | 15 | 13 | — | Yes | — | — | Damages all targets in room not following the channeler. Chance to Deafen. |
| **Strength** | No | 3E 3W 2S | 12 | 15 | — | No | — | — | Increases target Str based on channeler's (level/10), **max +3**. |
| **Sword of Flame** | Yes | 3E 4A 5F | 30 | 14 | — | — | — | — | Creates & wields a two-handed woven medium blade for the duration. |
| **Travel** | Yes | 7E 2S | 20 | 20 | — | — | — | — | NO QUIT status increases the fail rate. Range scales with %. |
| **Ward Object** | No | 3S | 10 | 10 | — | — | — | — | Targeted object cannot be picked up off the ground by anyone else. |
| **Warding vs Damage** | Yes | 4E 4A 4W | 25 | 25 | 3 tics | Yes | — | — | Reduces damage received by 1/2 to 1/4 for the duration. |
| **Warding vs Evil** | No | 1S | 7 | 10 | 4 tics | Yes | — | — | Makes target appear friendly to enemy forces for the duration. |
| **Whirlpool** | Yes | 5W | 20 | 13 | — | — | — | — | — |

*(Cells marked `—` were blank/greyed in the source table; `?` and "Verify?" notes
in the wiki indicate values the WoTMUD wiki itself flags as unconfirmed.)*

---

## Quick reference: SP-cost tiers

| SP | Weaves |
|----|--------|
| 5  | Armor, Create Food, Create Water, Light Ball |
| 7  | Change Weather, Create Fog, Cure Light Wounds, Warding vs Evil |
| 8  | Slice Weaves |
| 10 | Blind, Chill, Cure Blindness, Cure Fear, Deafen, Hailstorm, Poison, Remove Contagion, Remove Warding, Sense Warding, Silence, Sleep, Slow, Ward Object, Flame Strike |
| 12 | Call Lightning, Cure Critical Wounds, Cure Poison, Hammer of Air, Locate Life, Locate Object, Strength |
| 13 | Contagion |
| 15 | Create Phantom Object, Earthquake, Fear, Fireball, Freeze, Hurricane, Ice Spikes, Refresh, Sonic Boom |
| 20 | Travel, Whirlpool |
| 25 | Elemental Staff, Incinerate, Warding vs Damage |
| 30 | Sword of Flame |
| 40 | Gate |
| 50 | Heal |

---

## See also

- `channeling.md` (existing) — design analysis of WoTMUD's One-Power
  translation for AnotherMUD S2 scoping (SP pool model, `embrace`/`seize`/
  `channel`/`release` verbs, interrupt-vs-round combat, gender asymmetry).
- `classes-overview.md` — class system + cross-class prac economy.
