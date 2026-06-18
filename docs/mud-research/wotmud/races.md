# WoTMUD — Playable Races

Source:
- https://wotmud.info/race/
- Backing data: https://wotmud.fandom.com/wiki/Race and https://wotmud.fandom.com/wiki/Homeland

Fetched: 2026-06-18

> Sourcing note: wotmud.info/race/ is JS-rendered and embeds
> `https://wotmud.fandom.com/wiki/Race`. Race traits, stat ranges, and class
> restrictions below come from that wiki page via the MediaWiki API. Geography
> (which homeland sits where) lives in `world-homelands-and-zones.md`; this file
> holds racial *traits* and the per-homeland stat tables that gate character
> rolls.

---

## 1. Playable races

Originally **three humanoid races** were playable — **Human, Seanchan, Trolloc** —
but Seanchan was **folded into the Human race**, leaving **two playable races**:

| Race | Side | Notes |
|------|------|-------|
| **Human** | Light / Seanchan | Both Light and Seanchan characters are mechanically Human. Future humans may take the **oaths of the Corenne** and join Seanchan justice clans (Deathwatch, Damane, Sul'dam). |
| **Trolloc** | Dark | All Dark players begin as Trollocs. |

**Remort races** (earned via remort, not picked at creation):
- **Dreadlord** — available to Trollocs and Darkfriends
- **Myrddraal (Fade)** — available to Trollocs and Darkfriends

**Mob-only races** (not playable): **Aiel** and **Ogier**.

---

## 2. Racial stat adjustments

- **Humans** roll **9–19** in every stat category. Homeland + class combinations
  can raise the minimum and/or lower the maximum available.
- **Trollocs** suffer penalties to **Int** and **Wil** (as low as **3** each),
  reflecting their "primitive and bestial nature." But their **Strength can be
  augmented as high as 21** — via Strength Teas (*a cup of thready brown tea*) or
  the **Strength weave**, and certain stocks roll up to 21 Str naturally.
- **Minimum statsum for humans is 70.** Rolls below that statsum are tossed and
  rerolled.
- **Channelers are capped at 280 hit points** (male and female alike), regardless
  of Constitution (implemented Aug 2, 2017).

Stats: STR, INT, WIL, DEX, CON. Modern characters enter with **pre-rolled base
stats** by race+class; homelands now only affect the **rerolling** process (see
`world-homelands-and-zones.md` §2 for the why).

---

## 3. Class restrictions by race

| Race | Allowed classes |
|------|-----------------|
| **Trolloc** | Warrior, Hunter, Rogue |
| **Human** | Warrior, Hunter, Rogue, **Channeler** (male *or* female — with important differences between the two) |

(Channeler mechanics live in the sibling `channeling.md`.)

---

## 4. Language

| Race | Speech / Yell |
|------|---------------|
| Humans & Seanchan | Share a language — understand each other in speech and via **Yell**. |
| Trollocs | Speak their own language — **cannot** understand or be understood by Humans/Seanchan. |
| Fades (remort) | Can understand **both** Trolloc and Human/Seanchan speech. |
| Dreadlords (remort) | Can understand the speech of **all** character types. |

This is a hard cross-faction comms barrier at the race layer — Light and Dark
literally cannot read each other's chat unless one side has remorted.

---

## 5. Per-class stat modifiers

These modifiers apply on top of homeland base stats (Lightside/Seanchan table):

| Class | STR | INT | WIL | DEX | CON |
|-------|-----|-----|-----|-----|-----|
| Warrior | +2 | 0 | -1 | 0 | +2 |
| Hunter (Lightside) | +1 | 0 | 0 | +1 | +1 |
| Hunter (Trolloc/Seanchan) | 0 | 0 | 0 | +1 | 0 |
| Rogue | -1 | 0 | 0 | +3 | 0 |
| Channeler (Human only) | -2 | +3 | +3 | -1 | -3 |

The Channeler modifier (`-2 / +3 / +3 / -1 / -3`) is the clearest mechanical
statement of the archetype: mental stats up, physical stats and CON down.

---

## 6. Trolloc "stocks" (homeland equivalents) and their flavor

Trollocs choose a **stock** (animal base-type) instead of a homeland. The five:

| Stock | Base stats (STR INT WIL DEX CON) | Player notes |
|-------|----------------------------------|--------------|
| **Beaked** | 15 9 7 14 15 | Highest INT; good combo/dodge of any class with good mentals (beaked-rogue now obsolete) |
| **Bearish** | 16 5 6 10 14 | Can reach 20–21 STR by class; historically problematic (CON/MV limits); pre-rolled 21 7 7 17 17 |
| **Boarish / Boarheaded** | 16 8 7 12 16 | "Perfect abs stock" — easy 19/19 for warriors and hunters |
| **Ramshorned** | 14 10 7 13 15 | Balanced; max 19 STR / 19 CON warriors; favors mentals + high base MVs; combo hunters |
| **Wolfish** | 15 9 7 14 16 | Darkside rogue option; favors DEX over mentals; combo/dodge of any class |

Per-stock max possible stats (warrior / rogue / hunter) and the D100 reroll
modifier tables are in `world-homelands-and-zones.md` §2.

---

## 7. Remort base stats

| Remort | Base stats (STR INT WIL DEX CON) |
|--------|----------------------------------|
| **Dreadlord** | 15 14 14 15 14 |
| **Myrddraal** | 16 14 12 14 16 |

---

## 8. Takeaways for AnotherMUD

- **Two playable races, faction-gated, with remort as the "elite race" path.**
  Seanchan was *collapsed into Human* — a reminder that fewer mechanical races +
  more clans/roles is a viable simplification.
- **Channeler is a class, not a race**, and is Human-only with a sharp stat
  signature (mentals up, physicals down). Maps cleanly onto our channeler class
  work (S2 One Power).
- **Hard language barrier by race** (Trolloc vs Human, remort-pierces-it) is a
  gameplay-meaningful comms gate — different from our current chat model.
- **Homeland/stock only affects rerolling now, not the starting character** — the
  game moved from roll-at-creation to pre-rolled bases. Relevant to how AnotherMUD
  thinks about character-creation stat assignment.
