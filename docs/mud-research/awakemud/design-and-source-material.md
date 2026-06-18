# AwakeMUD — Design & Source Material

Source:
- https://github.com/luciensadi/AwakeMUD/wiki/Thematic-Elements-of-CE
- https://github.com/luciensadi/AwakeMUD/wiki/Source-Material
- https://github.com/luciensadi/AwakeMUD/wiki/Staff-Commands
- https://github.com/luciensadi/AwakeMUD/wiki/Staff-Documentation
- https://github.com/luciensadi/AwakeMUD/wiki/The-House-File
Fetched: 2026-06-18

---

## Setting & theme ("CE")

AwakeMUD CE is set in the **Shadowrun** universe — a mashup of traditional cyberpunk and
high fantasy magic. Verbatim from the wiki:

> "Shadowrun is a mashup of traditional cyberpunk and high fantasy magic, with space for
> both mirrorshades (gray, gritty, realistic) and pink-mohawk (bright, surreal, wacky)
> elements. Mix in a bit of corporate espionage, hyper-violence, and slapstick and you've
> got Shadowrun in a nutshell."

The setting deliberately accommodates two tonal poles:

- **Mirrorshade / grounded** — gray, gritty, realistic gameplay.
- **Pink-mohawk / surreal** — bright, wacky, absurdist elements.

Core narrative threads: corporate intrigue, intense action, and dark comedy.

> Note: the wiki page titled "Thematic Elements of CE" does not explicitly expand the "CE"
> abbreviation. Throughout the docs the game is referred to as "AwakeMUD CE" / "Awake CE."

---

## Source material (rules & lore)

- **Awake CE uses Shadowrun 3rd Edition (SR3) rules and lore.** It tries to stay close to
  source material where possible, but some discrepancies exist; the team invites feedback
  via Discord on canon adherence.
- The Source-Material wiki index points to three companion pages: **Theme**,
  **Corporations**, and an **Equipment List**.
- Specific SR3 references surface in the editor docs rather than a single bibliography:
  - **Magic in the Shadows** — cited for room background-count spellcasting TN rules
    (p. 83), referenced by the `redit` "Background Count" field.
  - **Shadowrun 3rd Edition Core rulebook** — cited for Matrix host security color /
    rating / intrusion difficulty (`hedit`).
  - Shadowrun slang (frag, drek, hoop, slitch, etc.) is mandated over real-world swearing
    in the Style Guide.

The wiki does not publish a single consolidated rulebook list; for the full inventory of
SR3 supplements used, the maintainers point to the linked pages and the Discord community.

---

## Staff structure & governance

The Staff-Documentation and Staff-Commands pages are largely index/navigation hubs; the
substantive content is in the linked sub-pages. What is captured:

- **Staff levels gate capability.** Examples from the docs:
  - Builders are typically advanced to **level 2** (or **level 4** for new builders) and
    have OLC enabled (`SET <target> OLC ON`).
  - The global `OLC` toggle requires **Level 7+**.
  - Mob "PRIVATE" stat-hiding blocks Immortals **below level 4**.
  - Staff-level gating exists per-room (`redit` field `n`, "Staff Level Required").
- **Documentation lineage / attribution:** the staff command docs draw from the
  *CircleMUD Admin Guide*, *CircleMUD Wizard Commands*, *Awakenedworlds Staff
  Documentation*, and *"Bug City — The Definitive Builder's Guide."* This places AwakeMUD
  CE firmly in the CircleMUD/DikuMUD lineage.
- **Staff Documentation hub** references: Staff Commands, Staff Guides, The Definitive
  Builder's Guide (WIP), Manual Testing Guidelines, and Coder Tips.

### Staff command families (index)

Over 100 staff commands are catalogued. By family:

| Family | Commands |
|---|---|
| Movement / navigation | GOTO, TELEPORT, VTELEPORT, TRANS, AT, RETURN |
| Object management | LOAD, ILOAD, WIZLOAD, PURGE, ICLONE, MCLONE, RCLONE, ILIST, MLIST, RLIST, VLIST |
| Editing tools | IEDIT, MEDIT, REDIT, ZEDIT, HEDIT, HELPEDIT, QEDIT, OLC |
| Player management | SET, STAT, VSTAT, SHOW, RESTORE, UNAFFECT, POSSESS, SNOOP |
| Communication | ECHO, AECHO, GECHO, ZAECHO, PAGE, WTELL, SEND, FORCE |
| Moderation | MUTE, MUTEOOC, FREEZE, THAW, BAN, UNBAN, PENALIZE, PARDON, NOTITLE |
| Server operations | REBOOT, SHUTDOWN, COPYOVER, CRASHMUD, UPTIME |

(See `building-olc-overview.md` for builder onboarding using ADVANCE / SET OLC /
ZSWITCH / STAT, and the per-editor files for the editing commands.)

---

## Housing data: The House File

Housing/apartments are defined in a flat data file (`lib/etc/houses`) rather than via an
OLC editor — relevant as a world-design / persistence pattern.

**File structure:**
- First line: count of landlord entries.
- A landlord line followed by its room lines, repeated per landlord.
- Trailing blank line.

**Landlord line:**
`<landlord mob vnum> <race bitfield (unused — set 0)> <base cost in nuyen> <num rooms>`

**Room line:**
`<room vnum> <key vnum> <direction of exit to atrium> <lifestyle> <apt name> <PC owner idnum (0)> 0 <paid until (0)>`

**Lifestyle values:** `0` = Low, `1` = Middle, `2` = High, `3` = Luxury.

**Direction values:** `0` N, `1` NE, `2` E, `3` SE, `4` S, `5` SW, `6` W, `7` NW.

---

## Design takeaways relevant to AnotherMUD

- **Vnum-namespaced, zone-bucketed world** (100-vnum zones, `zone × 100` base) with
  builder-ID access lists and per-zone reset modes — a different model from AnotherMUD's
  namespaced pack ids, but the same "authoring + reset + access-gating" concerns.
- **Reset-as-data zone commands** (typed M/O/E/G/P/D/etc. with global-vs-room spawn
  counts and IF-LAST conditionals) parallel AnotherMUD's spawn tables.
- **Setting-agnostic vs. setting-baked:** AwakeMUD bakes SR3 deeply into its editors
  (background counts, Matrix security colors, race-grouped aggression flags), whereas
  AnotherMUD keeps specs setting-agnostic and pushes flavor into content packs.
- **Tonal range as explicit design policy** (mirrorshade ↔ pink-mohawk) is a useful model
  for documenting a world's intended tone band.
