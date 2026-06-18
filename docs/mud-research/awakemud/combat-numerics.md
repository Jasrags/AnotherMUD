# AwakeMUD — Combat Numerics & Physical Combat

Source: https://awakemud.com/dokuwiki/doku.php?id=meatguide (Physical Combat);
https://awakemud.com/dokuwiki/doku.php?id=khai_numerics (Combat Numerics);
https://awakemud.com/dokuwiki/doku.php?id=mageslaying (The Most Dangerous Game / Mage Slaying)
Fetched: 2026-06-18

---

## Test Resolution (SR3 dice system)

- **Standard Target Number (TN): 4.**
- Success: any die rolled **≥ TN**.
- Automatic failure: any roll of **1**, regardless of TN.
- High TN (> 6): roll a 6, then roll again; success if the cumulative total
  exceeds the TN (re-roll mechanic).
- Rolling against TN 6 yields ~half the successes compared to TN 5.

Higher attribute/skill ratings = more dice = more successes and greater "degrees
of success." TN modifiers (smartlink lowers TN, poor conditions raise it) shift
difficulty.

## Combat Modifiers (affect both players and NPCs)

| Modifier | Effect |
|----------|--------|
| **Vision** | Darkness, mist, smoke, distance, invisibility raise TN. Thermographic penalty > astral; thermo applies only to attacks, astral to "everything." |
| **Pain / Injury** | Each damage level raises TN progressively. A Pain Editor negates this entirely. |
| **Cover** | Characters automatically use available room cover vs. ranged attacks. |
| **Prone** | Improves ranged attack + defense TNs; **reverses** if an enemy shares the room. |
| **Background Count** | Raises spellcasting TN; rises from spellcasting, mob kills, astral activity. |

### Background Count (BGC) details

- Physical combat sets the count to **1** in unpolluted areas.
- Each death temporarily increases the count by **+1**.
- High counts impose casting/drain penalties; strong NPCs refuse to use magic.
- Astral Perception detects the count; mundanes can infer it via blood present.

## Armor & Damage Resistance

- **Body** stat determines the dice rolled to resist (soak) damage.
- Armor directly reduces the TN of the damage-resistance test (rolling Body dice).
- Natural/magical abilities outperform equipment-based equivalents.

### Armor layering (worked example, Quickness 12)

The highest rating that does **not** get halved is the piece (or combined set
total) with the highest Ballistic + Impact.

| Slot | Rating |
|------|--------|
| About (set piece) | 3.5 B / 3.5 I |
| Body (underneath) | 5 B / 4 I |
| Form-fitting | 5 B / 3 I |
| Legs (set piece) | 1.5 B / 1.5 I |
| Feet (set piece) | 1 B / 1 I |
| Shield (set piece) | 1 B / 1 I |
| **Total (layered)** | **12 B / 10 I** |

## Firearms Combat

- Base TN to dodge: **4**.
- Defender takes **+1 TN per 3 rounds fired** (rounded down).

### Weapon table (standard accessories)

| Weapon | Mode | Damage | Notes |
|--------|------|--------|-------|
| Heavy Pistol | BF | 12S | One-handed; allows shield |
| Submachine Gun | FA 6 | 13D | One-handed; base 7M, +1 integral recoil |
| Assault Rifle | FA 6 | 14D / 15S | +1 or +2 integral recoil options |
| Shotgun | BF | 13D | Underbarrel slot for bayonets |
| Sniper Rifle | SA | 14D / 15S | −2 TN bonus for cross-room shots |
| Heavy MG | FA 5 | 16D | Requires Body 8, Strength 8; base 11S |

### Cyberarm Gyromount (3-point recoil compensation)

| Weapon | Mode | Damage |
|--------|------|--------|
| SMG | FA 9 | 16D |
| AR | FA 9 | 18D |
| Light MG | FA 8 | 16D |

### Heavy Gyromount / Tripod (6-point) or Vehicle Mount (halves recoil)

| Weapon | Mode | Damage |
|--------|------|--------|
| SMG | FA 10 | 17D |
| AR | FA 10 | 19D |
| Heavy MG | FA 10 | 21D |

(Damage codes: number = Power; letter = Wound level — L/M/S/D = Light/Moderate/
Serious/Deadly.)

## Attack Optimization (general)

- **TN penalties:** poor light, wounds, range, recoil.
- **TN bonuses:** Smartlink-2, Reach (melee).
- **Dice sources:** Skill, Combat Pool, Enhanced Articulation, Reflex Recorder.

---

## Mage Slaying — "The Most Dangerous Game"

Three primary strategies for engaging spellcasters:

### 1. Range

- Spellcasting is limited to same-room targets (house rule).
- Each room of distance imposes +2 TN.
- Recommended: sniper rifles (count as one less room) or high-power automatics.

### 2. Speed (win initiative → impose wound penalties on the mage's casting)

| Category | Initiative sources |
|----------|--------------------|
| Cyberware | Wired Reflexes, Boosted Reflexes, Move-by-Wire, Reaction Enhancers |
| Bioware | Synaptic Accelerators, Enhanced Articulation, Suprathyroid Gland, Adrenal Pump |
| Magic | Increase Reaction, Increase Reflexes (spells / adept powers) |
| Drugs | Cram, Jazz, Kamikaze |

- **Cap: +5 maximum bonus initiative dice.**

### 3. Tanking

**Direct Combat Spells** (Powerbolt, Manabolt, Stunbolt):
- Opposed test (Sorcery vs. Body / Willpower); cannot be dodged or soaked.
- NPCs avoid high-Body/Willpower targets; default to **Stunbolt**.

**Indirect Elemental Manipulations** (Flamethrower, Lightningbolt, Acid Stream):
- Treated as physical ranged attacks; can be dodged and soaked.
- Capped at Force 10 by strong NPCs.

### Stunbolt defense

- Bioware: Pain Editor, Adrenal Pump.
- Drugs: Kamikaze, Nitro.
- Magic: Spell Defense, Spell Reflect, Magic Resistance (adept power).
- Damage mitigation: Pain Editor (prevents mental penalties), Trauma Damper
  (−1 box), Damage Compensator (reduces wound penalties).

### Invisibility counters

| Spell | Counter |
|-------|---------|
| Invisibility | Thermographic vision |
| Improved Invisibility | Ultrasound **or** Astral Perception |

### NPC mage behavior

1. First buff is always an invisibility variant.
2. Additional buffs applied with randomized delays.
3. Casting inflicts drain (mental fatigue).
4. A single damage box can strip high-force buffs.
5. Dealing damage imposes wound penalties on future casts.
