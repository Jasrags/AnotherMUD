# WoTMUD — Rogue Class & Skills

Source: https://wotmud.info/rogue-class-in-wheel-of-time-online-game/ and https://wotmud.info/rogue-skills/ (both embed the Fandom wiki via `data-resource`; authoritative source: https://wotmud.fandom.com/wiki/Rogue and https://wotmud.fandom.com/wiki/Rogue_skill)
Fetched: 2026-06-18

---

## Class identity

> "Rogues are the sneaky, slightly villainous characters of the world and this
> is heavily reflected in their skill sets. The skills required to burgle,
> ambush, assassinate, bypass security, etc. are all in the rogue's domain.
> This class is unique in that a rogue who is content with sticking strictly to
> class-related gameplay can spend **every prac at his own trainer**, thereby
> achieving a uniquely high level of proficiency in a wide range of skills,
> whereas a hunter or warrior must spend at least a few pracs at other trainers
> despite the out-of-class penalties."

The rogue is "the only class which includes both **offensive and defensive
skills** (short blades and dodge) in addition to their own class-essential
skills," so a rogue can become a master pickpocket / burglar / ambusher without
ever paying out-of-class prac penalties.

> Formerly known as **Thieves**; renamed during the **v4.3 update on February 8,
> 2002**.

## Stat modifiers

| Str | Int | Wil | Dex | Con |
|:---:|:---:|:---:|:---:|:---:|
| -1 | 0 | 0 | +2 | 0 |

## Practice costs (pracs per training session)

| Warrior skills | Hunter skills | Rogue skills |
|:---:|:---:|:---:|
| 3 | 2 | 1 |

## Class abilities (passive / unique)

| Ability | Effect |
|---------|--------|
| **Autoscan** | Automatically scan the surrounding rooms (rather than manually typing `scan`). |
| **Inventory Peek** | See some/all items in a player or mob's inventory — useful with the **Steal** skill. |
| **Free Sneak** | Unlike other classes, rogues suffer **no movement-point penalty** when Sneak is on — a useful bonus for backstabbing. |

## Practice-percentage formula

`(Dex * 3 / 4) + (Int / 4)` % — each term rounded down.

---

## Rogue skills

| Skill | Effect |
|-------|--------|
| **Steal** | Steal an item from a mob/player inventory. Rogues can "peek" to identify desirable items. Success depends on relative level (?), target-item weight, and steal skill. |
| **Pick** | Pick locks on doors and chests — enter key-locked or faction-restricted rooms. Some locks cannot be picked. |
| **Hide** | Hide your presence from others entering (or re-looking). Backstab and ambush rely heavily on it. |
| **Sneak** | Prevent others observing your **entry** to a room; reduces auto-aggro chance from mobs. With **hide**, avoid detection entirely until you leave an occupied room. Costs extra movement for all classes **except rogues**. Only usable unmounted. |
| **Dodge** (rogue skill) | The art of evading attacks entirely. Drastically raises dodge bonus. High dodge + parry can make a character almost un-bashable and almost impossible to hit. Heavily dependent on Dexterity, plus carried/worn weight, level, and equipped item types. |
| **Attack** | Lets a player with most **piercing** weapons occasionally attack twice per combat round — to make up for a rogue's weak melee skills. Has since been expanded (also granted to warriors in berserk mood, regardless of weapon class). |
| **Backstab** | The primary rogue combat skill — typical entry to combat, surest way to finish a crippled enemy. **Stab and ambush damage bypass armor effects**, making them dangerous to high-abs players. Success depends heavily on backstab/hide/sneak training, Dexterity (?), and stabber-vs-stabbee level difference. Damage depends on Dexterity (?), weapon equipped, and stabber's level (?). |
| ↳ **Ambush** | (sub-skill of Backstab; wiki blurb is a stub) |
| **Palm** | Take items from a room without anyone noticing; draw a weapon from a sheath without a room emote. Success depends on Dexterity, palm training, item weight, character level. Mild effect on Projectiles usage. |
| **Short blades** | Raise offensive + parry bonus with short blades. Grants **residual practices** to long, medium, and fencing blades. |
| **Projectiles** | Raise offensive bonus with projectile weapons; lets you **throw** equipped items (spears, rocks, throwing knives, etc.) — some high-damage (spears, knives), some low-damage with interesting effects. Implemented; gives residual practices to bow/crossbow/sling (not yet fully implemented). One of the only ways to deal with flying adversaries (ravens, crows). |
| ↳ **Throw** | (sub-skill of Projectiles; wiki blurb is a stub) |

## Prac costs for rogue skills, by class

| Warrior | Hunter | Rogue | M. Channeler | F. Channeler | Myrddraal |
|:---:|:---:|:---:|:---:|:---:|:---:|
| 3 | 2 | 1 | 3 | 3 | 1 |
