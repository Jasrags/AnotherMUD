# Shadowrun Starter Area — design

> **Status: design draft (2026-07-13).** Not built. Scopes the player-facing
> onboarding front door for the `shadowrun` world and the first proper street-doc
> in Seattle. Sits beside [`shadowrun-mvp.md`](shadowrun-mvp.md) (the MVP build
> plan) and [`shadowrun-pack-plan.md`](shadowrun-pack-plan.md) (the full analysis).
> Related: the scoped onboarding-guide (`onboarding-guide-scoping` memory), the
> shipped karma-as-XP advancement, and the SR-M4 Essence/cyberware slice
> (`sr-m4-essence-build-log` memory).

## 1. Overview

Today a new Shadowrun character spawns straight into **Downtown**
(`shadowrun:westlake-plaza`, the `make run-shadowrun` start) — a fixer sells gear
in the room, but there is no onboarding beat: no mentor, no "here's how the sprawl
works," no safe place to find your feet before the streets bite. The nearest
cyberware (the existing street-doc in Loveland) is a four-room walk into the
Puyallup Barrens.

The **starter area** is a purpose-built onboarding zone that becomes the real
front door: a new runner spawns into a safe fixer's safehouse, meets a mentor,
gears up (weapons / armor / ammo / starter chrome), learns the core verbs, and
then **graduates** onto the street — into Downtown, the existing hub. It is
distinct from two things it is easy to confuse with:

- **`street-corner`** — the SR-M3c **smoke-test harness** (an intentionally
  isolated 4-room island the live tests pin via `START_ROOM=street-corner`).
  Stays test-only; the starter area does not touch it.
- **`westlake-plaza`** — today's default start, tomorrow's **Downtown hub** you
  graduate *into*. Stays the reachable heart of the city.

### Goals

- A **safe, self-contained** first experience: no hostiles, everything reachable
  on foot, a clear beginning (arrive) and end (graduate to the street).
- A **mentor** who greets you, teaches the core verbs, and points at the services
  — the already-scoped onboarding-guide NPC.
- **First commerce** in one place: a fixer's gear table (weapons/armor/ammo) and a
  back-room chop-doc (starter chrome), reusing the existing shop system.
- A clean **graduation** into Seattle proper that also starts the guide trailing
  you (onboarding-guide slice 1), so the mentor persists across the transition.

### Non-goals (v1)

- Full SR **priority/karma character generation in-world** — core chargen stays
  the pre-game wizard (race/class/background). See §6.
- A **qualities** system (positive/negative traits) — greenfield SR mechanic, its
  own future spec, not part of onboarding.
- The Matrix / astral / SIN gating / drones — unrelated arcs.

## 2. Narrative frame

A fixer you've done a favor for stakes you to a **flophouse / coffin-motel squat**
— a neutral safe space off a Downtown alley. He's your first contact: he explains
the shadows, fronts you the basics, and knows a doc who works cheap. This frame
justifies every element (mentor, gear, a back-room ripperdoc) in one coherent
place, and the "step out the door onto the street" graduation is literal.

Alternatives considered (see Open Questions §9-Q1): the **Ork Underground**
entrance, **Dante's Inferno** back room (the iconic runner bar), a **Puyallup
squat**. The Downtown fixer's safehouse wins on: adjacency to the existing hub
(graduation is one exit), safe-zone plausibility, and the fixer doubling as the
guide.

## 3. Layout (v1)

A small cluster (3–4 rooms), all `safe`/`safe-room` tagged, no hostiles:

```
  [ the flop ]  ← spawn. The guide/fixer here. MOTD + first tips.
       │ (door / stairs)
  [ the fixer's table ]  ── gear shop (weapons/armor/ammo)
       │
  [ the back room ]  ── chop-doc: starter chrome (a tutorial ripperdoc)
       │ "out to the street"
  ===> shadowrun:westlake-plaza  (Downtown — graduation target)
```

- Ids namespaced under a new area (e.g. `shadowrun:the-flop`, area
  `seattle-safehouse` or folded into `seattle-downtown` — Q4). The graduation exit
  targets the existing `shadowrun:westlake-plaza`; add the return exit too so a
  graduated runner can revisit.

## 4. The NPCs

- **The mentor / guide (the fixer).** Reuses the **onboarding-guide** design
  (`onboarding-guide-scoping` memory — scoped, not yet built): a stationary
  mentor in the flop you `ask` for orientation, who on graduation converts to the
  **trailing guide** (slice 1: appears at creation/login under a level cap, trails
  the newbie on move, gives situational tips, departs at the cap). Building the
  onboarding-guide is therefore a **dependency** of the full starter-area
  experience; the zone can ship first with a stationary-only mentor and gain the
  trailing behavior when the guide slice lands.
- **Gear vendor.** Either the fixer himself or a separate armorer; sells
  starter-tier weapons / armor / ammo. Mirror the existing `fixer` mob's `shop`
  block (`content/shadowrun/mobs/fixer.yaml`) — no new shop code.
- **Chop-doc (tutorial ripperdoc).** A lightweight back-room doc selling the three
  starter implants (wired reflexes / muscle replacement / cybereyes), reusing the
  `street-doc` shop model. Distinct from the **first real street-doc** (§7). Q3:
  whether the starter zone even needs its own doc, or points you at the real one.

## 5. Commerce & economy

- Starting nuyen is the street-kid background's **500¥**. That covers ammo + a
  cheap weapon; the armored jacket and *all* chrome (cheapest = cybereyes 4,000¥)
  are a **payday away** — deliberately aspirational, SR-authentic ("chrome is a
  goal, not a handout"). The starter area shows you the ripperdoc's stock so the
  *want* is seeded early; the *buy* comes after your first runs.
- Reuses the shop service wholesale (economy-survival §3), the nuyen currency
  label (already reskinned), and SR-M4 Essence (installing bought chrome spends
  the budget, `score` shows it). No new economy code.

## 6. Character-build finalization — scoped, mostly deferred

The ask floated "player creation items like skills, attributes, qualities." Today
character generation is the **pre-game wizard** (account → roster → create → race /
class / background), and karma is the **advancement** currency (karma-as-XP,
shipped). Building an in-world point-buy chargen is a large greenfield system; the
recommendation is to **keep core chargen in the wizard** and phase in-world
build-spending as later slices, NOT v1:

- **v1:** no in-world chargen. The starter area's "build" contribution is **gear**
  (shops) — the choices that matter at level 1.
- **Later — a karma trainer.** An NPC in the starter area (or a training hall) that
  spends **karma** on skills/proficiencies, riding the existing karma-as-XP +
  use-based proficiency systems. A natural, contained slice once desired.
- **Later — qualities.** A positive/negative-trait system (SR "qualities") is its
  own greenfield spec (creation-time picks + mechanical hooks). Onboarding is a
  *consumer* of it, not the place to build it.

## 7. The first real street-doc (Seattle proper)

Separate, concrete follow-on deliverable. The existing `street-doc` (Loveland /
Touristville) is **minimal** — the mob dropped into a generic room. The first
*real* street-doc is a **fleshed-out ripperdoc clinic**: a dedicated room (the
"chrome den" — a back-alley clinic in the **Puyallup or Redmond Barrens**, the
classic SR home for cheap deniable chrome), an atmospheric description, a
better/deeper chrome stock (and higher prices — the real doc, not the tutorial
chop-shop), and a disposition/faction posture that fits the Barrens. This is the
destination the starter-area chop-doc *points at*: "want real chrome? see a doc in
the Barrens." Build order: the clinic can land **independently** of (and before)
the full starter area — it only needs a room + a richer `street-doc` variant, all
existing systems.

## 8. Relationship to existing systems

| System | Role |
|---|---|
| Default start | Repoint `make run-shadowrun` + the pack start room from `westlake-plaza` → the flop. `westlake-plaza` stays the graduation target. `street-corner` stays test-only. |
| Onboarding-guide (`onboarding-guide-scoping`) | The mentor. A **dependency** for the trailing behavior; the zone ships stationary-first. |
| Shop service (economy-survival §3) | Gear vendor + chop-doc, reused verbatim. |
| SR-M4 Essence / cyberware | Bought chrome installs and spends Essence; `score` shows it. |
| Karma-as-XP advancement | The (later) karma trainer's currency. |
| Faction / safe-room | The zone is a safe, hostile-free faction-neutral space. |
| Geography (`sr2075_geography_mud.md`) | The clinic lands in the Puyallup/Redmond Barrens per the gazetteer's security-zone map. |

## 9. Decisions (resolved 2026-07-13)

- **Q1 — Where does the starter area physically sit?** **→ a Downtown fixer's
  safehouse**, one exit from the existing hub (`westlake-plaza`). The fixer doubles
  as the mentor; graduation is a single step onto the street.
- **Q2 — In-world chargen scope for v1?** **→ none.** Core chargen stays the
  pre-game wizard; gear is the only build choice at level 1. The karma-skill trainer
  and the qualities system are explicit **later slices** (§6), not v1.
- **Q3 — Own chop-doc, or point at the real one?** **→ both.** A lightweight
  tutorial chop-doc in the safehouse seeds the want; the fleshed-out Barrens clinic
  (§7) fulfills it.
- **Q4 — New area, or fold into `seattle-downtown`?** **→ a dedicated area**
  (`seattle-safehouse`) so the zone reads as its own place and graduation to Downtown
  is an explicit area transition.
- **Q5 — Guide: stationary, trailing, or both?** **→ both.** Stationary mentor in
  the zone now; converts to the trailing guide on graduation once the
  onboarding-guide slice lands (`onboarding-guide-scoping`).
- **Q6 — Default start now, or opt-in until the guide ships?** **→ repoint now.**
  `make run-shadowrun` + the pack start move to the safehouse once the rooms + shops
  + stationary mentor exist; the trailing guide upgrades it in place.

## 10. Build order

1. **First real street-doc** (§7) — ✅ **SHIPPED 2026-07-13.** Scalpel's **Chrome
   Den** (`shadowrun:chrome-den`), a basement ripperdoc clinic down a ladder behind
   Hell's Kitchen in the Puyallup Barrens, reachable on foot from the Downtown start.
   A named ripperdoc (`ripperdoc` = "Scalpel") with deep chrome stock — the existing
   three implants plus three new ones (`dermal-plating`, `reaction-enhancers`,
   `cyberarm`) so the real doc out-stocks the tutorial chop-doc. Bought chrome
   installs + spends Essence via SR-M4. Tests: `pack.TestShadowrun_RipperdocReachableFromStart`,
   live `TestLive_ShadowrunRipperdocClinic`.
2. **Starter-area rooms + shops + stationary mentor** (§3–§5) — ✅ **SHIPPED
   2026-07-13.** A dedicated `seattle-safehouse` area of three safe rooms:
   `the-flop` (spawn; the mentor **Rook** = mob `fixer-mentor`, stationary,
   `ask rook about <topic>` orientation) → `the-fixers-table` (east; the existing
   `fixer` gear shop) → `the-back-room` (west; the existing `street-doc` as the
   tutorial chop-doc). Graduation is a two-way stairwell: `the-flop` down ↔
   `westlake-plaza` up. The default start (`make run-shadowrun` / `watch-shadowrun`)
   is repointed to `shadowrun:the-flop`. Tests: `pack.TestShadowrun_StarterAreaReachability`,
   live `TestLive_ShadowrunSafehouse`. *(Trailing-guide behavior is step 3.)*
3. **Onboarding-guide slice 1** (the trailing mentor) — ✅ **SHIPPED 2026-07-13.**
   A generic, opt-in trailing-guide engine (`docs/specs/onboarding-guide.md`): a
   friendly NPC materializes at a new character's side on world-entry, trails them
   on every move, and departs at a graduation level (`ANOTHERMUD_GUIDE_LEVEL_CAP`,
   default 3), with a `shoo` verb to send it off. Shadowrun wires **Patch** (mob
   `street-guide`) via `ANOTHERMUD_GUIDE_TEMPLATE=shadowrun:street-guide` (the make
   targets). A simplified mirror of the hireling machinery (own `IsGuide` marker,
   `liveGuide` overlay, `guideService`, Manager-driven spawn/trail/graduate/drain).
   Tests: `session.TestGuideOverlay_*`, live `TestLive_ShadowrunGuide` +
   `TestLive_ShadowrunGuideGraduates`. *(Slice 2 — situational tips — is deferred
   in the spec's Open Questions.)*
4. **Later:** karma trainer (§6); qualities system (own spec); guide situational
   tips (onboarding-guide slice 2).
