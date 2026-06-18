# NukeFire — Classes & Progression

Source: https://nukefire.org/wiki/classes (and per-class + progression topic pages)
Fetched: 2026-06-18

Deep-read topics for this doc: `classes`, `levels`, `remort`, `remortskills`, `prestige`, `classlegacy`, `alignment`, `age`, `hometowns`, `groupskills`, and the class pages `assassin`, `barbarian`, `curist`, `cyborg`, `knight`, `mutant`, `ranger`, `samurai`, `slinger`, `vagrant`, `pirate`, `infiltrator`, `fanatic`. Prestige class identity comes from the `classes`/`prestige` pages (individual prestige skill pages are referenced in `magic-and-skills.md`).

---

## Overview

NukeFire is a **CircleMUD-lineage, sci-fi/post-apocalyptic** game built around **endless remort**. A character is one class, levels 1→50, then **remorts back to level 1** for permanent bonuses — repeatable indefinitely. Class identity is locked at creation (you cannot reroll class by remorting; you either start over or use `longwalk`). Prestige classes are unlocked by accumulating large numbers of class-specific remorts.

NukeFire is **class-only** (no separate race system surfaced in help) — `mutant`/`cyborg` are classes, not races. The fiction is wasteland: hometowns are **Tek Angeles, Silicia, Cyboria, Lost Wages, Casablanca**.

---

## Base classes

`classes` advertises "10 classes available to new players" but enumerates 13 base archetypes (more "coming soon"). Identities verbatim-paraphrased:

| Class | Role identity | Notes |
|---|---|---|
| **Assassin** | Stealth striker + dark caster | Universally evil-leaning; hide/sneak/backstab + malevolent spells |
| **Barbarian** | Pure melee bruiser | No self-heal/magic; relies on curists + slingers in groups |
| **Infiltrator** | Covert ops / stealth-precision | Slips locks & patrols, strike-and-extract |
| **Fanatic** | Gun-toting zealot (gun + battle-prayer) | Tougher than pure gunners, less refined than casters; → **Occultist** prestige |
| **Curist** | Holy support healer/caster | The old "cleric" role: heal, protect, group recovery, judgment spells |
| **Cyborg** | Cybernetic enhanced human | Repair/overboost/turbo tech skills; nanoswarm DOT |
| **Knight** | Holy warrior (tank + support) | Durable front-line + defensive magic & judgment |
| **Mutant** | Radiation survivor / psionicist | `grow` body-mod system + psionic attacks; mental powers don't hit machines |
| **Ranger** | Wilderness tracker | Stealth in terrain, nature magic, tracking |
| **Samurai** | Disciplined martial warrior | Bushido code, body-and-soul combat mastery |
| **Slinger** | Spellslinger glass cannon | Avoids melee (heavy armor hurts slinging); capstone `arcane torrent` |
| **Vagrant** | Knife-combat skirmisher | Predicts enemy moves; → **Gypsy** prestige |
| **Pirate** | Drunken swashbuckler | Combat + treasure + free-spirit; → **Gypsy** prestige |

### Spell/skill ladders (selected, verbatim from class pages)

**Assassin** — L1 Sneak/Hide/Backstab(+dmg)/Dual Wield/Shoot/Shadowform(toggle); L2 Catseye/Track/Pick Lock; L5 Steal; L7 Poison; L13 Blindness; L15 Dispel Good/Circle; L17 Energy Drain; L20 Protection from Good; L22 Disembowel; L24 Curse; L25 Invisibility; L40 Dagger Dance.

**Barbarian** — L1 Kick + True Grit(innate); L2 Bash; L3 Rescue; L9 Track; L10 Dual Wield/Knee Smash; L12 Elbow Strike; L15 Headbutt + Barehand Proficiency I(innate); L18 Bodyslam; L20 Grapple; L23 Break Door/Clothesline; L25 Barehand Proficiency II(innate); L35 Whirlwind Strike; L40 Tornado Suplex. Passive procs inside Tornado Suplex/melee: finishing move, flying elbow, groin punch, kick-and-stomp, kidney punch, throat punch.

**Cyborg** — L1 DAMCON/Kick/Nanorepair(innate)/Overboost; L2 Selfrepair; L3 Bash; L4 Piston Punch; L5 Entrench; L8 Dual-Wield; L10 Cyborepair/TURBO; L13 Pound; L15 Break Door; L25 Plasma Arc; L35 Neural Overload; L40 Nanoswarm (DOT).

**Curist** (holy caster; uses `sling '<spell>'`) — L1 Cure Light/Armor/Kick; L3 Detect Poison; L4 Call Lightning/Cure Blind/Detect Align; L5 Bless; L6 Blindness/Detect Invis; L7 Catseye; L8 Poison/Prot-from-Evil/Prot-from-Good; L9 Cure Critic/Aura of Protection; L10 Remove Poison/Summon; L11 Holyfist/Identify; L12 Earthquake/Remove Curse/Word of Recall; L14 Dispel Evil/Dispel Good/Heal; L15 Sanctuary; L17 Control Weather/Aura of Escape; L18 Invigorate/Sense Life; L19 Harm; L20 Call/Summon Paladin; L22 Aura of Healing; L25 Rejuvinate; L26 Augury; L30 Aura of Invigoration; L35 Aura of Rejuvination; L40 Restoration.

**Knight** — L1 Kick/Dual Wield; L4 Rescue; L5 Armor/Bless; L6 Bash/Detect Invis/Detect Magic; L7 Cure Blind; L9 Detect Align; L10 Detect Poison; L11 Swordplay; L12 Prot-from-Evil/Prot-from-Good; L15 Flurry; L16 Remove Poison; L18 Dispel Evil/Dispel Good/Remove Curse; L19 Word of Honor/Word of Recall; L20 Grapple/Sword Sweep; L25 Heal/Barrier of Valor/War Banner; L27 Sanctuary; L35 Sacred Vengeance; L40 Oathbreaker's Resolve.

**Mutant** — birth: Devour (eat corpses → mana), Sneak. `grow <part>` body mods: arms (extra attacks), body (HP/move), brain (INT), claws (fighting), eyes (vision), fingers (DEX), fins (swim), muscle (STR/end), scar (heal), shell (protection), skin (eat own flesh). Psionics: L3 emit, L10 spew, L13 psionicwave, L15 psiattack (armor ignored, sanctuary helps), L25 psiblast/Rend + defensive-stun proc, L40 Mind Crush, L45 Radiation Blast. Most mental powers don't affect machines.

**Ranger** — L1 Kick/Nature's Renewal/Shoot/Sneak/Swordplay; L2 Pick Lock; L3 Cure Light; L5 Rescue/Track; L6 First Aid; L9 Bash/Catseye/Dual Wield/Hide; L12 Animal Spirit/Create Water/Detect Poison; L15 Bushwhack/Invigorate/Remove Curse/Remove Poison/Bark Skin; L16 Ambush/Heal; L20 Grapple; L25 Control Weather/Parry; L30 Sword Sweep (AOE).

> Ability-ladder takeaway for AnotherMUD: classes mix **innate/passive** grants (toggles like Shadowform; "always-on" procs like Barehand Proficiency) with **level-gated learnable** skills/spells, and skills improve by **use** (see `magic-and-skills.md`), not by spending practice sessions.

---

## Prestige classes (9)

Unlocked via class remorts (`prestige`). Some need **two class paths**, others **deep single-class mastery**:

| Prestige | Unlock requirement |
|---|---|
| **Headhunter** | 25 Cyborg + 25 Barbarian class remorts |
| **Ninja** | 25 Assassin + 25 Samurai class remorts |
| **Wolfman** | 50 Ranger class remorts |
| **Kaiju** | 50 Mutant class remorts |
| **Heretic** | 25 Curist + 25 Knight class remorts |
| **Gypsy** | 25 Pirate + 25 Vagrant class remorts |
| **Voidstriker** | 50 Slinger class remorts |
| **Outlander** | 50 Infiltrator class remorts |
| **Occultist** | 50 Fanatic class remorts |

Each prestige has signature abilities (e.g. Occultist **Chant** litanies spending *Devotion*; Outlander **Suppress Fire**; Voidstriker **Eldritch Cataclysm** + **Arcane Torrent**-tier capstones; Gypsy **Advanced Knifeplay**, **Sell Your Soul** voodoo contract). See `magic-and-skills.md`.

---

## Levels & XP

- `levels` shows XP needed per level; `levels <n>` shows n levels around current; `levels <min>-<max>` shows a span. Related: `tnl` (to-next-level), `expstart`.
- Level cap before remort is **50**.

## Alignment

Tri-axis, numeric, affects mob aggression and item usability:

| Range | Alignment |
|---|---|
| -1000 to -350 | Evil |
| -350 to 350 | Neutral |
| 350 to 1000 | Good |

Some items are alignment-restricted. Some dark pacts hard-set alignment (Gypsy "Sell Your Soul" → -1000).

## Age

Begins at character creation and **advances in real time whether or not you're logged in**.

---

## Remort (the core long-game)

**Reach level 50 → "buy remort" at the Remorter → reset to level 1** with permanent bonuses. Verbatim mechanics:

- **Cost:** base **50,000 credits**, **+10,000 per subsequent remort**.
- **Stat upgrades** are applied "intelligently" in priority order: **CON, STR, WIS, INT, DEX**.
- Each remort grants **+1 Total remorts AND +1 to your current-class remorts**.
- **Class-specific skills** start appearing at the **5th class remort** of your current class.
- **Switching class:** you begin earning remorts for the new class; you **keep the remort *counts* for prior classes but lose their *benefits***. **Total-remorts benefits are always retained.**
- **Tiered Total-remort bonuses:** at thresholds (50+ remorts, and class milestones at 100/200/300 total class remorts) you gain extra **-armor, +damage, +hit, +mana, +move, and +1 hit-per-round** (see `hpr` in `combat-mechanics.md`).
- The **REMORT flag** confers additional benefits; rumor of special bonuses at high remort counts.
- `remortskills` / `remortskills3` list what your remort journey has unlocked.

### Class Legacy (permanent runes) — `classlegacy` / `runes`

A permanent **rune system for prestige characters**. Runes are bought with current remorts + current class remorts and stay forever. Class Legacy **protects 100-remort milestones** (buying a rune won't drop you below 300 if you're at 300 — you need current remorts *above* the milestone to pay). Utility runes include:

- **Rune of the Morgue-Walker** — protects belongings from pillager looting on death; full restore after leaving the morgue.
- **Rune of the Farrier's Grip** — makes speedwalk movement-interruption much rarer.
- **Rune of the Ragpicker's Tithe** — doubles `rsell` value; `rsell all` may uncover rare remort materials.
- **Rune of the Needlewright's Mercy** — implanting/tattooing cost half; removal free.
- **Rune of the Steady Crucible** — improves TekForge success, prevents critical forge catastrophes.
- **Rune of the Candlekeeper's Claim** — unlocks `candletrack <inscription>` to track lost inscribed items.
- **Rune of the Keymaster** — unlocks `keycopy <key>` anywhere while carrying the real key (copies fade on logout/reboot).

`classlegacy list` (buyable now), `classlegacy all` (every rune), `runes` (already burned in).

### Longwalk

`longwalk` is the documented path to **change class** (vs. remorting, which keeps class). Mentioned by `remort`/`prestige` as the class-switch mechanism.

---

## Group progression (`groupskills`)

A parallel progression axis for **approved 4-person groups**, gated by **group level** + **combined group remorts** (the four members' group-remorts summed; e.g. 4×50 = 200 combined). Distinct from personal remorts.

**Passive group powers** (auto, when the full approved group is together at the required group level):
- **Group Sync Attack** (GL2, 0 remorts) — follow-up strike when members share a target.
- **Group Leader Takes Hits** (GL15) — leader soaks hits meant for members.
- **Curist/Heretic Autoheal Low-Health Tank** (GL20) — emergency heal when the tank is low.
- **Shared Guard** (GL24) — distributes incoming damage, reducing total taken.

**Command group powers** (typed; share a group-skill cooldown):
- **Group Wardrum** (GL15) `groupwardrum` — temp hitroll+damroll.
- **Group Rally** (GL20, 25 combined) `grouprally` — partial HP/Mana/Move + short regen.
- **Coordinated Strike** (GL30, 100 combined) `groupstrike` — mark target, each fighting member strikes in sequence.
- **Group Phalanx** (GL35) `group phalanx` — defensive stance: defense/DR/hold-the-line.
- **Battle Lattice** (GL40, 200 combined) `grouplattice` — boosts melee power/spellpower/DR/speed.
- **Group Restore** (GL50, 40 combined) `grouprestore` — full HP/Mana/Move for all four.

`groupskills`/`gskills` reports formation completeness, lowest group level, combined remorts, cooldown state, and READY/LOCKED per power.

---

## Relevance to AnotherMUD

- **Endless-remort loop** is a strong contrast to AnotherMUD's track/proficiency progression — worth noting as a "prestige + permanent meta-bonus" model if a long-game meta is ever wanted.
- **Group-remort as a separate currency** unlocking shared group powers maps cleanly onto AnotherMUD's group system if cooperative endgame content is desired.
- **Class Legacy runes** are a clean "permanent QoL unlock store" pattern (anti-looting protection, cheaper tattoos, key-copying) that decouples grind from raw power.
