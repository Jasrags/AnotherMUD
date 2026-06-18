# AwakeMUD — Archetypes, Magic, Matrix, Rigging & 'Ware

Source: per-archetype guides (streetsamarch, deckerarch, adeptarch, shamanarch,
streetmagearch); https://awakemud.com/dokuwiki/doku.php?id=subject_of_magic;
https://awakemud.com/dokuwiki/doku.php?id=sunnys_decking_guide;
https://awakemud.com/dokuwiki/doku.php?id=hail_riggering_guide;
https://awakemud.com/dokuwiki/doku.php?id=incompatible_ware;
homepage archetype blurbs
Fetched: 2026-06-18

---

## The Five Archetypes — recommended starting builds

### Street Samurai ("Warriors of Chrome and Steel")

Mundane (no magic). Melee + ranged combat specialist trading flesh for chrome.

- **Skills:** Athletics 3, Clubs 6, Assault Rifles 6, Electronics 3, Biotech 5,
  Negotiation 3, Street Etiquette 3, Driving Motorcycles 1.
- **Cyberware:** Armored mk. II cyberarms; smartlink-2 system.
- **Bioware:** Enhanced articulation; metabolic arrestor; pain editor (cultured);
  symbiotes II.
- **Armor (cumulative):** Secure jacket 5B/3I; plated armor vest 4B/3I;
  form-fitting body armor III 4B/1I; forearm guards +0B/+1I.
- **Primary weapon:** Colt M-23 (assault rifle) with smartlink-2, improved gas
  vent IV, foregrip.
- **Tips:** Set firing mode to full-auto 6; allocate combat pool to damage soak
  (good armor); attack spellcasters at close range; pain editor reduces stun
  penalties — monitor physical damage carefully.

### Adept ("Physical Adepts: Seeking Perfection")

Magic-priority B; powers enhance the body. Melee weapons outperform firearms.

- **Skills:** Athletics 3, Pole Arms 6, Rifles 6, Electronics 3, Negotiation 3,
  Corporate Etiquette 3, Street Etiquette 3, Stealth 6, Driving Motorcycles 1,
  Brawling +2 (bonus).
- **Adept Powers (point cost):** Improved Quickness 1, Kinesics 1, Side Step 6,
  Counterstrike 2.
- **Equipment:** Ruger 100 sport rifle (laser sight, silencer, bayonet); wooden
  bo staff; layered armor (jacket/vest/body); DocWagon modulator; Harley
  Scorpion (vehicle).
- **Tips:** "Activate your powers!" Lower armor than other archetypes is
  "compensated through dodge dice" and counterstrike. Kinesics is strong for
  negotiating higher pay and buying/selling.

### Shaman (path of harmony with nature)

Magic-priority A; power from a **Totem**; emotion/instinct casting.

- **Skills:** Pole Arms 3, Shotguns 5, Negotiation 3, Conjuring 6, Sorcery 6,
  Street Etiquette 3, Driving Trucks 1.
- **Totem: Wolf** — +2 dice to combat spells, +2 dice to detection spells, +2
  dice to forest spirits.
- **Spells & Forces:** Stunbolt F6, Heal F5, Improved Invisibility F6, Stealth
  F1, Levitate F1, Armor F3, Combat Sense F3.
- **Foci:** 3 orichalcum bracelets + 1 ash-leaf anklet (sustaining, F1–F3).
- **Gear:** Remington 990 shotgun (laser sight, bayonet); DocWagon basic
  modulator; armor total 10B/5I (layered).

### Street Mage / Hermetic ("Knowledge is Power")

Magic-priority A; arcane formula caster; conjures & binds **Elementals**
(stronger than spirits, sustain spells, protect the mage).

- **Skills:** Clubs 3, Tasers 5, Electronics 3, Negotiation 3, Conjuring 6,
  Sorcery 6, Corporate Etiquette 3, Driving Cars 1.
- **Spells & Forces:** Stunbolt F6, Increase Body F3, Improved Invisibility F6,
  Stealth F1, Armor F3, Levitate F1, Waterbolt F5.
- **Foci:** Power focus (R1) orichalcum necklace; three R3 sustaining orichalcum
  bracelets (armor, increase attribute, stealth).
- **Cyber/Bioware:** Trauma damper (cultured), Cerebral Booster II (cultured),
  Smartlink-2 system (alpha).
- **Armor/weapon:** Plated vest 4B/3I + form-fitting III 4B/1I; Defiance Super
  Shock taser with smartlink-2.
- **Tips:** "Activate your power focus"; sustain spells via foci/elementals to
  avoid action penalties. Stunbolt counters living targets; waterbolt handles
  non-living obstacles. Improved Invisibility + Stealth bypasses ultrasound.

### Decker ("Data is Nuyen")

Hacks the Matrix; fights like a (lighter) street samurai in meatspace.

- **Skills (R6):** Submachine Guns, Computers, Computer Building and Repair, Data
  Brokerage. Secondary: Athletics 3, Clubs 3, Electronics 3, Biotech 4,
  Negotiation 3, Corporate Etiquette 3, Driving Trucks 1.
- **Starting cyberdeck:** CMT Avatar, MPCP r7.
- **Persona programs:** Evasion r3, Bod r6, Masking r6, Sensors r6.
- **Utilities:** Matrix Sword r4, Armor r5, Decrypt r6, Analyze r6, Deception r6,
  Browse r6, Read/Write r6, Sleaze r5.
- **Weapon:** Ingram Smartgun (SMG) with improved gas vent II and smartlink-2
  (burst/full-auto).
- **Early progression:** Acquire a Programming Suite first, then Evaluate and
  Defuse programs. Complete the Young Decker Quest series at 200 reputation
  before building a custom deck. Replace gas vent 2 with improved gas vent 4 at a
  gunsmithing workshop for full-auto 6.

---

## Magic System

The in-character lore essay "On the Subject of Magic" (by Epiphany) covers the
philosophy (hermetic vs. shamanic), cultural traditions (Aztec, Egyptian, Norse),
and schools of thought (Classic Hermeticism, Unified Magic Theory) — it directs
to MitS and SOTA 2064 for mechanics. The mechanical rules live in `house-rules.md`
and `combat-numerics.md`. Key points:

- **Spellcasting** rolls Sorcery vs. a TN; spell success is capped at the spell's
  **Force**. Targets must be in the same room.
- **Drain**: every cast inflicts mental fatigue; staging drain requires ≥1 success.
- **Conjuring**: Hermetics summon **Elementals** (stronger, sustain spells up to
  their Force indefinitely); Shamans summon **Nature Spirits** (weaker, 4 MUD-day
  duration, despawn outside their Domain).
- **Astral Projection**: drains 1 Essence/real-hour; Essence ≤ 1.0 → death.
- **Foci / Initiation / Metamagic**: see `house-rules.md` for Force caps,
  initiation costs (50k–825k¥, TKE-gated), and the limited metamagic set.
- Direct combat spells (Manabolt/Powerbolt/Stunbolt) are opposed and cannot be
  dodged/soaked; indirect elemental spells (Flamethrower/Lightningbolt/Acid
  Stream) act as physical ranged attacks (Force 10 cap by strong NPCs).

---

## The Matrix (Decking)

### Cyberdeck components

| Component | Detail |
|-----------|--------|
| **MPCP** (Master Persona Control Program) | Deck OS; caps persona/utility ratings. Total persona chip points = MPCP × 3; each persona program capped at MPCP. Response Increase max = MPCP ÷ 3 (round down). |
| **Hardening** | Resistance to Black/Gray IC (Black Hammer, Killjoy). |
| **Active Memory (RAM)** | Running-utility capacity (MP); cannot exceed AM total. |
| **Storage Memory** | File storage; utilities in storage still consume space. |
| **I/O Speed** | Transfer rate in MP per combat turn. |

- **ASIST**: Cold (store-bought decks) vs. Hot (custom decks only — full decker
  potential but risks brain damage).
- **Sample hot-ASIST custom deck** (start ~200,000¥): MPCP 6+, Hardening 4,
  Active 1000 MP, Storage 3000 MP, I/O 240.
- **Custom-deck memory scaling** (house rule): Active = MPCP × 250 MP;
  Storage = MPCP × 600 MP. Utility memory cost scales exponentially with rating.

### Persona programs

| Program | Function |
|---------|----------|
| BOD | Icon damage resistance |
| SENSORS | Detection capability |
| MASKING | Evade host/IC notice. Detection Factor = (Masking + Sleaze) ÷ 2 |
| EVASION | Matrix-combat defense |

BOD and SENSORS must be installed for the deck to boot. Max combined persona
rating = MPCP × 3.

### Utility programs (action → utility)

| Action | Utility | Notes |
|--------|---------|-------|
| Abort shutdown | Swerve | — |
| Attack | Attack, Lock-on | Lock-on prevents enemy evasion |
| Analyze | Analyze | Works on IC, files, subsystems |
| Armor | Armor | Reduces incoming IC damage |
| Crash host | Swerve | Weakens IC |
| Decrypt | Decrypt | Remove encryption |
| Disarm | Defuse | Remove databombs |
| Evade | Cloak | Avoid IC detection |
| Download | Read/Write | Transfer files |
| Locate host | Browse | — |
| Locate IC | Analyze | Hidden IC detection |
| Locate decker | Scanner | Hidden decker detection |
| Logoff | Deception | Exit safely |
| Logon host | Deception | Access host |
| Parry | Cloak | Defense boost |
| Repair | Medic | Icon restoration |
| Scan decker | Scanner | Detailed decker assessment |
| Sleaze | Sleaze | Raises Detection Factor |
| Trace redirect | Camo | Datatrail concealment |

### Decker chrome

- **Essential:** Datajack (to jack in).
- **Recommended:** Math SPU (+hacking pool = rating), Encephalon (+hacking pool =
  rating), Cerebral Booster II (+Intelligence).
- **Not recommended:** Vehicle Control Rig (hacking-pool penalty).
- Hacking Pool with no deck: Intelligence ÷ 3 (house rule).

### Paydata economy

- Security color value (low → high): **Blue < Green < Orange < Red**.
- Valuation factors: host security color; data rarity/saturation; freshness;
  Negotiation differential; Data Brokerage skill.
- Baseline price: 1000–2000¥ for standard blue/green data.
- Vendor: Krackerjack (West Tacoma, Crescent St) — pays more for fresh orange/red.

### Sample host security (Seattle/Tacoma LTG)

| Host | Threat | Notes |
|------|--------|-------|
| Seattle Library | Nil | Baseline; 1000–2000¥ paydata |
| Dantes' | Danger | IC present; entry to LTG |
| Nybbles n Bytes | Nil–Low | Independent; 1000–2000¥ paydata |
| Tacoma Junkyard | Low | IC if careless |
| Tacoma Mall | Low | Abundant paydata; non-lethal IC |

---

## Rigging & Vehicles

- **Reaction** from Quickness + Intelligence; **Combat Pool** from Intelligence +
  Quickness + Will. **Will** resists dumpshock (triggered if your rig is destroyed).
- **Example night-one rigger stats:** Body 1, Quickness 8, Strength 4,
  Intelligence 6, Charisma 3, Will 6, Reaction 7.

### Rigger 'ware (Essence)

| Item | Type | Essence |
|------|------|---------|
| Boosted Reflexes III (alpha) | Cyber | 2.24 |
| Reaction Enhancer (alpha) | Cyber | 0.24 |
| Datajack (alpha) | Cyber | 0.16 |
| Vehicle Control Rig II (alpha) | Cyber | 2.40 |
| Synaptic Accelerator II (cultured) | Bio | Rating 2 |
| Cerebral Booster II (cultured) | Bio | Rating 2 |
| Muscle Toner IV | Bio | Rating 4 |
| Enhanced Articulation | Bio | Rating 1 |

### Drones & vehicles

- Starting vehicle: Ford Americar (storage). Primary combat drone: GM-Nissan
  Doberman ("most armored drone available"). Control deck: Toyota ECR-3 Remote
  Control Deck.
- Mounts: one external hardpoint (max); one firm point (max; possibly three firm
  points total).
- **Cannot** control a drone/vehicle remotely while in a vehicle.
- Combat rule: set speed to **idle** before targeting mounts, or the drone/vehicle
  keeps ramming the target.
- Vehicle armor install cap (house rule): half body or +2, whichever is lower.

### Rigging commands

`subscribe` (secure drone access) · `return` (consciousness to body) · `target`
(designate mounts) · `upgrade my.doberman hard/firm` · `attach my.doberman
(weapon) (mount#)` · `reload (drone) (mount#)` (while holding ammo).

---

## Cyberware / Bioware Incompatibilities

Augmentations that cannot be combined (NERP = effectively non-functional/blocked).

### Cyberware vs. cyberware

| Augmentation | Incompatible with | Notes |
|--------------|-------------------|-------|
| Filters | same-type filters, NERP, Vehicle Control Rig | — |
| Boosted Reflexes | Boosted Reflexes, Vehicle Control Rig, Wired Reflexes, Move-By-Wire | doesn't stack with spell/adept reaction/init |
| Wired Reflexes | Boosted Reflexes, Move-By-Wire | doesn't stack with spell/adept reaction/init |
| Move-By-Wire | Boosted Reflexes, Wired Reflexes, Reaction Enhancers | doesn't stack with spell/adept reaction/init |
| Oral Weapons | other Oral Weapons | one oral weapon per body |
| Dermal Plating | Dermal Sheathing, Cyberskull, Cybertorso, Cyberarms/legs | — |
| Dermal Sheathing | Dermal Plating, Cyberskull, Cybertorso, Cyberarms/legs | — |
| Reaction Enhancers | Move-By-Wire | doesn't stack with spell/adept reaction/init |
| Cyberskull | Dermal Plating, Dermal Sheathing | mundanes only |
| Cybertorso | Dermal Plating, Dermal Sheathing, Bone Lacing | mundanes only |
| Cyberarms/legs | Dermal Plating, Dermal Sheathing, Bone Lacing, Muscle Replacement | mundanes only |
| Muscle Replacement | Cyberarms/legs | — |
| Bone Lacing | Cybertorso, Cyberarms/legs | — |
| Optical Magnification | Electronic Magnification | — |
| Electronic Magnification | Optical Magnification | — |
| Datajack (any type) | other datajacks | — |

### Cyberware vs. bioware

| Cyberware | Incompatible bioware | Notes |
|-----------|----------------------|-------|
| Air Filtration | Tracheal Filter | NERP |
| Ingested Filtration | Digestive Expansion | NERP |
| Move-By-Wire | Adrenal Pump, Synaptic Accelerator, Suprathyroid | doesn't stack w/ reaction/init |
| Wired Reflexes | Synaptic Accelerator | doesn't stack w/ reaction/init |
| Cyber Eyes & Eye Mods | Cat's Eyes, Nictitating Membranes | includes Protective Covers |
| Muscle Replacement | Muscle Augmentation, Muscle Toner, Calcitonin, Erythropoietin | — |
| Dermal Plating | Orthoskin | — |
| Dermal Sheathing | Orthoskin | — |
| Cyberskull | Orthoskin | — |
| Cybertorso | Orthoskin | — |
| Cyberarms/legs | Orthoskin, Muscle Augmentation, Muscle Toner, Calcitonin | — |

### Bioware vs. bioware

| Bioware | Incompatible with | Notes |
|---------|-------------------|-------|
| Calcitonin | Platelet Factories | mundanes only |
| Platelet Factories | Calcitonin | aspirin/anticoagulants to avoid heart attacks |
| Metabolic Arrester | Adrenal Pump, Suprathyroid | — |
| Adrenal Pump | Metabolic Arrester | — |
| Suprathyroid | Metabolic Arrester | — |

Additional: Erythropoietin and Phenotypic Alteration are mundane-only. Trauma
Dampers, Pain Editors, and Damage Compensators have functional incompatibilities
despite being technically compatible.
