# AwakeMUD — House Rules (SR3 → MUD translation)

Source: https://awakemud.com/dokuwiki/doku.php?id=houserules
Fetched: 2026-06-18

---

The House Rules document how Shadowrun 3rd Edition tabletop rules are adapted to
real-time multiplayer MUD play. All numeric values below are extracted verbatim
from the source. References to SR3/MitS/SRC/SOTA are the original Shadowrun
source books cited in the rules.

## Time Scale

MUD time runs **30× real time**.

| MUD interval | Real time |
|--------------|-----------|
| 1 Combat Turn (3 sec MUD) | 1/10 second real |
| 1 MUD hour | 2 real minutes |
| 1 MUD day | 48 real minutes |
| 1 MUD month | 1 real day |
| 2.5 MUD years | 1 real month |

## Combat & Initiative

- One attack **or** spell per Combat Phase per character.
- Directional movement cooldown: 1 Combat Phase.
- `FLEE` cooldown: half the time of directional movement.
- Initiative: a single global ladder; combatants enter/exit continuously.
- Manual directional movement: once every 2 Combat Phases.
- Automatic `FLEE`: once per Combat Phase.
- Close to a melee target: requires 2 successes on a Quickness test.
- Flee from melee: requires 1 success on a (Quickness × 1.25) test, per capable
  adversary in the room.

## Healing

Base Physical-damage healing rate is position-dependent, modified by:

| Modifier | Effect |
|----------|--------|
| Medical Workshop | ×1.8 |
| Social Room | ×1.5 |
| Sterile Room | ×1.5 |
| Bioware Overstress | −10% × Rating |
| Rapid Healing | +50% × Rating |
| Lifestyle | varies by type |

- NPC healing rate: 2× PC rate.
- Mental (Stun) track heals 33–50% slower than Physical.

## Death & Respawn

- **Hardcore mode**: opt-in, permanent death.
- **Standard mode**: respawn at DocWagon at full health.
- Newbie protection: until 100 Karma.
- Character purge: after `50 + TKE/10` real-life days of inactivity.
- Magic attribute: **not** reduced by deadly wounds.
- Other attributes: 4% chance of reduction from deadly wounds.

## Attributes

- Character generation: +1 from metatype.
- In-play improvement: **2 Good Karma × new rating + 1,000¥ × new rating**.
- Cap = **Racial Modified Limit (RML)**; Good Karma raises only to RML, not the
  Racial Maximum (RM).
- Hard cap at rating **20**: diminishing returns above — only every 2nd increase
  counts.
- Bioware / Adept Power bonuses count as **augmented** and can exceed RML.

### Attribute-raising via magic

| Source | Maximum |
|--------|---------|
| Adept Powers raising attributes | Force 6 |
| Spells raising attributes | Force 12 (needs 24 dice to auto-succeed the Ritual) |

## Magic Rating

- Hard maximum: **26**.
- Cannot reduce below 1 (implant surgery blocked).
- Initiations beyond Magic 26: no benefit.

## Skill Caps by Magic Priority

| Magic Priority | Magical skills | Negotiation / Languages | All other skills |
|----------------|----------------|-------------------------|------------------|
| **A** (full Magicians / Mages / Shamans) | Rank 12 | Rank 12 | Rank 8 |
| **B** (Adepts / Aspected Conjurers / Sorcerers / Shamanists) | — | Rank 12 | Rank 10 |
| **E** (Mundanes) | — | Rank 12 | Rank 12 |

## Development Costs

### Skills

| Rating | Cost |
|--------|------|
| Rating 1 | 1,000¥ |
| Rating 2+ | 5,000¥ × current rating |

Chargen skill points are a simplified unified system (Active/Knowledge/Language),
1 point per rating regardless of linked attribute. No free Intelligence-based
points (differs from SR3).

### Initiation

| Grade | Cost | TKE gate |
|-------|------|----------|
| 1 | 50,000¥ | 0 TKE |
| 2 | 75,000¥ | 50 TKE |
| 3 | 125,000¥ | 100 TKE |
| 4 | 225,000¥ | 150 TKE |
| 5 | 425,000¥ | 200 TKE |
| 6 | 825,000¥ | then every 200 TKE |

- Adept Power Point (`AddPoint`): **20 Good Karma**, gated by TKE same as Initiation.

## Foci

### Force caps

| Focus | Force cap |
|-------|-----------|
| Weapon Focus (Reach 1–2) | F2 |
| Weapon Focus (Reach 0) | F3 |
| Sustaining Focus | F3 (mostly); F4 for two of them |
| Power Focus | F4 |
| Category Focus | F4 |
| Specific Spell Focus | F8 |
| Spirit Focus | F6 |
| Expendable Focus | F8 |
| Sorcery Library | F8 |
| Conjuring Library | F8 |

### Active focus limits

- Maximum active simultaneously: **(Intelligence)** foci.
- Total Force sum cap: **2 × Magic rating**.
- Sustained spell limit: equal to Sorcery skill number.

## Armor Stacking (layering)

- Highest-rated item: **full value**.
- All other items: **half value each** (rounded down), summed.
- FFBA (Form-Fitting Body Armor): subject to layering but **no** Combat Pool
  penalty and **no** Quickness TN raise.

## Cyberware Restrictions

- Smartlink-II with Level 2 smartgun: **TN −3** (instead of −2).
- Move-by-Wire: mundanes only.
- Cyberlimbs: mundanes only; bonuses not diluted across five hit locations.
- Cyberlimbs/eyes: no Capacity for enhancements (Essence-free).
- Tactical Computer: adds Rating to resist Invisibility (mundanes only).
- Ultrasound cybereyes: not obstructed by goggles/helmets/spectacles.
- Chipjack Expert Driver Task Pool: limited to chipped skill rating; provides
  Task Pool to every skillsoft in jack.
- Social skills penalty (Essence < 3.5): **not** implemented.
- Signature penalty (Essence < 3.0): **not** implemented.

## Bioware

- Counts as augmented (can exceed RML).
- Tailored Pheromones II (mundanes): additional TN −1.

## Essence & Augmentation

- Base Essence: **6**.
- **Essence Hole**: permanent, size of the largest single implant ever installed;
  cannot be repaired — only deletion/recreation recovers lost Essence.

## Spell Mechanics

- Spell success cap: **Force** (applies to Sustained Illusions and any spell).
- Drain stage minimum: 1 success required to stage damage.
- Sustained spells cancelled if damaged and the caster fails a Sorcery
  (Force + Damage Power) test.
- Out-of-combat casting delay: 0.5 real-second minimum between casts.
- Multiple spells per Phase: not implemented.
- Spell targets must be in the **same room** as the caster.

## Conjuring — Elementals & Nature Spirits

- **Elementals**: can sustain spells up to their own Force, indefinitely
  (differs from pen-and-paper).
- **Nature Spirits**: duration 4 MUD days; despawn if the conjurer leaves the
  spirit's Domain. Confusion Power at one-third effectiveness; Concealment Power
  cannot target vehicles, drones, or specific items.

## Ritual Sorcery

- Performed in a Circle or Lodge; **instantaneous** (not hourly as in PnP).
- Cost: raw Nuyen. Spell successes automatically maximized. No ritual team required.

## Astral Projection

- Essence drain: 1 Essence per real-life hour at ideal conditions.
- Rupture threshold: Essence ≤ 1.0 → spontaneous death.
- Warning at Essence 2: "Your link to your physical form grows tenuous."
- Warning at Essence 1: "You feel memories slipping away."
- Automatic return re-integrates to the physical body (differs from PnP).

## Lifestyle & Purchases

- Lifestyles cannot be bought outright for 100 months' rent (SR3 mechanic disabled).
- Lease duration: blocks of 30 real-life days.
- Living-Amenities vehicle: purchasable, indefinite residence (no Lifestyle conferred).
- High Lifestyle permit: Availability TN −1. Luxury Lifestyle permit: Availability TN −2.
- Merchant reorder attempt: after a 1-MUD-day cooldown. Item hold: 7 real-life days.

## Encumbrance & Vehicle Armor

- Carry limit: **30 + (Strength × 10) kg**. Body slots for all worn/carried gear.
- Vehicle armor install cap: half body **or** +2, whichever is lower.

## Vision & Ranged Combat

- Visibility penalties affect Dodge TN (PnP halves them in melee; here full).
- Image-magnification scope range extension:

  | Scope rating | Range | Weapon limit |
  |--------------|-------|--------------|
  | 1 | 2 rooms | SMG |
  | 2 | 3 rooms | rifle / assault rifle / minigun |
  | 3 | 4 rooms | sniper rifle |

- Range penalty per room boundary: TN +2. Sniper rifles ignore the first TN +2.

## First Aid / Biotech

- Unlimited applications while Wounds in Body Overflow (10+ Boxes Stun).
- Post-overflow applications: skill level ÷ 3 attempts max
  - Magician (cap 8): 2 attempts
  - Adept (cap 10): 3 attempts
  - Mundane (cap 12): 5 attempts (4 base +1 bonus at rank 12)
- Retry TN penalty applies regardless of success. Cost per attempt: **150¥**.
- Medkit: not implemented. Trauma Patch: Rating vs. TN 4 to stabilize.
- Stimulant Patch: mundanes only, no damage rebound.

## DocWagon

- Modulator: Platinum (50,000¥). Must be owned, bonded, worn, room accessible.
- Trigger: 10+ Boxes Physical Wounds.
- Cost on successful retrieval: 10% of carried cash (from bank/cash). Preserves job.
- Retrieval failure: delivered to login; equipment dropped in corpse at death
  location (owner-only loot).

## Skillsofts

- ActiveSofts require a Chipjack (with Skillwires); KnowSofts require Datajack +
  Knowsoft Link.
- Skillwires Rating limits each individual skillsoft rating (PnP limits the sum).
- Task Pool subject to magic-priority skill cap (Skillwires + CJED must fit cap).

## Decking (summary; full detail in archetypes-and-magic.md)

- Stock cyberdecks: cannot use Hacking Pool (cannot go hot ASIST).
- Custom cyberdecks: can use Hacking Pool (with hot ASIST).
- Hot ASIST illegality flag triggers NPC aggression on sight. Masking firmware:
  Legality 2-S (most illegal).
- Custom deck memory: Active = MPCP × 250 MP; Storage = MPCP × 600 MP. Utility
  memory cost scales exponentially with rating.
- Hacking Pool with no deck: Intelligence ÷ 3.

## Spell Design & Metamagic

- Learnable spells: Core and MitS only; custom spell engineering not implemented.
- Spell formula consumed upon learning. Exclusive/Fetish limitations not implemented.
- Metamagic: limited (see `HELP METAMAGIC`). Cleansing works on temporary and
  permanent BGC. Spell Defence persists even if caster unconscious. Reflecting
  available but limited to caster only.

## Character Creation Restrictions

- Attribute / Skill / Equipment Rating limit: 6. Availability limit: 8.
- Cyberware set to worthless (free Essence Hole repair by explant/reimplant).
- Restrings: 5 complimentary.
- Metatype variants: rearranged attribute array vs. base; steeper merchant trade
  penalty; dwarf/ork variants charge a steep creation tax (elf/troll do not);
  prestige metatypes (Dryad, Ghouls) consume system points at specific stage modifiers.

## Vehicle Combat

- Melee: vehicles auto-lose. Magic: vehicles auto-win. Ranged: cannot shoot
  outside current room.
- No player control of Control Pool. Drone Pilot not implemented; cannot
  `FOLLOW`/`ASSIST`.
- Vehicle cruise speed: 50% of max (cap 55 kph).

## NOT Implemented

Karma Pool · Edges & Flaws · Contacts · Skill Specializations/Concentrations ·
Projectile weapons (bows, crossbows) · Dual-wielding (melee and ranged) ·
Improved Ability (Adept power) · Enchanting Foci/Fetishes · Astral Quests ·
Watcher/Ally Spirits · Supermachinegun rate-of-fire · Shock-weapon
Strength-coupled damage · Cyberskates · Most weapons/cyberscans/checkpoints ·
Shapeshifters · Otaku.

## Disabled PvP

- Outside the Arena: all PvP disabled.
- Grenades: flagged NERPS (no special coded effects).
- AoE offensive spells: flagged NERPS (no special coded effects).

## Autorun Mechanics

- One active autorun at a time. Last 15 jobs remembered.
- Johnsons decline runners exceeding a reputation threshold (per-job, not per-Johnson).
- NPC respawn: on location, full health/equipment, auto-reload weapon.
- Quest item flag: `(Quest)` in yellow to the runner, `(Protected)` in magenta to others.
