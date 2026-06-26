# Hireable Mobs — Feature Specification

**Status:** Draft (spec; **slices 1–2 shipped** — the owned-companion substrate +
`hire`/`dismiss`/`hirelings` + persistence/logout/login (slice 1), and the bound
move-with-owner relocate (slice 2); combat assist + upkeep are later slices) · **Scope:** NPCs a character hires to
follow, fight for, and obey them (mercenaries, henchmen, hirelings): the
owner/controller relationship, the `hire` / `dismiss` / `order` verbs, a hireling
trailing its owner, combat assistance with owner-routed loot, a recurring upkeep
gold sink, and durable ownership across logout · **Audience:** Anyone
reimplementing or porting this feature in any language.

This document describes *what* the feature must do, not *how*. Specific prices,
upkeep cadences, caps, and XP rules are policy that lives in configuration or
content (see §10). The design pass and resolved pre-decisions behind this spec
live in [`docs/proposals/hireable-mobs.md`](../proposals/hireable-mobs.md).

A hireling is **greenfield** but adds little new machinery: it is an **owned,
world-resident creature** — structurally the same as a [mount](mounts.md) — that
*fights* rather than *carries*. It reuses the owner relationship, materialize /
dematerialize, the owned-record persistence, and the logout teardown built for
mounts; the one genuinely new piece is an **AI behavior** that makes the hireling
trail and defend its owner. This feature layers on
[mobs-ai-spawning](mobs-ai-spawning.md) (the hireling is an owned creature with a
behavior), [follow](follow.md) (the move-with-leader mechanic; this consumer
brings the mob-move signal follow deferred), [combat](combat.md) +
[grouping](grouping.md) (kill credit, the corpse owner-set, auto-assist),
[loot-and-corpses](loot-and-corpses.md) (loot rights), and
[economy-survival](economy-survival.md) (hire cost + upkeep as gold sinks).

---

## 1. Overview

A **hireling** is an NPC a character **hires** to accompany them: it follows its
owner from room to room, fights what its owner fights, and is paid for with an
up-front cost and recurring upkeep. It is a world creature — it occupies a room,
can be looked at, can die — owned by exactly one character, and it is a **losable
asset**: lapsed upkeep or death ends the contract.

### Core concepts

- **Hireling** — an owned, world-resident creature with a controller (its
  **owner**), an AI behavior that keeps it near its owner and pulls it into the
  owner's fights, and a content-fixed combat profile. It is not an inventory item
  and is never carried or stored.
- **The hire contract** — the durable ownership link between a character and a
  hireling, persisted as character state (§9). The contract — not the live
  creature — is the durable thing.
- **Following** — a hireling automatically trails its owner room to room, reusing
  the [follow](follow.md) move-with-leader mechanic.
- **Assistance** — a hireling joins its owner's combat, contributing attacks; its
  kills route loot rights to the owner.
- **Upkeep** — the recurring gold sink that keeps a hireling; a lapse ends the
  contract.

### Goals

1. Let a character field a single capable companion that follows and fights for
   them, as a meaningful, **losable** investment with real gold sinks.
2. Reuse the owned-creature substrate (ownership, materialize/dematerialize,
   owned-record persistence, logout teardown) built for [mounts](mounts.md)
   rather than duplicating it.
3. Bring the **mob-move signal** [follow](follow.md) deferred, so a mob can be a
   move leader — the shared seam the onboarding guide and a future pack's
   summoned/controlled entities also need.
4. Reward the owner's **participation**, not their wallet (§6.4).
5. Never silently delete an owned asset (§9).

### Non-goals

- **A new movement, combat, or loot model.** Following reuses
  [follow](follow.md); fighting reuses [combat](combat.md); loot reuses the
  [corpse owner-set](loot-and-corpses.md). This spec adds an owner relationship
  and an AI behavior, not new primitives.
- **Mercenary bands.** v1 caps simultaneous hirelings small (§3.3); multiple
  hirelings, formations, and patrol routes are deferred (§11).
- **Hireling advancement or gear management.** A hireling is content-fixed (no
  XP, levels, or equipment slots in v1), like a mount.
- **Morale / loyalty / desertion** as a system. The only v1 "loyalty" lever is
  lapsed-upkeep-departs (§7).
- **Pack/companion control surfaces for other settings** (summoned spirits,
  slaved drones). Those are pack consumers built on this same owned-entity seam,
  not part of generic v1.

---

## 2. The hireling entity

A hireling is a **world-resident creature** with an **owner**: in addition to the
ordinary creature surface (a room, a name, hit points, the ability to be looked
at and to die), it carries:

- an **owner** — the character entitled to command it; a hireling has at most one
  owner, and only the owner may dismiss or order it;
- an **AI behavior** that keeps it near its owner and joins the owner's fights
  (§5, §6);
- a content-fixed **combat profile** (its stats, attacks, hit points), authored
  on its mob template.

A hireling is **not** an inventory item and is never carried, stacked, or stored.
It is recruited, dismissed, or (on the owner's logout) resolved to a resting
contract (§9).

**Acceptance criteria**

- [ ] A hireling has a single owner; a non-owner cannot dismiss or order it.
- [ ] A hireling is a world creature (look-able, hit-pointed, killable), never an
      inventory item.
- [ ] A hireling exposes its owner and a content-set combat profile.

---

## 3. Hiring and dismissing

### 3.1 Hiring

A character **hires** a hireling from a content-defined **recruiter** (a
mercenary-post / sellsword NPC) with the `hire` verb. Hiring MUST:

- be refused, with a clear message, when: the target is not a recruitable
  hireling, the actor is at their hireling **cap** (§3.3), the actor cannot
  afford the **hire cost**, or the actor is in a state the policy forbids;
- charge the **hire cost** (a gold sink, [economy-survival](economy-survival.md)
  §3) up front; an actor who cannot pay is refused with no state change;
- on success, establish the hire contract (§2), materialize the hireling into the
  owner's room, announce it to the room, and begin the follow/assist behavior
  (§5, §6).

### 3.2 Dismissing

The owner **dismisses** a hireling with the `dismiss` verb, ending the contract
and removing the hireling from the world. Dismissing is always available to the
owner. A dismissed hireling is gone — the gold spent is not refunded (refund
policy is §10).

### 3.3 The cap

A character may field at most a configured number of hirelings at once
(`Hireling cap`, §10; v1 default a small number — one henchman). A `hire` that
would exceed the cap is refused.

**Acceptance criteria**

- [x] `hire` is refused for a non-recruitable target, an at-cap actor, an actor
      who can't pay, or a forbidding state — each with a clear message and no
      gold charged on refusal. **SHIPPED (slice 1).**
- [x] A successful `hire` charges the hire cost, binds the hireling to the owner,
      places it in the owner's room, and announces it. **SHIPPED (slice 1).**
- [x] `dismiss` (owner only) ends the contract and removes the hireling from the
      world. **SHIPPED (slice 1).**
- [x] An owner at the hireling cap cannot hire another until one is dismissed,
      dies, or its upkeep lapses. **SHIPPED (slice 1** — `ANOTHERMUD_HIRELING_CAP`,
      default 1**).**

> **Slice 1 acquisition (model b):** `hire <name>` resolves the hireable template
> by name among **all** mob templates carrying a `hireling:` block (no recruiter
> access point in v1 — the recruiter gate is a later UX slice). The §3.1 "recruiter"
> framing is the eventual model; slice 1 lets a player hire from anywhere.

---

## 4. The hireling in the world

At any moment an owned hireling is in exactly one of these states:

- **Active** — materialized in the world, following and assisting its owner
  (§5, §6).
- **Resting (contract held)** — the owner is logged out; the live creature is
  dematerialized but the contract persists, re-materializing the hireling when
  the owner logs back in (§9).
- **Ended** — dismissed (§3.2), dead (§7), or upkeep-lapsed (§7); the contract is
  gone.

A hireling whose owner leaves the world (logout / link-death) MUST NOT be left
abandoned in a room: it dematerializes to the resting contract (§9), the same
never-orphan guarantee mounts give.

**Acceptance criteria**

- [x] An owned hireling is always exactly one of active / resting / ended.
      **SHIPPED (slice 1).**
- [x] An owner logging out dematerializes the live hireling without ending the
      contract; logging back in re-materializes it. **SHIPPED (slice 1** — logout
      drain + login re-materialize**).**
- [x] No owned hireling is left standing in a room after its owner leaves the
      world. **SHIPPED (slice 1** — `Manager.Remove` drains + dematerializes**).**

---

## 5. Following the owner

A hireling stays at its owner's side: when the owner changes rooms, each of their
live hirelings **relocates to the owner's new room**. A hireling is **bound** to
its owner — always co-located — rather than an independent trailer that can drift
or be left behind. This is the deliberate v1 model (`proposals/hireable-mobs.md`
§3.1): a paid bodyguard sticks with you, including through recall/teleport, which
distinguishes it from a [follow](follow.md) relationship (consent-free, gate-
respecting, breakable).

**Deferred refinements:** gate-respecting independent trailing (the
[follow](follow.md) §5 "attempt the step / left behind / rejoin" model); room
departure/arrival broadcasts so bystanders see the hireling move; and **emitting
the mob-move signal** [follow](follow.md) §1 deferred (so a *player* can follow a
*mob* — the onboarding-guide case). The relocate is a placement move today; the
generalized "a mob changed rooms" event is a small follow-on (§11).

**Acceptance criteria**

- [x] A hireling relocates to its owner's room when the owner moves (it stays
      co-located). **SHIPPED (slice 2** — `PullHirelings` on the `PlayerMoved`
      reaction**).**
- [x] An owner with no live hireling, or an offline owner, is a safe no-op.
      **SHIPPED (slice 2).**

## 6. Combat

### 6.1 Assistance

When its owner engages an enemy (or is engaged), an **idle** hireling in the same
room joins the fight against that enemy, reusing the **auto-assist** path
([grouping](grouping.md) §9): it engages and takes combat rounds like any
combatant. A hireling already fighting is left on its current target.

### 6.2 Being targeted, and death

A hireling is a world creature: it **can be targeted and killed**. Its death
**ends the contract** (§7) — the owner has lost their investment. A hireling does
not flee on its owner's behalf; its own flee/retaliate behavior is its AI's.

### 6.3 Loot

A corpse from a **hireling's kill** is owned by the **owner** (the corpse
owner-set, [loot-and-corpses](loot-and-corpses.md) §4): the kill-credit id is the
hireling's, and the owner-set hook maps it to the owner (extending to the owner's
party under the party's loot policy, [grouping](grouping.md) §5). The owner — not
the hireling — picks up the spoils.

### 6.4 Experience (the participation rule)

Kill experience rewards the **owner's participation, not their wallet**
([grouping](grouping.md) §4):

- a kill the **owner actively fought** (the owner was a combatant against the
  slain enemy) grants the owner normal kill-XP;
- a hireling's **solo** kill — the owner was not engaged with that enemy — grants
  the owner **no** XP.

A hireling is a force multiplier in fights the owner joins, not an idle XP farm;
the upkeep gold sink is the counterweight. (This is the resolved fork,
`proposals/hireable-mobs.md` §5 PD-4.)

**Acceptance criteria**

- [ ] An idle, co-located hireling joins its owner's fight against the owner's
      enemy; a hireling already in combat is not redirected.
- [ ] A hireling can be killed; its death ends the contract.
- [ ] A corpse from a hireling's kill is owned by the owner (loot rights), under
      the owner's party loot policy where a party exists.
- [ ] A kill the owner actively fought grants the owner kill-XP; a hireling-only
      kill grants the owner none.

## 7. Upkeep and contract end

A hireling costs **recurring upkeep** — a gold sink charged on a configured
cadence (`Upkeep cost` / `Upkeep cadence`, §10; the recurring-drain pattern of
[economy-survival](economy-survival.md) §4.4). When the owner **cannot pay**
upkeep, the hireling **departs** — the contract ends with a clear message,
rather than the hireling working for free.

The contract ends — the hireling is removed and its record dropped — on any of:

- **`dismiss`** (§3.2),
- **death** (§6.2),
- **upkeep lapse** (this section).

**Acceptance criteria**

- [ ] Upkeep is charged on its cadence; an owner who can pay keeps the hireling.
- [ ] An owner who cannot pay upkeep loses the hireling (contract ends, message),
      not a free worker.
- [ ] `dismiss`, death, and upkeep-lapse each end the contract and remove the
      hireling.

## 8. Orders

The owner directs a hireling with **`order <hireling> <command>`**, a small stance
set: **follow** (the default — trail and assist, §5/§6), **stay** (hold this
room, don't trail), **guard** (stay but still assist the owner if combat reaches
the room), and **attack `<target>`** (engage a specific enemy now). Orders set a
stance the AI behavior reads; only the owner may order their hireling.

**Acceptance criteria**

- [ ] `order` is owner-only; a non-owner is refused.
- [ ] `follow` / `stay` / `guard` / `attack` set the corresponding behavior; a
      `stay` hireling does not trail, a `guard` hireling holds but still assists.

## 9. Persistence

A hireling is **durable owned state**, persisted as the character's property so a
hire contract survives logout and restart. What persists:

- **the contract** — the link from a character to each hireling they own, with
  the hireling's **template identity** (and any upkeep/condition state policy
  keeps);
- nothing about the **live creature** beyond its identity: the live mob, its
  current room, its follow target, and its combat state are **transient** and
  re-resolved on load.

On logout the live hireling is **dematerialized** to the resting contract; on
login it is **re-materialized** into the owner's room (active). This mirrors the
mount persistence rule ([mounts](mounts.md) §10): persist the *contract*,
re-derive the live creature. The save shape is **additive** and versioned/migrated
like other save changes (a hireling list on the player save, parallel to the
owned-mount list).

**Acceptance criteria**

- [x] A hire contract (ownership + the hireling's identity) round-trips across
      logout and restart. **SHIPPED (slice 1** — save v33 `Hirelings`**).**
- [x] The live creature and its follow/combat state are **not** persisted; a
      returning owner gets a fresh hireling instance in their room. **SHIPPED
      (slice 1).**
- [x] No owned hireling contract is lost across a restart (an active hireling
      resolves to a resting contract and back). **SHIPPED (slice 1).**

## 10. Configuration surface

The following are externally configurable and not fixed by this spec.

| Policy | Where it applies |
|---|---|
| Hireling roster — per-type combat profile, hit points, hire cost, upkeep | §2, §3.1, §7 (content) |
| Hireling cap (simultaneous per character) | §3.3 |
| Hire cost | §3.1 |
| Upkeep cost and cadence | §7 |
| Lapsed-upkeep policy (depart immediately vs. grace period) | §7 |
| Dismiss refund policy (none vs. partial) | §3.2 |
| Recruiter access model (a tagged recruiter NPC vs. any suitable NPC) | §3.1 (content) |
| XP participation rule constants (what counts as "owner fought it") | §6.4 |
| Loose-hireling resolution (re-materialize location on login) | §9 |
| User-facing copy for hire/dismiss/order, refusals, departure, death | §3–§8 |

## 11. Open questions / future work

- **Multiple hirelings / bands.** v1 caps small; mercenary bands, formations, and
  patrol routes are deferred.
- **Hireling as a party seat.** Does a hireling count toward grouping's XP/loot
  split, or stay strictly the owner's asset (outside the party math)? Lean:
  owner's asset, not a party seat (simpler; doesn't dilute human shares).
- **Order depth.** Conditional orders ("attack anything that attacks me",
  "retreat below half HP"), patrol, and scripted dialogue are deferred.
- **Hireling equipment / advancement.** Content-fixed in v1; gear management and
  any progression are later.
- **Morale / loyalty / desertion** as a real system (v1 has only upkeep-lapse).
- **A shared owned-companion service.** Mounts and hirelings (and a future pack's
  summoned/controlled entities) share the owned-creature shape; whether to
  extract one service or keep parallel ones is an implementation choice to
  revisit when the third consumer lands.
- **Generalizing the mob-move signal.** §5 emits it for hirelings; making it the
  general "a mob changed rooms" event other systems can consume is a small
  follow-on.

## 12. Cross-references

- [mounts](mounts.md) — the owned-creature precedent (ownership, materialize /
  dematerialize, owned-record persistence, logout teardown) this reuses; §5.5 and
  §12 anticipate this consumer.
- [follow](follow.md) — the move-with-leader mechanic (§5); this consumer brings
  the mob-move signal follow §1 deferred.
- [grouping](grouping.md) — the auto-assist path (§6.1), the kill-credit + corpse
  owner-set (§6.3), and the kill-XP grant (§6.4).
- [loot-and-corpses](loot-and-corpses.md) §4 — the corpse owner-set the hireling's
  loot routes through.
- [mobs-ai-spawning](mobs-ai-spawning.md) — the mob template + AI behavior
  registry the hireling behavior registers with.
- [economy-survival](economy-survival.md) §3–§4 — the hire cost and the recurring
  upkeep gold sinks.
- [combat](combat.md) — the engagement, targeting, and death the hireling
  participates in.
