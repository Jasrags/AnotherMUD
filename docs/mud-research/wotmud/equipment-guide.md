# WoTMUD — Equipment Guide

Source:
- https://wotmud.info/wotmud-equipment-guide/
- Equipment spreadsheet: https://docs.google.com/spreadsheets/d/e/2PACX-1vSezB0p0kcJmkTarxCJAdP5wKCuI5sOAg32IP0OAEtMwVUjB4Rx-K1_SoBxSUvko-4tG88oSPxzpYES/pubhtml
- Backing mechanics: https://wotmud.fandom.com/wiki/Armor and https://wotmud.fandom.com/wiki/Weapon

Fetched: 2026-06-18

> Sourcing note: the wotmud.info equipment page is a thin wrapper around a
> **player-maintained Google Sheet** ("Wotmud Equipment") embedded in an iframe;
> the sheet's cells are JS-rendered and not directly fetchable. The mechanical
> model below comes from the WoTMUD Fandom wiki (Armor / Weapon) via the
> MediaWiki API. The page itself says: "A player maintained spreadsheet to help
> you compare stats so that you can build the best setup for your character. Plug
> these numbers into the action calculators to see how well you'll face against
> your foes!"

---

## 1. Armor

**Armor** = items equipped for a defensive bonus. Each piece can boost some mix of:

| Attribute | Abbrev | Effect |
|-----------|--------|--------|
| **Dodge bonus** | DB | Chance to dodge an attack entirely (no damage). |
| **Parry bonus** | PB | Chance to parry; **PB is split when fighting more than one opponent** (DB is not). |
| **Absorption** | Abs | % of incoming damage absorbed. On-screen value is a **weighted average** of each piece's raw abs × that body part's chance-to-be-hit. |

High-Abs pieces "often have negative DB and/or PB" — the core tradeoff. Armor also
has **weight** (drives **encumbrance**) and a **±MV** impact (movement points).

### Armor playstyles (build archetypes)
Armor is classified as **Dodge, Combo, Parry, or Abs**, "though many pieces blur
these lines." These are the canonical build archetypes referenced throughout
character planning (e.g. "abs char", "combo hunter", "dodge char").

### Armor slots (7)
Six main body slots — each can modify DB / PB / Abs / MV:
**Head · Chest · Arms · Hands · Legs · Feet**
Plus a seventh slot — **Shields** — which modifies **PB only** (and can negatively
impact DB).

### Durability & mending (armor)
Armor starts **pristine**; the **Abs %** degrades as it takes hits. **Abs armor
takes the most hits and wears out fastest.** Mend at an **armorsmith**:
`mend <armor>` while standing in the smith's room. The worse the condition, the
more frequently it must be re-mended to restore full Abs %.

---

## 2. Weapons

A **weapon** is a wielded item used to inflict damage. Weapons may be
**one-handed** (shield allowed), **two-handed** (no shield), or **either**.

### Weapon classes ("broad groups")
Each class has 3–4 **weapon groups**. **Skill bleeds across groups in the same
family** — learning short blades gives limited skill in long blades, etc. So a
non-warrior can be good in two weapon types, and a warrior can master many.

| Broad group | Weapon groups |
|-------------|---------------|
| **Blades** | Short blades · Long blades · Medium blades · Fencing blades |
| **Concussion** | Axes · Clubs · Staves |
| **Poles** | Lances · Spears · Javelins · Polearms |
| **Flex** | Flails · Whips · Chains |
| **Range** | Projectiles · Bows · Crossbows · Slings |

Special abilities tie to specific weapon classes: **Attack, Backstab, Charge,
Throw**. All weapons (and shields) can be used with **Bash**, "though not all to
the same effect." Weapons can be **sheathed** (the class determines which sheath
holds it; some armor/trinket items also sheath).

### Honing & hardening (weapon upgrades)
A **Master Blacksmith** (in various cities) can improve a weapon in two steps:

| Step | Effect | Cost |
|------|--------|------|
| **Honing** | Additive **2d4 damage** bonus + ability to hit **spirit-type mobs** | 75 gold crowns |
| **Hardening** | +5 to the weapon's base **Offensive bonus (OB)** | +150 gold crowns |

### Durability & mending (weapons)
A weapon starts pristine; its **damage output degrades** with use. Repair at a
**weaponsmith**, or with **an oilstone** / **a piece of sandstone** (depending on
weapon type). Stone syntax while dismounted: `remove <weapon>` → `hold stone` →
`mend <weapon>` (in a weaponsmith's room he takes it and mends it for a fee
instead). Worse condition → more frequent mending.

### Master weapons
A distinct category ("old master weapons vs new master retooling") — the
top-tier/named weapons, tracked separately on the wiki.

---

## 3. The combat-stat vocabulary (what the spreadsheet compares)

The equipment spreadsheet exists to let players compare and total these per-setup
stats, then feed them into the **action calculators**:

| Stat | Meaning |
|------|---------|
| **OB** | Offensive bonus (to-hit / attack power) |
| **DB** | Dodge bonus |
| **PB** | Parry bonus (split across multiple foes) |
| **Abs** | Absorption % (weighted by hit location) |
| **MV** | Movement points (encumbrance-relevant) |
| **Weight / Encumbrance** | Drives MV cost + carry limits |

---

## 4. Takeaways for AnotherMUD

- **Three orthogonal defensive axes — Dodge / Parry / Absorption — with explicit
  tradeoffs** (high Abs costs DB/PB) and a "PB splits across multiple foes" rule.
  This is a richer defense model than a single AC and aligns with our armor-depth
  work (DB/PB/Abs vs our `ac` + Dex-cap composition); the *split-on-multi-target*
  parry rule is a notable mechanic we don't have.
- **Seven equipment slots** (Head/Chest/Arms/Hands/Legs/Feet + Shield), with
  shields as PB-only. Maps onto our `slot` package and equipment-slots milestone
  (M25).
- **Weapon families with skill bleed-across-groups** mirrors our proficiency model
  but adds a "learning one group partially trains its family" wrinkle worth
  considering for skill-gain.
- **Honing/hardening = a two-step paid upgrade path** ( +2d4 dmg + spirit-hit, then
  +5 OB ) — comparable to our masterwork/grade system, but as a *service you buy*
  rather than a crafting output. The "honing lets you hit spirit-type mobs" gate is
  a clean damage-type unlock pattern.
- **Durability that degrades and is restored by mending at a smith (or with a
  stone)** — armor Abs and weapon damage both wear; Abs armor wears fastest. A
  concrete upkeep/economy loop we could mirror in crafting/economy.
