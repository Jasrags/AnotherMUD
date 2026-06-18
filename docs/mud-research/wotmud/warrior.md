# WoTMUD — Warrior Class & Skills

Source: https://wotmud.info/warrior/ and https://wotmud.info/warrior-skills/ (both embed the Fandom wiki via `data-resource`; authoritative source: https://wotmud.fandom.com/wiki/Warrior and https://wotmud.fandom.com/wiki/Warrior_skill)
Fetched: 2026-06-18

---

## Class identity

> "Warriors are the heavy-hitting, overtly offensive characters in game. They
> receive stat bonuses to strength and constitution, two characteristics which
> favor bashing, charging and abs playstyles. They also receive a minus to
> dexterity which tends to prevent them from being dodgy or stabby, though dodgy
> warriors are not unheard of and a very few warriors have been seen to stab."

The prac-cost structure (warrior skills = 1 prac, hunter = 2, rogue = 3) pushes
warriors toward **bash / charge / kick** and away from hide / sneak / backstab.

## Stat modifiers

| Str | Int | Wil | Dex | Con |
|:---:|:---:|:---:|:---:|:---:|
| +2 | 0 | -1 | -1 | +2 |

## Practice costs (pracs per training session)

| Warrior skills | Hunter skills | Rogue skills |
|:---:|:---:|:---:|
| 1 | 2 | 3 |

## Class abilities (passive / unique)

| Ability | Effect |
|---------|--------|
| **Increased Offensive Bonus** | Innate **+5 offensive bonus**. |
| **Increased Damage vs Humanoids** | **+50% damage** vs Humanoid mobs (includes darkside humanoids — trollocs, fades, etc.). |
| **Reduced Damage Taken** | **1–2 less damage per hit** taken. |
| **Berserk Attack (REMOVED)** | Formerly: while in Berserk mood, could Attack with *any* weapon type, up to 2 hits per round based on Level + Attack proficiency. Now removed. |

## Practice-percentage formula

`(Str / 2) + (Dex / 4) + (Con / 4)` % — each term rounded down.

---

## Warrior skills

### Combat maneuvers

| Skill | Effect |
|-------|--------|
| **Bash** | Knocks opponent off their feet for **1–2 rounds**, preventing them from attacking while granting everyone engaging that opponent a chance at critical damage. While downed, target's **DB reduced ~75%** and **PB ~25%**. Has a **13-pulse execution timer**. |
| **Kick** | Slight damage; **interrupts any weaves, charges or bashes** being cast/readied; reduces the target's current movement points. Causes ~**1.25 rounds of intentional self-lag** to the kicker — which makes stopping sequential bashes with kick very difficult and lowers your own offensive melee power. |
| **Rescue** | Rescues an engaged player, shifting the attacker's focus from the rescued to the rescuer. |
| **Shield parry** | Increases the usefulness of shields (can drastically raise parry bonus). The applied bonus is **dependent on Dexterity**. |
| **Charge** | Like a bash but a huge strike of its own instead of a knockdown. **Only usable while mounted with a spear or lance equipped**, and only begun on an **unengaged** opponent. Base **7-pulse execution timer** (fleelag can add pulses). |

### Weapon proficiencies

Each weapon skill raises offensive + parry bonus when that weapon type is
equipped, and grants **slight residual proficiency** in related weapon classes:

| Weapon skill | Residual proficiency granted |
|--------------|------------------------------|
| **Long blades** | medium blades, fencing blades |
| **Medium blades** | long blades, fencing blades |
| **Fencing blades** | long blades, medium blades |
| **Axes** | clubs, staves (all other concussion weapons). Some axes usable as Projectiles (require Projectile skill). |
| **Clubs** | axes, staves |
| **Staves** | axes, clubs |
| **Lances** | spears, javelins, polearms (other shafted weapons) |
| **Spears** | lances, javelins, polearms. Some spears usable as Projectiles without the Projectile skill. |
| **Javelins** | lances, spears, polearms. Javelins usable as Projectiles without the Projectile skill. |
| **Polearms** | lances, spears, javelins |
| **Flails** | whips, chains (other flexible weapons) |
| **Whips** | flails, chains |
| **Chains** | flails, whips |
| **Bows** | "Not fully implemented." Residual to crossbows, slings. |
| **Crossbows** | "Not fully implemented." Residual to bows, slings. |
| **Slings** | "Not fully implemented." Residual to bows, crossbows. |

## Prac costs for warrior skills, by class

| Warrior | Hunter | Rogue | M. Channeler | F. Channeler | Myrddraal |
|:---:|:---:|:---:|:---:|:---:|:---:|
| 1 | 2 | 3 | 3 | 4 | 1 |
