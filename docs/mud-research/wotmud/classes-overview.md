# WoTMUD — Class & Skill System Overview

Source: https://wotmud.info/player-classes-in-wotmud/ (class overview) — the per-class skill content on wotmud.info is embedded from the WoTMUD Fandom wiki (`data-resource` points at `wotmud.fandom.com/wiki/...`), which is the authoritative mechanical source captured here: https://wotmud.fandom.com/wiki/Warrior, https://wotmud.fandom.com/wiki/Hunter, https://wotmud.fandom.com/wiki/Rogue, https://wotmud.fandom.com/wiki/Channeler, https://wotmud.fandom.com/wiki/Myrddraal
Fetched: 2026-06-18

---

## What classes are

> "Based on D&D rules like most other games of the MUD genre, player classes
> dictate which abilities and skills they will gain as they increase in level.
> The class you choose will help determine what your play-style will be."

- **Class choice can be constrained by race** — each race can pick from a
  different subset of classes.
- **Some classes (or class/race combos) are remort-only** — reached through the
  remort process, which is "the highest level of gameplay," so remorts offer
  greater flexibility than the base classes.
- Each class offers "its own unique balance of weaknesses and strengths."

There are **four base classes** (Warrior, Rogue, Hunter, Channeler) plus
**remort/advanced classes** (Dreadlords & Myrddraal). A meta-progression layer
called **Legend** sits on top.

## The base classes (verbatim blurbs)

| Class | Identity (from the overview) |
|-------|------------------------------|
| **Warrior** | "Hard hitters with major brute strength… the most dynamic class when it comes to weapons. You'll usually find them in heavy armor… able to go 'berserk', a skill that lends an offensive bonus and the ability to hit twice in a round, but makes them unable to flee while engaged." |
| **Rogue** | "Most comfortable in the shadows, these classic dagger-wielding sneaks are generally the most nimble. Their keen senses allow them to scan nearby indoor rooms… tend towards lighter armor… you could find yourself dead without even having seen one." |
| **Hunter** | "Widely considered the most versatile of the four basic classes… the jack-of-all-trades. Most easily adaptable to any type of armor… Gifted in the woods, they also have the ability to passively read tracks, giving them the potential to be excellent leaders." |
| **Channeler** | "One of the more difficult classes to learn, channelers wield the One Power. Able to do massive amounts of damage quickly, they are balanced by being somewhat of a glass cannon. Female channelers are easier to start with, as male channelers are hunted by most nations. Wilders, Aes Sedai, Dragonsworn and even Kin can be found in game." |

### Remort / advanced

> **Dreadlords & Myrddraal** — "advanced 'remort' classes, more powerful
> incarnations of trollocs and darkfriends that have been rewoven into the
> pattern by the Dark One. They are achieved through special quests that involve
> but are not limited to; deep game knowledge, leadership, strategy and warfare."

### Legend (meta-progression)

> "Challenges on a character never stop. Dedicated players can continue to
> progress even after they've reached Master status. Gain a greater number of
> questpoints to earn bonuses such as special gear, increased statistics, your
> own home and more."

---

## How the skill/prac economy works (the load-bearing system)

This is the single most important mechanic and it cuts across every class.

### 1. Skills are bought with practice sessions ("pracs")

Each class has its **own** skill set (warrior skills, hunter skills, rogue
skills, etc.). You raise a skill by spending **pracs** on it at a trainer mob.

### 2. Cross-class training costs more pracs (the "prac handicap")

Training a skill *outside* your class costs **more pracs per session** the
"further" the skill is from your class. This is the core balance lever — it
pushes each class toward its own playstyle and prevents everyone from learning
everything. The per-class prac cost matrices (collected from each class page):

| Skill set being trained | Warrior | Hunter | Rogue | M. Channeler | F. Channeler | Myrddraal |
|-------------------------|:-------:|:------:|:-----:|:------------:|:------------:|:---------:|
| **Warrior skills**      | 1 | 2 | 3 | 3 | 4 | 1 |
| **Hunter skills**       | 2 | 1 | 2 | 2 | 2 | 1 |
| **Rogue skills**        | 3 | 2 | 1 | 3 | 3 | 1 |
| **Channeler skills**    | — | — | — | 1 | 1 | — |

(Channeler skills are trainable only by channelers, from a separate prac pool —
see below. Myrddraal train all base-class skills at cost 1.)

### 3. Practice-percentage formulas differ per class (stat-driven %-gain)

How much skill % you gain per prac depends on a **class-specific stat formula**.
Each skill set has its own equation, captured on the per-class pages:

| Skill set | Practice-percentage formula |
|-----------|-----------------------------|
| Warrior   | `(Str / 2) + (Dex / 4) + (Con / 4)` % (each term rounded down) |
| Hunter    | `(Str + Int + Wil + Dex) / 4` % (rounded down) — the only *balanced* equation in the game |
| Rogue     | `(Dex * 3 / 4) + (Int / 4)` % (each term rounded down) |
| Channeler weaves | percentage per prac is "based only on your intelligence" |
| Channeler elements | "fixed and is not influenced by stats" |

The hunter's balanced equation is *why* the class is "jack-of-all-trades": its
%-gain doesn't punish any single stat, so a hunter with well-balanced stats can
reset pracs and re-train into an entirely different playstyle.

### 4. Channelers have TWO prac pools

> "Unlike the other classes, channelers have two prac pools. One is for normal
> skills, which increase at the same rate as a non-channeling character. The
> other pool is for channeling-related skills trainable only by this class."

The two pools advance in parallel but at different rates. (Full detail in
`weaves.md`.)

---

## Stat modifiers by class (verbatim)

Each base class applies fixed stat modifiers at creation:

| Class | Str | Int | Wil | Dex | Con |
|-------|:---:|:---:|:---:|:---:|:---:|
| **Warrior**  | +2 | 0 | -1 | -1 | +2 |
| **Hunter**   | 0 | 0 | 0 | 0 | 0 |
| **Rogue**    | -1 | 0 | 0 | +2 | 0 |
| **Channeler**| -2 | +3 | +3 | -2 | -3 |

The hunter's all-zero line is the mechanical root of its "versatile" identity.
The channeler trades away every physical stat (Str/Dex/Con) for mental ones
(Int/Wil) — the glass-cannon shape.

---

## Naming history

Two base classes were renamed in WoTMUD **v4.3 (February 8, 2002)**:
- **Ranger → Hunter**
- **Thief → Rogue**

The Fandom skill formulas still spell these as "Ranger/Hunter" and
"Thief/Rogue."

---

## Cross-references

- `warrior.md` — warrior class detail + full warrior skill list
- `hunter.md` — hunter class detail + full hunter skill list
- `rogue.md` — rogue class detail + full rogue skill list
- `myrddraal.md` — myrddraal remort class + skill list
- `weaves.md` — channeler class detail + the full in-game weave table
- `channeling.md` (existing) — research analysis of how WoTMUD translated the
  One Power onto a real-time MUD, for AnotherMUD S2 scoping
