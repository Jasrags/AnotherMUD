# WoTMUD — Progression & Experience

Source:
- https://wotmud.info/wheel-of-time-game-exp/
- https://wotmud.info/action-calculators-for-wotmud/

Fetched: 2026-06-18

**Sourcing caveat:** both source pages are JavaScript-gated — only the intro blurb is
server-rendered. The mechanical body never reaches a non-JS fetch. What rescues this doc
is the **"Art of War" forum sidebar** that the site embeds verbatim on several pages: real
players pasting live game output (score lines, condition prompts, rank strings). Those
pasted lines are **primary, verbatim game data** and are the most concrete progression
numbers on the whole site. They are captured below and flagged as forum-paste evidence.

---

## 1. How to Gain Experience [intro only]

> "Did you know? You can get killed by almost anything in the game at level 1. Even fudgy
> wudgy hedgehogs, so stick to saplings. They don't hit back!"

Takeaways from the intro framing:

- **Level 1 is lethal** — even trivial mobs can kill a fresh character. The game leans
  hostile from the start (a Diku tradition).
- The newbie progression path starts with **non-retaliating targets** ("saplings … they
  don't hit back") before fighting real mobs. There is a deliberate **safe-grind ramp**.
- EXP comes from **killing mobs** (the page is "Leveling 101"); smobbing (`crafting-and-survival.md`)
  and quests supplement it.

---

## 2. Action Calculators [intro only]

> "Bash, stab, melee, charge and kick calculators. Also some statistics on players.
> Designed by Thibaud to help you be on top of your game!"

This names the **five player-facing combat actions** WoTMUD models as discrete,
calculable rolls:

| Action | Notes |
|--------|-------|
| **Bash** | Shield/weapon bash — stun/knockdown opener |
| **Stab** | The Rogue dagger-assassination move (see the **"bricked"/"clumsied"** PK-glossary terms in `pvp-and-pk.md` — a failed stab "bricks" with a wasted timer) |
| **Melee** | The standard auto-attack swing |
| **Charge** | Mounted/closing attack |
| **Kick** | Secondary unarmed strike |

The existence of **per-action calculators** (and "statistics on players") confirms WoTMUD
exposes its combat math as **deterministic chance rolls** the player can pre-compute from
their stats vs. a target — skill % + stat modifiers feeding a hit/effect probability. This
is exactly the **tick/chance** model AnotherMUD's WoT EPIC Decision 0 chose to translate
onto (not a d20 rewrite).

---

## 3. Verbatim live game data (forum "Art of War" pastes)

These are real `score`/prompt lines players pasted into the forum, rendered by the site.
They are the most concrete numbers available.

### 3.1 The `score` line (a high-level character)

> "You have **112(408) hit, 142(142) dark power and 114(286) movement points**.
> You have scored **114098348 experience points** and **4412 quest points**.
> You **need 4401652 exp to level** and **1588 qp to rank**.
> You have amassed **348 Turn points** to date, ranking you **Reaver Fourth**.
> You have played **449 days and 7 hours** …"

What this exposes about the progression model:

- **Three resource pools**, each shown as `current(max)`:
  - **hit** (HP) — `112(408)`
  - **dark power** (DP) — `142(142)` — the **Dark-side channeler / Myrddraal resource**
    (the gendered One Power pool's dark-side label; see `channeling.md`).
  - **movement points** (MV) — `114(286)` — encumbrance/travel resource (mirrors our
    `internal/pool` movement + `movement-cost`).
- **Two parallel currencies of advancement:**
  - **experience points** → spent/needed **to level** (`need … exp to level`).
  - **quest points (qp)** → spent/needed **to rank** (`need … qp to rank`). **Ranking is a
    separate axis from leveling**, driven by quest points, not XP.
- **Turn points** — a *third* prestige currency (the "348 Turn points … Reaver Fourth"
  rank). Turn points also gate top-tier **crafting** (`crafting-and-survival.md`). The
  rank title ("Reaver Fourth") is a Turn-point ladder.
- **Played-time tracking** ("449 days and 7 hours") — long-horizon characters; this is a
  multi-year-character MUD.

### 3.2 Condition / prompt bands

Players' prompts show **named condition bands** rather than raw HP numbers — the standard
Diku descriptive-health ladder. Observed in the pastes:

- **HP bands (worst → best, partial):** `Critical` < `Wounded` < `Battered` < `Beaten` <
  `Hurt` < `Full` (and `Bleeding` as an active damage-over-time state — "You wish that
  your wounds would stop BLEEDING so much!").
- **DP (dark power) bands:** `Strong`, `Bursting` (e.g. `DP:Bursting`).
- **MV (movement) bands:** `Winded` < `Tiring` < `Strong` < `Full`.

The prompt format players run is roughly:
`* R S HP:<band> DP:<band> MV:<band> - <Target>: <band> >`
(flags `R`/`S` = riding / sneaking-or-stab-ready state; trailing target-condition for the
current opponent).

**For AnotherMUD:**
- The **descriptive-band prompt** (Critical/Wounded/…/Full) matches our `render` prompt
  philosophy — bands, not raw numbers, for the opponent.
- The **dual XP-to-level / qp-to-rank** split is a clean way to separate *power* (level)
  from *prestige/status* (rank), with a **third PK/trade prestige currency (turn points)**
  on top. Our progression has tracks + alignment but no equivalent prestige-rank ladder;
  this is the model to study if we want PK/quest prestige distinct from level.
- **Dark power** as a first-class pool (separate from HP/MV) confirms the channeler/Shadow
  resource sits beside vitals exactly like our `internal/pool` One-Power pool.

---

## Cross-references

- The five action calculators (bash/stab/melee/charge/kick) ↔ `pvp-and-pk.md` (stab/brick
  terms) and our `combat` package.
- Turn points ↔ `crafting-and-survival.md` (turn-point-token crafting tier).
- DP pool / channeling ↔ existing `channeling.md`.
