# NukeFire — Item Database Schema & Samples

Source: https://nukefire.org/api/items/csv (live, publicly downloadable)
Fetched: 2026-06-18

Full CSV is downloadable at the URL above. This file captures the schema and sample items.

---

## CSV Schema

```
vnum, player_name, item_type, short_description, abilities, worn_locations,
extra_flags, valid_classes, weight, retail_value, rent, min_remorts,
ac_apply, damage_dice, average_damage, affects, timestamp
```

### Field Notes

- **vnum**: unique item ID (virtual number, Circle lineage term)
- **player_name**: who identified/found the item (last identifier)
- **item_type**: WEAPON, ARMOR, WORN, IMPLANT, TATTOO, KEY
- **abilities**: special active abilities on the item (e.g. DAMCON, DET-INVIS, CATSEYE)
- **worn_locations**: where it can be equipped (space-separated flags)
- **extra_flags**: SOCKET_OBJECT, VARIANT, GLOW, HUM, CONCEALABLE, SHOP_BOUGHT
- **valid_classes**: "All Classes" or space-separated class names
- **min_remorts**: minimum total remorts to use item
- **ac_apply**: flat AC value (armor items)
- **damage_dice**: XdY format (weapons)
- **average_damage**: calculated average damage (weapons)
- **affects**: comma-separated stat bonuses

---

## Worn Location Reference

### Standard Armor Slots
HEAD, FACE, JAW, SHOULDER, BODY, LEGS, WAIST, FINGER, WRIST, ACCESSORY, OVERSHIELD

### Weapon Slots
WIELD, DUAL_WIELD

### Implant Slots (IMP_*)
IMP_FINGER, IMP_BACK, IMP_CHEST, IMP_SKULL, IMP_JAW
*(likely more: IMP_ARM, IMP_LEG, IMP_EYE, etc.)*

### Tattoo Slots (TAT_*)
TAT_FACE, TAT_JAW, TAT_FINGER, TAT_FOOT, TAT_WRIST, TAT_CHEST, TAT_ARM
*(likely more)*

---

## Stat Applies Reference

| Stat | Description |
|---|---|
| DAMROLL | Flat bonus to every melee attack damage |
| HITROLL | Accuracy bonus |
| FIGHTSPEED | Combat speed modifier (negative = faster) |
| DAM_REDUCTION | Adds to DR random reduction pool |
| SPIKE_GUARD | Reflects damage back to attacker |
| DODGEROLL | Avoidance bonus |
| BULLET_IMPACT_DAMAGE | Flat bonus added to every gun shot |
| MAXHIT | Maximum HP increase |
| MAXMANA | Maximum mana increase |
| MAXMOVE | Maximum movement increase |
| ARMOR | AC bonus (negative = more protection) |
| HIT_REGEN | HP regeneration rate |
| MANA_REGEN | Mana regeneration rate |
| MOVE_REGEN | Move regeneration rate |
| SPELLPOWER | Spell damage/effectiveness multiplier |
| GOLD | Bonus gold (carrying capacity or drop bonus) |
| LEECH | Life drain on hit |
| STR | Strength stat bonus |

---

## Sample Items (from live CSV, June 18 2026)

### Weapons

**Luka's Spear** (vnum 60605)
- Type: WEAPON | Slots: WIELD, DUAL_WIELD
- Damage: 44d41 (avg 924) | Min remorts: 20
- Flags: SOCKET_OBJECT
- Affects: SPIKE_GUARD +3, FIGHTSPEED -3, DAMROLL +45, SPELLPOWER +40, MAXHIT +5000, MAXMOVE +5000

**a midnight stick-hook** (vnum 33972)
- Type: WEAPON | Slots: WIELD, DUAL_WIELD
- Damage: 33d33 (avg 561) | Min remorts: 5
- Flags: CONCEALABLE, SOCKET_OBJECT
- Valid Classes: NINJA, GYPSY, INFILTRATOR, FANATIC, OCCULTIST (class-restricted)
- Affects: HITROLL +23, DAMROLL +23, SPELLPOWER +40

**an old handsaw** (vnum 4662)
- Type: WEAPON | Slots: WIELD, DUAL_WIELD
- Damage: 19d8 (avg 85.5) | Min remorts: 0
- Affects: DAMROLL +5, HITROLL +6, FIGHTSPEED -2, DAM_REDUCTION +5, GOLD +30, MAXMOVE +100

**a red-painted chain** (vnum 10807)
- Type: WEAPON | Min remorts: 0
- Damage: 15d13 (avg 105)
- Flags: HUM
- Affects: DAMROLL +3, MAXHIT +60, DAM_REDUCTION +4, ARMOR -10, HIT_REGEN +30, FIGHTSPEED -1

### Armor

**✪ MYTHIC ✪ pair of Festival Lederhosen** (vnum 42036)
- Type: ARMOR | Slot: LEGS | Min remorts: 60
- Flags: VARIANT, SOCKET_OBJECT
- AC: 1000 | Retail: 5,000,000 gold
- Affects: MAXHIT +7035, DAMROLL +124, FIGHTSPEED -8, SPELLPOWER +135, BULLET_IMPACT_DAMAGE +1000, HITROLL +157

**Palegate Bulwark** (vnum 64200)
- Type: ARMOR | Slot: OVERSHIELD | Min remorts: 5
- Valid Classes: FANATIC, OCCULTIST
- AC: 125 | Retail: 6,900,000 gold
- Flags: SOCKET_OBJECT
- Affects: SPELLPOWER +55, MAXHIT +7500, GOLD +500

**psalm panel** (vnum 39114)
- Type: ARMOR | Slot: SHOULDER | Min remorts: 10
- Valid Classes: SLINGER, CURIST, KAIJU, HERETIC, GYPSY, VOIDSTRIKER, INFILTRATOR, FANATIC, OCCULTIST
- AC: 288 | Retail: 2,000,000 gold
- Flags: GLOW, HUM, SOCKET_OBJECT
- Affects: MAXMANA +2500, SPELLPOWER +30, DAMROLL +50, GOLD +300, LEECH +500, FIGHTSPEED +3

**a set of gleaming blue scale mail armor** (vnum 31107)
- Type: ARMOR | Slot: BODY | Min remorts: 21
- AC: 10 | Retail: 1,000 gold
- Affects: DAMROLL +4, HITROLL -2, STR +2

### Implants

**✪ MYTHIC ✪ Stinger Tail Graft** (vnum 42089)
- Type: IMPLANT | Slot: IMP_BACK | Min remorts: 0
- Flags: VARIANT, SOCKET_OBJECT | Retail: 9,000,000 gold
- Affects: MAXHIT +11211, DAMROLL +243, ARMOR -1997, SPELLPOWER +138, DAM_REDUCTION +200, GOLD +500

**Drowned-Heart Regulator** (vnum 60876)
- Type: IMPLANT | Slot: IMP_CHEST | Min remorts: 2
- Abilities: DAMCON
- Flags: VARIANT, SOCKET_OBJECT | Retail: 800,000 gold
- AC: 120
- Affects: DAMROLL +56, MAXHIT +2910, BULLET_IMPACT_DAMAGE +758, SPIKE_GUARD +3, GOLD +803, DAM_REDUCTION +69

**Volatile Cranial Undershell** (vnum 60977)
- Type: IMPLANT | Slot: IMP_SKULL | Min remorts: 2
- Flags: VARIANT, SOCKET_OBJECT | Retail: 680,000 gold
- AC: 120
- Affects: DAMROLL +25, MAXHIT +2028, SPELLPOWER +22, SPIKE_GUARD +1, GOLD +400, DAM_REDUCTION +48

**a chrome fang implant** (vnum 10845)
- Type: IMPLANT | Slot: IMP_FINGER | Min remorts: 0
- Retail: 50,000 gold
- Affects: DAMROLL +3, MAXHIT +50, MAXMOVE +50, DAM_REDUCTION +5, DODGEROLL +5, FIGHTSPEED -1

**Ebony Finger Implant** (vnum 64209)
- Type: IMPLANT | Slot: IMP_FINGER | Min remorts: 5
- Retail: 1,800,000 gold
- No affects listed (base implant, likely socketed)

### Tattoos

**a redline_oath tattoo** (vnum 10860)
- Type: TATTOO | Slots: TAT_FINGER, TAT_FOOT, TAT_WRIST | Min remorts: 0
- Retail: 45,000 gold
- Affects: SPELLPOWER +3, MAXMANA +50, MAXHIT +50, MANA_REGEN +50, HIT_REGEN +30, ARMOR -15

**Living Stitches (warped)** (vnum 60966)
- Type: TATTOO | Slots: TAT_FACE, TAT_JAW | Min remorts: 2
- Flags: VARIANT
- AC: 120 | Retail: 900,000 gold
- Affects: DAMROLL +25, SPELLPOWER +29, MAXHIT +3000, SPIKE_GUARD +1, DAM_REDUCTION +40, DODGEROLL +40

### Worn (non-armor)

**an old Riff signet ring** (vnum 10873)
- Type: WORN | Slot: FINGER | Min remorts: 0
- Flags: HUM | Retail: 100,000 gold
- Affects: SPELLPOWER +3, MAXMANA +50, MAXMOVE +50, MANA_REGEN +40, MOVE_REGEN +40, ARMOR -25

**smoked welding goggles** (vnum 4621)
- Type: WORN | Slot: FACE | Min remorts: 0
- Abilities: DET-INVIS, CATSEYE
- Flags: HUM, SOCKET_OBJECT | Retail: 21,000 gold
- AC: 5
- Affects: MAXMOVE +100, MAXHIT +100, HITROLL +4, ARMOR -37, DAMROLL +2, DAM_REDUCTION +5

**a painted alley-crown cap** (vnum 10812)
- Type: WORN | Slot: HEAD | Min remorts: 0
- Retail: 30,000 gold
- Affects: DAMROLL +2, MAXHIT +50, MAXMOVE +50, HIT_REGEN +30, MOVE_REGEN +30, ARMOR -15

---

## Design Observations

1. **Three parallel gear tracks**: standard armor, implants, and tattoos all occupy separate slot spaces — a character can have all three simultaneously
2. **Implants as core character build** — IMP_CHEST, IMP_SKULL, IMP_BACK, IMP_FINGER, IMP_JAW all visible; implants carry as much stat weight as armor
3. **Tattoos are a third layer** with their own slot namespace (TAT_FACE, TAT_JAW, etc.)
4. **SPIKE_GUARD** — a retaliatory damage stat that appears across multiple item types; reflects damage to attackers
5. **BULLET_IMPACT_DAMAGE** — a gun-specific stat that appears even on implants (Drowned-Heart Regulator), showing gun builds are supported via non-weapon items
6. **LEECH** — life drain on hit; appears on armor (psalm panel at LEECH +500)
7. **Class-restricted items** exist (psalm panel, Palegate Bulwark, midnight stick-hook)
8. **VARIANT flag** = crafted items from the instability system; can be warped/unstable/perfected
9. **Min remorts** gate access — creates a natural progression ladder beyond just level
10. **CONCEALABLE** flag on weapons — hidden weapons have gameplay implications
