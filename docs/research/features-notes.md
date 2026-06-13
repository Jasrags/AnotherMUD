# Discworld MUD — Features Notes

Source: discworld.imaginary.com (robots.txt blocks direct fetch; content from search snippets)
Engine: FluffOS (fork of MudOS — LPMud family)
Online since: 1991 (public 1992)

---

## Overview

Free MUD set in Terry Pratchett's Discworld. Continuous updates since 1992. Codebase has been
released (cut-down versions) for other MUDs to use.

---

## Key Features

### Skill System
- **Hierarchical** — hundreds of skills, organized in a tree
- Skills are NOT individually documented — players discover what skill applies to what task
  by experimenting (e.g., "other.direction" is used for navigation AND map crafting AND
  reading)
- Use-based improvement — practice and use to level skills

### Death System
- **Circumstantial permanent death** — characters begin with 7 lives
- Lives can be replaced in-game
- Die with 0 lives remaining → character cannot be revived
- PvP deaths do NOT reduce your life count (same effects otherwise)

### Persistence Model
- Majority of game world is NOT persistent (areas and objects reset invisibly)
- Persistent: players' inventories, contents of rentable houses, and safe deposit box-like vaults

### Class System (or lack thereof)
- Originally designed to move away from a restrictive class system
- Any player can do (almost) anything with enough effort and advancement
- Guilds exist: Assassins, Priests, Thieves, Warriors, Witches, Wizards, Fools

### Player Governance
- Cities of Ankh-Morpork and Djelibeybi run by councils of elected player magistrates
- Ankh-Morpork: 7 magistrates; Klatch council: 5
- Elections held for these positions
- Player laws apply to city areas (beyond the game's acceptable use policy)

### Crafting
- Crafting and map-making exist — use the skill system (same skills used for multiple purposes)
- Agatean Tea Ceremony is a specific noted skill
- Weaving is a specific noted skill

### Other Systems
- Multiple guilds with unique abilities
- Quests
- Religion systems (joining gods' orders)
- Minigames

---

## Website Structure

The Discworld MUD website (discworld.imaginary.com) is well-organized with:
- Concepts documentation
- Command reference
- Essentials
- Rules
- Room help and item help
- Soul commands (emotes)
- Guild-specific pages (Assassins, Priests, Thieves, Warriors, Witches, Wizards, Fools)
- Atlases (maps)
- Clubs and families
- Creator Manuals (LPC for dummies, Being a Better Creator)

---

## Notable Design Choices

- Undocumented skills are intentional — discovery is part of the experience
- 7 lives system creates stakes without full permadeath
- Persistent inventory/vaults but non-persistent world rooms — hybrid model
- Classless-by-intent but guild-structured in practice
- Player-elected governance for major cities

---

## Further Research

Direct URL (note: robots.txt blocks automated scraping):
- https://discworld.imaginary.com/lpc/playing/documentation.c
- Discworld wiki: search "Discworld MUD wiki" for player-maintained documentation
