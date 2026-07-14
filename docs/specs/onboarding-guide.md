# Onboarding guide

## Overview

A new character is met by a **guide** — a friendly NPC that materializes at the
character's side, **trails them** as they move through their first rooms, and
**departs** once the character has found their feet (a level threshold). The guide
is a low-friction teaching presence: it costs nothing, requires no player action,
and cannot be lost by walking away (it is bound to its owner, like a hireling, not
a follower that trails and can be left behind).

The guide is **live-only session state** — it is never persisted. It re-appears on
each login while the character is still under the graduation level, so the help is
there across sessions until it is no longer needed. A character who wants to be rid
of it early can `shoo` it away for the session.

Concepts:

- **Guide** — a mob materialized under an owner and marked a guide (a marker
  distinct from a hireling: a guide never fights, incurs no upkeep, counts against
  no cap, and credits no loot or XP).
- **Graduation level** — the character level at or above which a guide no longer
  appears and any live guide departs (configuration surface).
- **Bound trailing** — the guide is relocated to its owner's room on every move,
  so it is always co-located (the hireling follow model, not the player-follower
  trail model).

Non-goals (this spec): a guided walking *tour* (the guide leading the player); a
content-scripted speech tree; situational per-move tips (a later slice); durable
persistence of a "dismissed" preference across logins.

## Materialization

The guide appears when a character **enters the world** — at creation and on each
login — while their level is **below** the graduation level. Exactly one guide
exists per character at a time.

**Acceptance criteria**

- [ ] A newly created character below the graduation level has a guide at its side
  after entering the world.
- [ ] A returning character below the graduation level re-acquires a guide on
  login (a fresh live creature; nothing is read from the save).
- [ ] A character at or above the graduation level acquires no guide on login.
- [ ] A character that already has a live guide does not acquire a second.
- [ ] Materialization is a no-op when no guide template is configured (the feature
  is opt-in per world).
- [ ] The owner is told the guide has arrived; bystanders in the room see it appear.

## Trailing

When the owner moves, the guide is relocated to the owner's new room, so it stays
at the owner's side.

**Acceptance criteria**

- [ ] After the owner moves from room A to room B, the guide is in room B.
- [ ] Bystanders left in A see the guide leave; bystanders in B see it arrive.
- [ ] The guide cannot be outrun or left behind (it is relocated, not walked).
- [ ] Trailing is a no-op when the owner has no live guide.

## Graduation

When a character reaches the graduation level, any live guide departs with a
farewell; no guide re-appears thereafter (the login gate declines to spawn one).

**Acceptance criteria**

- [ ] On the level-up that reaches the graduation level, the live guide leaves the
  world and the owner is told it has gone.
- [ ] After graduating, a subsequent login spawns no guide.
- [ ] Graduation is a no-op for a character with no live guide.

## Dismissal (`shoo`)

A character may dismiss their guide early with `shoo`. This removes the live guide
for the session; because the guide is not persisted and the login gate still
applies, a character below the graduation level re-acquires a guide on the next
login (the dismissal is a session convenience, not a durable opt-out).

**Acceptance criteria**

- [ ] `shoo` with a live guide removes it and confirms to the owner.
- [ ] `shoo` with no live guide reports that there is no one to send away.
- [ ] A character below the graduation level who `shoo`s their guide re-acquires
  one on the next login.

## Departure on logout

A departing character's live guide leaves the world (it re-materializes on the
owner's return per the login gate). Ownership is not persisted, so nothing is
recorded.

**Acceptance criteria**

- [ ] On full logout, the character's live guide is removed from the world.
- [ ] The removal is atomic with respect to a concurrent `shoo` (no double-remove).

## Configuration surface

| Setting | Meaning | Default |
|---|---|---|
| Guide template | The mob template materialized as the guide. Empty ⇒ the feature is off for that world. | (unset) |
| Graduation level | The character level at/above which no guide appears and a live guide departs. | 3 |

## Open questions

- **Guide death.** A guide is friendly and does not fight, but nothing here makes
  it un-targetable, so a hostile mob could in principle kill a guide that trailed
  its owner into a fight. v1 has no active on-death teardown (unlike the hireling's
  `OnHirelingDeath`); instead the trail and the teardown paths **self-heal** — a
  guide whose entity no longer resolves in the world is dropped from the owner's
  live-guide overlay on their next move (no phantom placement is re-inserted), and
  graduation/dismissal of an already-gone guide is a silent no-op (no farewell for
  a ghost). A future slice could make guides un-targetable or add an immediate
  on-death notice mirroring the hireling's.
- **Situational tips.** The guide teaches only by presence in v1; emitting one-shot
  tips (first move → look/map, entering a shop, low sustenance, first fight) is a
  deferred slice.
- **Guided tour.** A guide that *leads* the player (patrol + `follow guide`) rather
  than trailing is deferred (needs a patrol behavior).
