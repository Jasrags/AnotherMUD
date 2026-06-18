# WoTMUD — Horses, Mounts, and Riding

Source:
- https://wotmud.info/horses/
- Backing data: https://wotmud.fandom.com/wiki/Horse and https://wotmud.fandom.com/wiki/Ride

Fetched: 2026-06-18

> Sourcing note: wotmud.info/horses/ is JS-rendered and embeds
> `https://wotmud.fandom.com/wiki/Horse`. The mechanics below come from the wiki
> (Horse + Ride pages) via the MediaWiki API.

---

## 1. What horses are

**Horses are mobs** that non-Trolloc player characters can **lead** and **ride**.
Riding lets you "travel longer distances without depleting your own MVs" — the
mount spends movement instead of you.

Key properties:
- **Trollocs cannot ride** (non-Trolloc PCs only).
- Like **pets**, a led horse can be **Grouped**, granting an **Experience bonus by
  the horse's level** — **lower-level horses give the largest XP bonus.**
- Horses are found in **stables**, dedicated **Pet shops**, and **roaming the
  world** (where appropriate commands make them follow).
- **Buying at a Pet shop lets you name the horse** — e.g. `buy warhorse lightning`,
  then `ride lightning` / `lead lightning`. Naming eases **horse targeting** when
  several players share a generic mount (e.g. multiple "warhorse"s in one room).
- **Hunters get a +50% damage bonus versus horses** (anti-cavalry niche).
- Horses have a **Level**, **MVs** (movement pool), and a **Regen** rate; clan
  mounts additionally belong to a **Clan**.

---

## 2. Riding — the Ride skill

**Ride is a Hunter skill** that lets a character ride and lead mounts. It works in
**Levels that rise every 14 practice percentage points**. Higher Ride levels unlock
more than just movement.

Syntax:
- `ride <mount>` — mount (or `ride` alone → your most-recently-dismounted horse)
- `dismount`
- `lead <mount>` — lead (or `lead` alone → your most-recently-ridden horse)
- `order <mount> <command>`

Leading is **rolled against your Ride %** — higher level = higher chance to lead
successfully (may take several tries; tries needed decrease with level).

### Ride level ladder

| Ride Lvl | Prac% | OB¹ | DB² | Special effects |
|----------|-------|-----|-----|-----------------|
| Lvl 0 | 0–13 | n/a | n/a | Can ride, **cannot lead**; mounts sometimes refuse commands |
| Lvl 1 | 14–27 | +5 | -10 | Can **lead** mounts³ and **charge** |
| Lvl 2 | 28–41 | +6 | -9 | (unconfirmed) |
| Lvl 3 | 42–55 | +7 | -8 | Can **fight while mounted** |
| Lvl 4 | 56–69 | +8 | -7 | Can **track / autotrack while mounted**; once led, mounts no longer refuse to follow⁴ |
| Lvl 5 | 70–83 | +9 | -6 | **Automatically intercept attacks against the mount**⁵ |
| Lvl 6 | 84–97 | +10 | -5 | Can **bash while mounted** |
| Lvl 7 | 98–99 | +11 | -4 | Mostly for extra autotrack lines (hunters) + the OB/DB stats |

¹ OB is added before weight effects and rounding; subject to mood multipliers.
² DB malus is subtracted before weight effects/rounding; actual reduction may be 1
DB worse depending on weight.
³ Multiple tries may be needed to lead; average tries drop with higher Ride level.
⁴ The lead-refusal restrictions still apply at all levels.
⁵ Faceoff and max-engage rules may still apply.

The headline tradeoff at every level: **riding grants OB but costs DB** (you hit
harder mounted but are easier to hit), and the malus shrinks as Ride improves.

### In-game help text (verbatim)
> "This allows you to ride a horse, thereby increasing the speed and distance you
> can easily travel. You can dismount the horse with the Dismount command. While
> level 1 ride will allow you to ride a horse, higher levels are required for more
> skillful activities, such as combat."

---

## 3. Horse types & gear

- **Horse classes** range from generic mounts up to **warhorses** (the combat
  mount referenced in shop examples).
- **Clan horses** are a distinct category (clan-restricted mounts, each tied to a
  Clan).
- **Horse Eq** — horses have their own equipment (barding/tack); the wiki tracks a
  "Horse Eq" page alongside Horse targeting and Ride.

---

## 4. Takeaways for AnotherMUD (mounts.md)

This directly informs our in-progress **mounts** feature (`docs/specs/mounts.md`,
Slice 1 shipped):

- **Mount = a specialized mob you lead/ride**, exactly our "mount = specialized
  MobInstance, reuse not new type" decision. WoTMUD treats horses as mobs with a
  Level + MV pool + Regen.
- **Riding spends the mount's movement, not the rider's** — the core travel value,
  and a clean mapping onto our movement-points pool (the mount carries its own MV).
- **Ride is a leveled skill with a graduated unlock ladder** (lead → charge →
  mounted combat → mounted track → intercept → mounted bash), each level trading
  **+OB for −DB**. A strong model for tying mount capability to a skill rather than
  a flat on/off ride state. Our mount temperament/travel-pool surface is the seam
  this would plug into.
- **Naming a bought mount for easy targeting** (`ride lightning`) is a small UX win
  worth copying for keyword resolution when many generic mounts coexist.
- **Grouped mounts grant XP (more for low-level mounts)** and **Hunters get +50%
  damage vs horses** — faction/class-asymmetric mount interactions, if AnotherMUD
  ever wants anti-cavalry counterplay.
- **Clan mounts** (mount access gated by clan/faction) is a natural extension once
  faction clans exist.
