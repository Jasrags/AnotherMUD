# NukeFire — Combat Mechanics

Source: https://nukefire.org/docs/combat-mechanics
Fetched: 2026-06-18

---

## Combat Engine Overview

- Tick-based: ~10 ticks per second
- NOT synchronized rounds — each combatant has their own counter
- Alternates between player and NPC processing on alternating ticks
- High-level NPCs may act every eligible tick (counter = 0)

---

## Fight Speed (FS / CS / Combat Speed)

**Lower = faster. Minimum = 2.**

When your FS counter hits 0, you take your turn (all HPR attacks fire), then counter resets.

### Base FS by Class Tier

| Tier | FS | Classes |
|---|---|---|
| Extremely fast | 3–4 | Wolfman, Ninja |
| Fast | 5–6 | Headhunter, Gypsy, Outlander, Infiltrator, Fanatic, Occultist, Vagrant, Assassin, Heretic, Kaiju |
| Moderate | 7–8 | Ranger, Samurai, Cyborg, Pirate, Voidstriker, Barbarian, Knight, Mutant |
| Slow | 10 | Slinger, Curist |

### FS Modifiers

**DEX:**
| DEX | FS Effect |
|---|---|
| 20+ | -2 |
| 18-19 | -1 |
| 17 | 0 |
| 14-16 | +1 |
| 10-13 | +2 |
| 9- | +3 |

**Heavy Armor (AC-based penalties):**
| AC | FS Penalty |
|---|---|
| Below -4,000 | +1 |
| Below -6,000 | +2 |
| Below -12,000 | +4 |
| Below -18,000 | +6 |
| Below -24,000 | +8 |
| Below -35,000 | +10 |

**Weight:** Every 500 lbs effective weight (worn/2 + carried) = +1 FS

**Item Count:** Carrying too many items adds FS penalty regardless of weight

**Active Spell Penalties:**
| Spell | FS Penalty |
|---|---|
| Sanctuary | +1 |
| Blindness | +3 |
| Poison | +3 |
| Chill Touch | +3 |

**Gear:** APPLY_SPEED on equipment adds/subtracts directly. Can push FS below 2 mathematically but clamps at 2.

**Check:** `whatsmy cs`

---

## Hits Per Round (HPR)

**Capped 1–10. More = better.**

All HPR attacks fire on your turn. Each is independent (own hit roll, damage, proc chance).

### HPR Sources

1. **One_Hit stat** (base + remort bonus + gear APPLY_ONE_HIT)
2. **Light load bonus:** effective weight under 200 lbs = +1 HPR
3. **Weight penalty:** every 500 lbs = -1 HPR
4. **Heavy armor penalty:**
   | AC | HPR Penalty |
   |---|---|
   | Below -6,000 | -1 |
   | Below -12,000 | -2 |
   | Below -16,000 | -3 |

**Check:** `whatsmy hpr`

---

## FS + HPR Together

Damage output ∝ HPR / FS

- FS 3, HPR 2 = 2 attacks per 3 ticks — high frequency, good for procs
- FS 7, HPR 5 = 5 attacks per 7 ticks — same rate but burstier feel

---

## Hit Chance (Melee)

- Base: 55%
- Modified by: target AC (lower AC = harder to hit), attacker hitroll, d20 luck component
- Minimum: 20% (always some chance)
- Maximum: 95% (never guaranteed)
- Blur effect on defender: -10% hit chance

---

## Avoidance (Separate from AC)

Fires AFTER a hit is confirmed — chance to negate the hit entirely.

| Class | Avoidance |
|---|---|
| Occultist | High |
| Ninja | Medium |
| Ranger, Pirate, Gypsy, Voidstriker, Kaiju, Mutant, Assassin, Wolfman | Low |

Passive, automatic, always active.

---

## Damage Per Hit (Melee)

**Weapon damage roll + Damroll**

Damroll sources: base + STR bonus + level bonus + remort bonus + concealed weapon bonus + gear/spells

**Check:** `whatsmy damroll`

---

## Damage Reduction Pipeline (5 stages, applied in order)

### Stage 1: Block Mechanics (before % reductions)

| Block Type | Trigger | Effect |
|---|---|---|
| Shield Block | 10 + Shield_Block stat % | Damage ÷ 5, then - Shield_Block value |
| Kaiju Block | Kaiju-specific | Scales with stats |
| Headhunter Block | Headhunter-specific | Scales with remort level |
| Shield of Flames | Spell effect | Negates entire attack (0 damage) |

### Stage 2: Percentage Reductions (multiplicative)

Applied multiplicatively — each reduces what remains after previous.

| Effect | Reduction |
|---|---|
| Sanctuary | 10% |
| Resolve | 10% |
| Eldritch Aura | 10% |
| Bark Skin | 10% |
| Blood Aegis | 10% |
| Voidstep | 10% |
| Wasteland Calm | 10% |
| Radcloak | 10% |
| Dark Covenant | 10% |
| Firebrand | 10% |
| Shadowform | 10% |
| Wraith Form | 10% |
| Panzer Form | 10% |
| Grow Shell | 10% |
| Voodoo Contract | 10% |
| Ka Ward | 20% |
| Barbarian Passive (5+ remorts) | 10% |
| Protection from Evil/Good | 3% (vs aligned only) |
| Dark Blessing | 3% |
| Ghost Step | 12–25% (12% base, 25% at 100+ Ninja remorts) |
| Bestial Fortitude | 10–40% (scales with Wolfman remorts, 40% at 800+) |
| Grow Carapace | 35–40% (35% base, 40% at 150+ Kaiju remorts) |
| Phantom Mirage | 50% (applies to attacker's output) |

Dodge procs that fire in this phase (chance to negate remaining damage entirely):
- Mirage Block: 30%
- Turkey Feather Block: 30%
- Voidstep Block: 30%

### Stage 3: Flat AC Reduction

AC / 50 = flat damage reduction per hit
Example: AC -5,000 = 100 flat reduction per hit

### Stage 4: Damage Reduction (DR) Stat

Random reduction between 1 and DR score per hit. Not guaranteed — random ceiling.
Visible as `damredux` in score.
**Check:** `whatsmy damredux`

### Stage 5: Class Deflection (High Remort)

| Class | Remort Threshold | Ability |
|---|---|---|
| Headhunter | 150+ | Headhunter Deflection |
| Kaiju | 150+ | Kaiju Deflection (can reduce to 0) |
| Wolfman | 650+ | Wolfman Deflection |
| Ninja | 600+ | Ninja Deflection |

---

## True Damage

Some attacks bypass part or all of the reduction pipeline. Mechanism: damage splits into:
- True portion (skips some stages)
- Reduced portion (normal pipeline)

Some powerful NPC attacks scale based on current HP rather than fixed damage.

---

## Active Combat Skills

- Skills have **cooldowns** (ticks)
- **Passive procs** trigger automatically on attacks
- **Zero-wait combos**: some skills have 0 wait time, allowing chaining — but capped by a combo burst budget to prevent infinite loops
- **NPC Learning System**: powerful NPCs learn your attack patterns over ~120 seconds and can partially block your heaviest hits. Resets if you vary patterns or pause. Cooldown ~4 seconds between predictions.

---

## Guns (Completely Separate System)

Guns do NOT use the FS/HPR framework. Firing imposes a **massive wait state** instead.

### Primary Gun Classes (accuracy advantage + damage multipliers)
Outlander, Infiltrator, Fanatic, Occultist

### Secondary Gun Classes (lesser accuracy)
Headhunter, Ranger, Pirate, Gypsy

### Gun Hit Chance (d20 system, not %)

| Target AC | DC to beat on d20 |
|---|---|
| Above -1,000 | 4-6 |
| -1,000 to -5,000 | 7-10 |
| -5,000 to -12,000 | 11-15 |
| -12,000 to -22,000 | 16-18 |
| Below -22,000 | 19 |

1 = always miss. 20 = always hit. Modifiers: DEX vs target DEX, shoot skill, hitroll, target paralyzed (+2), tanking (penalty), blind (penalty).

### Caliber Damage Ranges

| Caliber | Damage Range |
|---|---|
| .22 | 70–420 |
| 9mm | 80–560 |
| .38 | 90–720 |
| .45 | 100–900 |
| .44 Magnum | 120–1,200 |
| .357 | 140–1,400 |
| 20 Gauge | 220–3,300 |
| 12 Gauge | 240–3,840 |
| 10 Gauge / .410 | 250–3,750 |
| 10mm | 250–5,000 |
| 20mm | 200–5,000 |
| 5.56mm | 250–2,500 |
| 7.62mm | 190–2,280 |
| .308 | 200–1,800 |
| .30-06 | 200–2,000 |
| **.50 BMG** | **2,000–40,000** |
| **40mm Grenade** | **3,800–76,000** |

Flat bonuses on top: hitroll + Bullet Impact Damage stat

### Marksmanship Multiplier (Primary Gun Classes)

On successful Marksmanship check: **damage × (2 + remort_count/20)**
- Fresh primary gun class: 2×
- 20 remorts: 3×
- 100 remorts: 7×

Occultist special: Mark Outer Dark — if target has been marked, multiplier is higher (redacted in docs)
Outlander special: Kneecap ability — multiplies shot damage on trigger

### Firing Modes
- **Single shot** — one round, full wait state
- **Burst fire** — multiple rounds in one action
- **Magazine dump** — fire at random targets until empty
- **Sniping** — fire into adjacent room (`shoot north`): 2× damage, requires sniper rifle, blocked by locked doors/peaceful rooms

### Trigger Control skill
5% chance to not consume ammo when firing.

### Ammo System
- Guns consume ammo from loaded magazines
- Reload is automatic from inventory (adds combat delay)
- Ammo ledger system: tracks stored ammunition separately from magazine items

---

## Grenades

Timer-based. When detonated:
- Carrier/floor: full damage
- Others in room: 1/4 damage
- Adjacent rooms: hear explosion

Damage: dice × 100 (highly variable)
Types: Explosive (fire), Electric
50% chance of dud if detonated on yourself

---

## AOE Attacks

Centralized targeting:
- Your summoned/charmed followers: NEVER hit
- Allied group member's summons: NEVER hit
- Enemy NPCs and their followers: ALWAYS hit
- Other players: ONLY in Boneyard PvP zone

AOE skill examples:
- Sword Sweep, Whirlwind Strike, Radiation Burst, Dagger Dance, Vomit
- Area spells: Earthquake, Firestorm, Hellquake, Ravaged Terrain, Rotting Blight

---

## Diagnostic Commands

| Command | Shows |
|---|---|
| `whatsmy cs` | Fight speed breakdown with every modifier |
| `whatsmy hpr` | Hits per round breakdown |
| `whatsmy damroll` | Full damroll calculation |
| `whatsmy armor` | Full AC breakdown by source |
| `score` | Complete stat summary |
| `afx` | Active effects and passive stat bonuses |
| `faq fightspeed` | In-game help on FS |
| `faq hpr` | In-game help on HPR |
| `faq damredux` | In-game help on damage reduction |
| `faq armor` | In-game help on AC |
