# AwakeMUD — Building & OLC Overview

Source:
- https://github.com/luciensadi/AwakeMUD/wiki/OLC-Overview-and-FAQs
- https://github.com/luciensadi/AwakeMUD/wiki/The-Definitive-Builder%27s-Guide
- https://github.com/luciensadi/AwakeMUD/wiki/Welcome-%26-Tips
- https://github.com/luciensadi/AwakeMUD/wiki/Getting-Started
- https://github.com/luciensadi/AwakeMUD/wiki/Helpfile-Editing
- https://github.com/luciensadi/AwakeMUD/wiki/Style-Guide
Fetched: 2026-06-18

---

## What is OLC?

**OLC** stands for "On-Line Creation" — the system used for modifying the AwakeMUD
game world. It uses a menu-based interface designed to be intuitive. AwakeMUD CE is
a CircleMUD/DikuMUD-lineage codebase; the builder docs reference the CircleMUD Admin
Guide and the Bug City "Definitive Builder's Guide" as ancestry.

Prospective builders must contact the head builder (Vile) to gain access; the team
is volunteer-run so response times vary. There is a Discord: https://discord.gg/bKBpvNj

---

## Key terminology

| Term | Meaning |
|---|---|
| **Vnum** | A positive integer identifying an in-game entity (room, object, mob, vehicle). The same vnum can refer to different entity *types* (a room and an object may both be vnum 12345). |
| **Zone** | A collection of 100 vnums by default (0–99), numbered as `zone number × 100`. So zone 74 covers 7400–7499 (top-of-zone can extend it, e.g. 7400–7699). A zone holds reset logic, jurisdiction info, security level, and the authorized builder IDs. |
| **Zone commands** | Reset instructions: NPC placement, door states, vehicle/item spawning on a schedule. See `room-and-zone-editing.md`. |

---

## The OLC editors

| Command | Edits | Notes |
|---|---|---|
| `redit` | Rooms | `redit` edits current room; `redit <vnum>` edits/creates by vnum |
| `zedit` | Zones | reset behavior, security, jurisdiction, builders |
| `iedit` | Items / objects | (object editor; same role as oedit elsewhere) |
| `medit` | NPCs / mobiles | |
| `sedit` | Shops | |
| `qedit` | Quests | |
| `hedit` | Matrix hosts | |
| `helpedit` | Helpfiles | |

Enabling/disabling: the global `OLC` command (Level 7+) toggles the whole OLC system on
or off. Per-builder access is granted with `SET <target> OLC ON`.

---

## Becoming / setting up a builder (game-owner workflow)

From *Getting Started* — how an admin onboards a new builder:

1. **Elevate staff level:** `ADVANCE <target> 2` (or level 4 for new builders).
2. **Enable OLC permissions:** `SET <target> OLC ON` to grant building access.

Assigning a builder to a zone:

1. Retrieve the builder's IDNum with `STAT <target>`.
2. View available zones with `SHOW ZONES` to find an open slot.
3. `ZSWITCH <zone number>` to target the zone (e.g. `ZSWITCH 98` for vnums 9800–9899).
4. Enter `ZEDIT` and answer the creation prompt affirmatively.
5. Add the builder's IDNum to the **Editor IDNums** list.
6. Finish zone configuration, then save and exit.

**Troubleshooting:**
- *"Privileged operation"* error → OLC permissions not enabled; an Immortal must run `SET <name> OLC ON`.
- *"OLC temporarily unavailable"* error → OLC globally disabled; an Immortal (Level 7+) runs the `OLC` command to re-enable.

**Recommendation:** for live servers with mortal players, run a **separate builder
instance** so modifications stay hidden from regular gameplay and builder powers can't
be abused in the live game.

---

## Loading created content for testing

| Entity | Temporary load | Permanent load |
|---|---|---|
| **Rooms** | available immediately after editing; reach via `goto <vnum>` | — |
| **Objects** | `iload <vnum>` or `wizload object <vnum>` | requires zone commands |
| **Mobs** | `wizload mob <vnum>` | requires zone commands |
| **Vehicles** | `wizload veh <vnum>` (no owner), then `vset <vehicle> owner <ID>` (find ID via `stat self`) | wizload in vehicle-shop storerooms (10022 or 1398), then buy at shops (10021 or 1399) |

**Testing guidance:** test with a non-staff mortal character — staff characters have
perks that interfere with accurate testing. Keep a staff character online to move the
test character with `transfer` / `teleport`.

---

## Submission / audit process

1. Run `audit` on your zone to catch common issues.
2. Do a manual walkthrough for spelling and content review.
3. Run `audit submit` to **lock** the zone and notify staff for review.

---

## Tips for a successful build (from *Welcome & Tips*)

- **Theme & minimum viable area** — pick a clear theme ("jungle with predators",
  "abandoned factory") and identify the minimum rooms/objects to make it enjoyable
  before expanding.
- **Map your zone first** — plan layout on paper / doc / spreadsheet (room connections
  + basic descriptions) before building.
- **Consolidate vnums** — minimize rooms/objects. Convey length through *description*
  rather than 20 winding-path rooms. Start with an MVP, expand later.
- **Audit frequently** — `AUDIT` catches common issues early; `AUDIT SUBMIT` when done.
- **Use canon & templates** — reference existing canon items and template mobs from
  designated zones; you can **clone** items into your zone if you need to modify them.

---

## Style guide (description writing standards)

- **Description length:** aim for **3–6 lines**. Shorter feels sparse; longer overwhelms.
  Expand room detail via interactive decoration objects / extra descriptions instead of
  long text.
- **Color:** "Avoid the use of color in look descriptions." Color is permitted in names
  and room descriptions but used with restraint — neutral tones predominate, color for
  emphasis only.
- **Highlighting syntax:** use `##^W` before highlighted text and `^n` to close.
  Example: `To continue, head ##^WNORTH^n.` The `##` disappears for standard users but
  renders as a single `#` for screenreaders.
- **Extra descriptions (exdescs):** make them discoverable — highlight keywords in the
  base description, or give explicit instructions for accessing them (in books / objects).
- **Swearing:** use Shadowrun slang, not real-world swears — e.g. *frag, drek, hoop,
  slitch*. (Reference the SR wiki slang list.)
- **Capitalization & articles:** lowercase `a`/`an` in item names, no trailing period or
  extra spaces. Example: `a silver-plated ash tray` (not `A silver-plated ash tray!`).
- **Repetition:** avoid repeating content across zones unless deliberate narrative
  emphasis. Vary room names, mobs, and objects so the zone doesn't look copy-pasted.

---

## Helpfile editing (`helpedit`)

Command forms: `HELPEDIT [exact and full title]`, `HELPEDIT DEPRECIATED`, `HELPEDIT NEW`.

**Naming conventions:**
- Pluralize and use gerund forms so players searching variant word forms find the entry.
- For long titles or titles with substantial overlap, put the primary word in quotes —
  e.g. `"PUYALLUP EAST"` and `"PUYALLUP WEST"` as separate entries from the main
  `PUYALLUP` entry.
- Searching the full exact name returns only that specific helpfile regardless of shared
  terms.

**Formatting:**
- Helpfiles accept standard MUD formatting.
- Use `^n` to force new lines if your client strips empty lines.
- The percent symbol `%` **cannot** be used — it always inserts a slash.
- A "See Also" section commonly uses `^W` for white text and `^n` for line breaks.

**Deletion:** helpfiles cannot be deleted; rename obsolete entries to `DEPRECIATED`.

---

## Design notes

- The Definitive Builder's Guide and Staff Documentation pages are **index/navigation
  hubs** — their substantive content lives in the individual editor pages captured across
  these research files (OLC overview, room/zone/item/mob/shop/quest/matrix editing, flag
  references, style guide).
- Before creating a new item, **search existing GitHub code** for similar items
  (especially cyberware) to avoid duplication / "item bloat."
