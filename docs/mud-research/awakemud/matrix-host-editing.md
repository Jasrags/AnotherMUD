# AwakeMUD — Matrix Host Editing

Source: https://github.com/luciensadi/AwakeMUD/wiki/Matrix-Host-Editing
Fetched: 2026-06-18

---

## Overview

The `hedit` command creates and modifies **Matrix hosts** through the OLC system.
Matrix hosts model the Shadowrun cyberspace network that deckers connect to and run
against. Mechanics follow the Shadowrun 3rd Edition Core rulebook.

Syntax: `hedit <host_number>` — the number references the host's vnum.

A useful external reference cited by the wiki: a Matrix host generator at
`shadowrun.plasticwarriors.org`, plus the SR3 source material for technical specifics.

---

## Host hierarchy

Hosts follow a tiered structure:

- **RTG** — Regional Telecom Grids (top of the hierarchy)
- **LTG** — Local Telecom Grids (below RTGs)
- **PLTG** — Private LTGs (attach to either)

Currently LTG, RTG, and PLTG are implemented; other types exist for identification only.

---

## Main menu fields

| # | Field | Meaning |
|---|---|---|
| 1 | **Host** | The host's title / name |
| 2 | **Parent Host** | Hierarchical connection to the parent system (RTG → LTG → PLTG) |
| 3 | **Keywords** | Connection access terms. At least one keyword should be reflected in the description of each host that allows connections to this one. |
| 4 | **Description** | Host appearance text. Should include, in prose, information about which hosts can be accessed from here. |
| 5 | **Security** | Security color, numerical rating, and intrusion difficulty (per SR3 Core) |
| 6 | **Type** | Host category (LTG / RTG / PLTG implemented; others for identification) |
| 7 | **Ratings** | Subsystem ratings — each acts as the target number for a specific class of unauthorized action |
| 8 | **Subsystem Extras** | Scramble ratings and trapdoors (secret passages to other hosts via vnum) |
| 9 | **Trigger Steps** | Listed as TBD (to be determined) |
| 0 | **Exits** | Add / delete / view exits. Requires a target host vnum and an 8-digit phone number (format `XXXX-XXXX`). |
| A | **Shutdown Start Text** | Message displayed when a shutdown initiates |
| B | **Shutdown Stop Text** | Message displayed when a shutdown aborts |

---

## Related: room jackpoints

Rooms connect into the Matrix via a **jackpoint** (`redit` field `g` — "Edit Jackpoint"):
To Host (vnum), I/O Speed (0 = unlimited; -1 = capped at MPCP × 50), Commlink Number
(caller ID), and Physical Address (location string). See `room-and-zone-editing.md`.

A zone command of type **D (HOST)** loads an object into a Matrix host (see
`room-and-zone-editing.md` zone-commands table).
