# AwakeMUD — Flags Reference

Source:
- https://github.com/luciensadi/AwakeMUD/wiki/Affect-Flags
- https://github.com/luciensadi/AwakeMUD/wiki/Room-Flags-(redit)
- https://github.com/luciensadi/AwakeMUD/wiki/Item-Extra-Flags
- https://github.com/luciensadi/AwakeMUD/wiki/Mob-Flags
Fetched: 2026-06-18

---

This file consolidates AwakeMUD's flag systems. **Affect flags** are listed in full
below; room/item/mob flag tables live in their respective editor files
(`room-and-zone-editing.md`, `item-and-mob-editing.md`) and are cross-indexed at the
bottom.

A recurring convention: many affect flags are marked **"do not set"** — they are
applied/managed by the engine's combat, magic, conjuring, drug, healing, or astral code
and are not meant for builders.

---

## Affect flags

Affect flags apply ongoing effects/states to wearers, wielders, or NPCs. Set via item
**affection flags** (`iedit` field 7) or mob **affected flags** (`medit` field 6).

| Flag | Description |
|---|---|
| `Acid` | Applied by acid-based spells; **do not set**. |
| `ACTION` | Used by the combat code; **do not set**. |
| `APPROACH` | Used by the combat code; **do not set**. |
| `Banishing` | Used by the banish code; **do not set**. |
| `Binding` | Binds a character to the ground, unable to move without first passing a check. |
| `Bonding Focus` | Used by the bonding code; **do not set**. |
| `Building Lodge` | Used by the lodge-construction code; **do not set**. |
| `Conjuring` | Used by the conjuring code; **do not set**. |
| `COUNTERATTACK` | Used by the fight code; **do not set**. |
| `Damaged` | Used by the damage and healing code; **do not set**. |
| `Designing` | Used by the programming code; **do not set**. |
| `Det-invis` | Gives a +4 TN modifier instead of the usual +8 when fighting invisible enemies. |
| `Detox` | Used by the Detox spell to remove drugs; **do not set**. |
| `Drawing Circle` | Used by the circle-creation code; **do not set**. |
| `Charm` | Mimics the standard fantasy "Charm" spell. |
| `Fear` | Causes the NPC to flee in fear until a test is passed. |
| `Group` | Used by the group code; **do not set**. |
| `Healed` | Marks the character as having been healed already. |
| `Hide` | Hides the affected NPC from sight until the hide is broken. |
| `Infravision` | Sets the NPC's vision to Thermographic. |
| `ImpInvis (Spell)` | Makes the NPC invisible as per Improved Invisibility. |
| `Invis (Spell)` | Makes the NPC invisible as per Invisibility. |
| `Laser-sight` | Gives the NPC the benefits of a laser-sighted weapon. |
| `LL-eyes` | Sets the NPC's vision to Low-Light. |
| `Manifest` | Used by the astral projection code; **do not set**. |
| `Manning` | Used by the turret-manning code; **do not set**. |
| `NOTHING` | Legacy flag; does nothing. |
| `Packing Workshop` | Used by the workshop-packing code; **do not set**. |
| `Part Building` | Used by the deck construction code; **do not set**. |
| `Part Designing` | Used by the deck construction code; **do not set**. |
| `Petrify` | Locks the NPC/character down completely, blocking all commands. |
| `Pilot` | Used by the code to designate a driver; **do not set**. |
| `Programming` | Used by the code to designate a programmer; **do not set**. |
| `Prone` | Designates whether a character is in the prone position. |
| `ResistPain` | Marks a character as affected by pain resistance. |
| `Rigger` | Used by the code to mark an active rigger; **do not set**. |
| `Ruthenium` | Makes the character invisible as if covered in Ruthenium. |
| `Sneak` | Flags the character as sneaking. |
| `Spell Design` | Used by the spell-design code; **do not set**. |
| `Stabilize` | Used by the healing and damage code; **do not set**. |
| `Surprised` | Used by the combat code; **do not set**. |
| `Thermoptic` | Makes the character invisible as if covered in thermoptic material. |
| `Tracked` | Used in the astral tracking code; **do not set**. |
| `Tracking` | Used in the astral tracking code; **do not set**. |
| `Vision x1` | Marks the character as having 1x zoom vision. |
| `Vision x2` | Marks the character as having 2x zoom vision. |
| `Vision x3` | Marks the character as having 3x zoom vision. |
| `Withdrawl` | Used by the drug code; **do not set**. |
| `Withdrawl (Forced)` | Used by the drug code; **do not set**. |

---

## Cross-index to other flag tables

- **Room flags** (`redit` field 4) — full table in `room-and-zone-editing.md`. Notable:
  `!GRID`, `!MOB`, `GARAGE`, `INDOORS`, `PEACEFUL`, `SOUNDPROOF`, `STORAGE`,
  `STAFF_ONLY`, `STREETLIGHTS`, `TUNNEL`, `ARENA`, `FALL`, `ROAD`, `!WALK`.
- **Item extra flags** (`iedit` field 6) — full table in `item-and-mob-editing.md`.
  Notable: `GLOW`, `HUM`, `!RENT`, `INVISIBLE`, `MAGIC`, `!DROP`, `FORMFIT`, `!SELL`,
  `CORPSE`, `GODONLY`, `TWOHANDS`, `SNIPER`, the `NO<RACE>` family.
- **Mob flags** (`medit` field 5) — full table in `item-and-mob-editing.md`. Notable:
  `AGGR` + `AGGR_<RACE>` family, `ASTRAL`, `DUAL`, `GUARD`, `HELPER`, `MEMORY`,
  `SCAVENGER`, `SENTINEL`, `SNIPER`, `SPEC`, `STAY-ZONE`, `WIMPY`, `ISNPC` (always set).
- **Door flags** (`redit` exit submenu): Standard door, Pickproof, Warded, Glass Window,
  Barred Window, No Shooting Through, Strict Key Requirement.

### Race groupings (from the AGGR / NO-race flags)

| Base race | Metavariants grouped with it |
|---|---|
| Dwarf | Gnome, Menehune, Koborokuru |
| Elf | Wakyambi, Night-One, Dryad |
| Human | (Human) |
| Ork | Hobgoblin, Ogre, Satyr, Oni |
| Troll | Cyclops, Fomori, Giant, Minotaur |
