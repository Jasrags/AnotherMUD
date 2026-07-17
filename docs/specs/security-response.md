# Security Response (Heat)

> **Layer:** Action/interaction — a consequence engine over combat + the `security` zone property.
> **Status:** v1 — crime → heat → a timed patrol response that hunts the offender.
> **v2 (§8) SHIPPED** — crime kinds (lawful-victim weighting + a burned SIN as a crime), a
> **wanted level** with escalation waves, **de-escalation** (`wanted` / `bribe`), and heat
> **persistence** (save v38). Lawful-victim-tag exemption of self-defence and offline decay stay
> deferred; see §9.
>
> **Companion:** [sin-and-legality](sin-and-legality.md). That spec's §7.1 gates *access* by
> identity; this one gates *pursuit* by conduct. Together they are "the law reacts to you": a
> checkpoint decides whether you get in; heat decides whether the law comes after you — and a
> valid vs. burned/absent SIN is what lets the law *track* you (§4).

## 1. Overview

Cities in a cyberpunk setting are stratified by **enforcement**: a corp arcology answers a
gunshot in seconds; the barrens never answer at all. This system makes that real. Committing a
crime in a policed zone raises the offender's **heat**; when heat crosses the zone's threshold,
a **patrol response** is dispatched after a zone-scaled delay — law-enforcement mobs that spawn
and **hunt the offender specifically**. Heat **decays** over time, so lying low cools you down.

The engine already carries the inputs: an **area** `security` property (the `AAA…Z` tiers,
`sin-and-legality` neighborhood), combat **kill** events on the bus, a runtime **spawn** seam,
and the **grudge** mechanism that makes an otherwise-neutral mob pursue+engage one player. This
system is the loop that ties them together. It adds **no** persisted state (heat is runtime; a
relog or a server restart wipes it — "you went to ground").

### Goals

- A believable enforcement gradient: violence downtown is answered fast and hard; in the
  barrens the law does not come.
- Reuse the grudge/pursuit substrate so the response *finds and fights* the criminal, not the
  room.
- Tie the SIN axis in: a tracked identity means the law finds your *current* position; off the
  grid, you can lose them by moving.

### Non-goals (v1)

- **Non-kill crimes.** v1's trigger is a kill (see §2). Provoking hostility, theft, trespass, a
  burned SIN at a scan — all deferred (§7).
- **Escalation waves.** One response per heat crossing. A heat that keeps climbing bringing
  successively bigger waves is deferred.
- **Heat persistence.** Runtime only — no save-version bump.
- **Bystander / faction nuance.** Every kill in a policed zone is a crime regardless of who
  started it or who died (§2). Lawful-vs-hostile victim discrimination is deferred.

## 2. Crime: what raises heat

The v1 trigger is a **kill committed by a player in a policed zone**. On each kill:

- The offender is the **responsible player** for the killing blow (the killer, or the owner of a
  killing hireling — the engine already resolves this for loot/XP).
- The zone tier is the **area** `security` value of the room the kill happened in
  (`AAA > AA > A > B > C > D > Z`). An unset / unrecognized tier, or **Z**, is **unpoliced** —
  no heat, no response. Crime in the barrens is just Tuesday.
- Heat added is the tier's **heat-per-crime** (higher tiers react to less). Heat accumulates per
  player across kills and rooms until it decays.

The rule does **not** discriminate on who started the fight or whether the victim was hostile:
in a policed zone, discharging lethal force is the crime. (Flavor: the corps don't ask who drew
first. Fairer lawful-victim-only models are a deferred refinement, §7.)

### Acceptance criteria

- [ ] A player kill in a room whose area tier is policed (not Z / unset) adds that tier's heat.
- [ ] A kill in a **Z** / untiered area adds no heat and never schedules a response.
- [ ] Heat is attributed to the **responsible player** (killer or hireling owner), keyed by the
      same player id the grudge/targeting system uses.
- [ ] A mob-on-mob kill (no responsible player) raises no heat.

## 3. Heat, decay, and the response schedule

Heat is a per-player integer, held in memory. A recurring **sweep** (a scheduler handler, the
corpse-decay pattern) does two things each pass:

1. **Decays** every player's heat by a fixed amount, dropping the entry at zero.
2. **Fires** any response whose scheduled tick has arrived (§4).

When a kill pushes a player's heat to **at or above the zone's threshold**, a single response is
**scheduled** to fire after that tier's **delay** (short at high tiers, long at low). Only **one
response is pending per player** at a time — further crimes while a response is already scheduled
raise heat (and can re-arm after it fires) but do not stack simultaneous responses.

### Acceptance criteria

- [ ] Heat decays over time; a player who commits no further crime returns to zero and is
      forgotten.
- [ ] Crossing the tier threshold schedules exactly one response, after the tier's delay.
- [ ] A second crime while a response is already pending does not schedule a second concurrent
      response.
- [ ] The delay and threshold both scale by tier (higher tier = lower threshold, shorter delay).

## 4. The response: a patrol that hunts you

When a scheduled response fires, the system spawns the zone's **responder** mob (the tier sets
**how many**) into a room and stamps each with a **grudge** against the offender, so on the next
AI tick they path toward and engage that player specifically — the same mechanism a shot mob
uses to chase its shooter.

**Where they spawn is the SIN tie-in (`sin-and-legality` §7.1, the identity axis):**

- If the offender is carrying a **valid (unburned) credential**, the law reads them off the grid
  and the responders spawn in the offender's **current room** — wherever they've moved to.
- If the offender is **SINless** (no credential) or **burned**, the law has no live fix: the
  responders spawn at the **crime scene** (the room of the triggering kill). If the offender has
  moved on, the pursuit begins where they no longer are — they've slipped the net. Being off the
  grid is what lets you *run*.

On firing, the offender's heat is spent (reset), so a fresh crime spree must re-earn it.

### Acceptance criteria

- [ ] A fired response spawns the configured responder mob, count per the tier, and each carries
      a grudge against the offender (they pursue+engage that player).
- [ ] A carrying-a-valid-SIN offender is hunted at their **current** room; a SINless / burned
      offender is hunted at the **crime-scene** room.
- [ ] If the offender is offline / unplaced at fire time, the response degrades safely (spawn at
      the crime scene, or skip — never panic).
- [ ] Firing a response resets the offender's heat.

## 5. Configuration surface

| Key | Where | Default | Meaning |
|---|---|---|---|
| `security` | **area** property | *(unset ⇒ unpoliced)* | The enforcement tier `AAA…Z`; `Z`/unset = no law. |
| responder template | config | a law mob (e.g. `knight-errant-officer`) | The mob spawned to hunt the offender. |
| per-tier policy | config | table below | heat-per-crime, threshold, delay, responder count, per tier. |
| decay per sweep | env | tuned | How fast heat cools. |
| sweep cadence | env | tuned | How often decay + response-fire run. |
| master enable | env | on | A kill-switch for the whole system. |
| crime-kind weights | config | violence 0.25, burn 0.5 (murder 1.0) | Per-kind multiplier on the tier base (§8). |
| responder cap | config | 6 | Max size of an escalated wave (§8). |
| wanted decay | config | 30 sweeps | Sweeps a cooled offender's wanted level takes to drop one (§8). |
| bribe price | config | base 100 + 15/heat + 500/wanted | Nuyen a fixer charges to bury a record (§8). |

Indicative tier policy (tuned in config; the shape, not the numbers, is normative):

| Tier | heat/crime | threshold | delay | responders |
|---|---|---|---|---|
| AAA | high | low | very short | many |
| AA | high | low-mid | short | 2 |
| A | mid | mid | medium | 2 |
| B | low | high | long | 1 |
| C / D | low | high | very long | 1 |
| Z / unset | 0 | — | — | 0 (no response) |

No save-version bump — heat and pending responses are runtime only.

## 6. Ordering & failure

- Crime intake runs on the kill event (synchronous with the death handler). It only records
  heat + a schedule; it never spawns inline (spawning happens on the sweep, off the crime path).
- The response fire resolves the target room and spawns off the sweep handler. A missing
  player / room / template **logs and skips** — a broken response never aborts the sweep or
  panics (mirrors the per-item isolation in the decay sweeps).
- The grudge is single-target and single-room per mob (the existing retaliation limit); a
  response therefore hunts one offender. Multi-offender / multi-room pursuit is out of scope.

## 8. v2 additions (shipped)

- **Crime kinds (§2 generalized).** `OnKill` became `OnCrime(kind)`. Heat = the tier base ×
  a per-kind weight: **murder** (a lawful/civilian victim — a mob tagged `law` / `security` /
  `civilian`) = full; **violence** (a hostile/ganger kill) = a fraction; **burn** (a fake SIN
  caught at a scan) = a fraction. So killing a cop downtown brings the law hard, a ganger barely.
- **Burned SIN as a crime.** A `sin-and-legality` §7/§7.1 scan that burns a fake now feeds
  `OnCrime(burn)` — the tightest tie between the identity and heat systems: getting caught with
  a fake *is* a crime.
- **Wanted level + escalation.** Each response an offender provokes raises a per-player **wanted
  level**; the next wave grows with it (bounded by a responder cap). The level fades slowly once
  the offender has cooled off (no heat, no pending response).
- **De-escalation.** `wanted` (alias `heat`) shows the offender their heat band + wanted level;
  `bribe <fixer>` pays nuyen (scaled by heat + wanted) to a `fixer`-tagged mob to wipe heat and
  ease the wanted level.
- **Persistence (save v38).** `Save.Heat` + `Save.WantedLevel` snapshot the live tracker in
  `Persist` and re-seed it at login, so a relog no longer launders the law's attention.

## 9. Open questions / deferred

- **Lawful-victim exemption of self-defence.** v2 *weights* by victim (a ganger kill is lesser
  violence) but still charges some heat. A stricter model makes self-defence against a hostile
  entirely free; the `mob.aggro`-provoked-me distinction it needs is unbuilt.
- **More non-kill crimes.** Provoking hostility (`mob.aggro`), opening combat (`OnEngagement`),
  and theft as heat sources — v2 shipped only the burned-SIN non-kill crime.
- **Offline decay.** Persisted heat resumes at its logged-out value and decays live; it does not
  decay while offline (would need a persisted wall-clock stamp).
- **Alternative SIN models** (from `sin-and-legality` §8): SINless-draws-more-heat, or no
  identity interaction. v1/v2 ship the **evade-the-hunt** model (§4).
- **Faction pull / Lone Star contact** as an alternative de-escalation to a cash bribe.
