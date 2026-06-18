# AwakeMUD — Features Overview

Source: https://www.awakemud.com/ (+ archetype blurbs decker.html, street_samurai.html, adept.html, hermetic.html, shaman.html); https://awakemud.com/dokuwiki/ (start page table of contents)
Fetched: 2026-06-18

---

## What AwakeMUD Is

AwakeMUD CE is a text-based multiplayer game (MUD) set in a cyberpunk future
Seattle, implementing the **Shadowrun 3rd Edition** tabletop ruleset (with house
rules — see `house-rules.md`).

> "The year is 2064, and the world has been reforged in the fires of magic,
> technology, and greed."

Players take the role of **Shadowrunners** — freelance agents who conduct
illegal missions ("runs") for anonymous employers ("Mr. Johnsons") while building
power, reputation, and wealth in Seattle.

### Connection

| Channel | Detail |
|---------|--------|
| Telnet | `play.awakemud.com` port `4000` |
| Web play | via Grapevine |
| Discord | community / newcomer support |

### Project notes

- 100% free; no microtransactions or cash shops
- Active Discord community welcoming newcomers
- Comprehensive dokuwiki documentation

---

## Setting — Seattle 2064 (the Sixth World)

Shadowrun's "Sixth World": magic returned to the world (the "Year of Chaos,"
2011), metahumanity emerged (orks, trolls, elves, dwarves), and megacorporations
(AAA-corps like Aztechnology, Renraku) wield more power than governments. People
without a System Identification Number (SIN) — the "SINless" — live in the
shadows. See `getting-started.md` for the full runner-slang glossary that
encodes the setting.

---

## The Five (Six) Archetypes

Character creation offers these archetypes (see `character-creation.md` and
`archetypes-and-magic.md` for full builds):

| Archetype | Tagline | Role |
|-----------|---------|------|
| **Street Samurai** | "Warriors of Chrome and Steel" | Cybernetic combat specialist; no magic, all tech. Melee + ranged. |
| **Decker** | "Data is Nuyen" | Hacks the Matrix via cyberdeck. Highest earner, weak in a fight. |
| **Physical Adept** | "Seeking Perfection" | Magic enhances the body — stronger, faster, tougher. Melee-focused. |
| **Hermetic / Street Mage** | "Knowledge is Power" | Arcane formula caster; conjures & binds **Elementals** (stronger than spirits). |
| **Shaman** | path of "harmony with nature" | Draws power from a **Totem**; emotion/instinct casting; weaker spirits but higher ceiling. |

(The chargen menu references six archetypes; "Street Mage" and "Hermetic Mage"
are the mage-tradition split alongside the Shaman.)

### Archetype flavor (homepage blurbs)

- **Decker:** "the digital pulse of the world at your fingertips." Full-dive VR
  into server hosts; risks "dumpshock" from security traps. "Deckers are some of
  the highest-earning folks out there, and although they're not much good in a
  fight, your average decker is still going to have more nuyen than they know
  what to do with."
- **Street Samurai:** "a fast and bloody one, where flesh is traded away for the
  newest and shiniest upgrades to the human form." No magic; relies on
  "sufficiently-advanced technology." Melee (arm blades) and ranged (rifles).
- **Adept:** "true power comes from within... the Adept hones their innate gifts,
  making themselves stronger, faster, and better-equipped." Powerful Adepts "can
  punch through concrete, wade through fire, and even survive hits from assault
  cannons."
- **Hermetic Mage:** "Knowledge is Power." Conjures and binds **Elementals**,
  which outperform Shamanic spirits and protect the mage. Hard to kill if given
  time to prepare defenses.
- **Shaman:** power from a **Totem**, "channels their strength through emotion
  and instinct rather than logic." Can exceed Hermetic ceilings (not bound to
  written formulae) but commands weaker spirits and obeys Totem restrictions.

---

## Major Systems

| System | Summary | See |
|--------|---------|-----|
| **Physical Combat** | SR3 d6 vs. Target Number; firearms + melee; armor soak; cover/prone/pain | `combat-numerics.md` |
| **Magic** | Spells (Force/Drain), Conjuring (Elementals/Spirits), Astral Projection, Foci, Initiation/Metamagic | `archetypes-and-magic.md` |
| **The Matrix (Decking)** | Cyberdecks (MPCP), persona + utility programs, IC, paydata economy, ASIST | `archetypes-and-magic.md` |
| **Rigging / Vehicles** | Vehicle Control Rig, drones, vehicle mods, mounts, control pool | `archetypes-and-magic.md` |
| **Cyberware / Bioware** | Augmentation, Essence cost, incompatibilities | `archetypes-and-magic.md` |
| **Economy** | Nuyen, credsticks vs. cash, paydata, lifestyles, shops | `getting-started.md`, `house-rules.md` |
| **Roleplay (Pruns/ImmRuns)** | Player- and staff-hosted RP runs, rewarded with karma/nuyen/syspoints | `getting-started.md` |

---

## Dokuwiki Table of Contents (reference)

### Playing the Game
- Getting Started FAQ (`faq`), Character Creation Walkthrough (`chargen`)
- Sunny's Guide to Runner Slang (`runner_slang`)
- List of Grid Locations (`gridguidelist`), Runner Directory (`phonebook`)
- Background Helper (`background`), Aliases (`aliases`), Help Files (`helpfiles`)

### Maps
- Maps (`maps`), SVG Maps (`svgmaps`)

### Game Mechanics
- House Rules (`houserules`), Armor Layering Calculator (external)
- Physical Combat (`meatguide`), Combat Numerics (`khai_numerics`)
- The Most Dangerous Game / Mage Slaying (`mageslaying`)
- Incompatible Cyberware and Bioware (`incompatible_ware`)
- Khai's Cyberware/Bioware Musings (`khai_ware`)

### Street Samurai
- Street Samurai Archetype (`streetsamarch`), Dashing Through the Shadows (`shadowdashing`)

### Adepts
- Adept Archetype (`adeptarch`), Khai's Adept Musings (`khai_adept`)

### Mages and Shamans
- On the Subject of Magic (`subject_of_magic`) — in-character lore essay
- Street Mage Archetype (`streetmagearch`), Shaman Archetype (`shamanarch`)
- Talbot's Musings on Hermetic Magic (`talbot_mage`)

### Deckers and Decking
- Decker Archetype (`deckerarch`), Decking with Daiquiri (`decking_with_daiquiri`)
- Riv's Opinionated Guide to Deckbuilding (`riv_deckbuilding`)
- Sunny's Guide to Decking (`sunnys_decking_guide`)
- Sunny's Guide to Programming and Deckbuilding (`sunnys_guide_to_programming_and_deckbuilding`)
- Khai's Guide to Building a Decker (`khai_deckbuilding`)
- Traps in the 'trix (`trix_traps`), Decker Quests (`deckerquests`), Black Burst (`blackburst`)

### Vehicles and Rigging
- Vehicle Mod Slots (`vehicle_mod_slots`), Hail's Riggering Guide (`hail_riggering_guide`)
- Azazel's Rigger Notes (`azazel_rigging`), Bishi's Rigging Guide (`bishirig`)

### OOC Information
- Roleplaying Run FAQ (`roleplaying_run_faq`)
- PGHQ Guidelines (`pghq_guidelines`), PGHQ Gear Rules (`pghq_extras`)
- Syspoint Bounties (`syspoint_bounties`)
