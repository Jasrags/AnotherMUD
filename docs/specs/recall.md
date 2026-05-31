# Recall — Feature Specification

**Status:** Draft · **Scope:** The `set recall` / `recall` verb
pair that lets a character mark a room as their personal return
point and teleport back to it on demand; the per-character save
field that persists the address across logout; the cancellable
pre-event and post-fact event that let content layers gate or
react to a recall · **Audience:** Anyone reimplementing or
porting this feature in any language.

This document describes *what* the recall surface must do, not
*how* to implement it. Specific verb names, defaults, and policy
on edge cases live in the configuration-surface table at §7.

Recall is the third item in Theme C — World Depth (see
`docs/themes/world-depth-plan.md`). It is intentionally a thin
substrate: the engine ships the verb surface, the persisted
field, and the event hooks, but it ships **no built-in cost,
cooldown, charge, or chant** (PD-3). Packs that want those
behaviors subscribe to the events and impose them as content
policy.

---

## 1. Overview

Recall is a per-character bookmark of a single room. Two verbs
operate on it:

- `set recall` — saves the actor's current room as their recall
  point. Persists on the player save.
- `recall` — teleports the actor from their current room to the
  saved point. Publishes a cancellable pre-event; if no
  subscriber cancels, the teleport commits and a post-fact
  event fires.

### 1.1 What recall is *not*

- Not a global town-portal: each character has their own
  address; no shared registry.
- Not a multi-slot bookmark system: each character has exactly
  one recall point at a time (PD-3 v1 scope).
- Not a movement primitive: recall is a teleport. It does not
  walk through exits, does not honor doors, does not consume
  movement points, and does not publish `player.moved` for
  the *intermediate* path (it does publish for the source →
  destination transition exactly once, since that's the
  end-of-move room change every system observes).
- Not a costed verb in the engine. No HP / mana / sustenance
  deduction, no cooldown timer, no consumed item. Packs add
  those by subscribing to `recall.before` (to refuse) or
  `recall.after` (to debit a resource).

---

## 2. Setting the recall point

`set recall` (canonical verb in §7) captures the actor's
current room id and stores it on the live actor + save.

### 2.1 Resolution

- The actor must be in a room (the normal precondition for any
  movement-adjacent verb).
- The room being captured is the actor's *current* room at the
  moment of dispatch; there is no argument form that captures
  an arbitrary room.
- If the actor is already in this exact room as their saved
  point, the command is idempotent and a "already set" message
  is acceptable as a confirmation.

### 2.2 Output

- Actor sees a confirmation line naming the room they just
  bound to. Wording is policy (§7).
- No room-broadcast: setting a recall is a personal action and
  not observable to other occupants.

### 2.3 Acceptance — set recall

- [ ] `set recall` from a room stores that room's id on the
      live actor.
- [ ] The same id appears on the player save's `recall` field
      after the next autosave (or final flush on shutdown).
- [ ] Calling `set recall` a second time from a different room
      overwrites the prior address; only one recall point per
      character is retained.
- [ ] Idempotent re-set in the same room writes the same value
      and does not error.

---

## 3. Recalling

`recall` (canonical verb in §7) teleports the actor from their
current room to the saved point.

### 3.1 Resolution order

1. Actor must be in a room.
2. Actor must have a non-empty saved recall point. If empty,
   emit the "no recall point" message and stop.
3. The saved room id must still resolve in the world registry
   (it may not — content packs change between sessions, and a
   removed room leaves the save's address pointing at nothing).
   If unresolved, emit the "recall point gone" message and stop.
4. If the saved room is the actor's current room, the command
   succeeds trivially: the actor sees an "already here"
   confirmation, the destination room is not re-rendered, and
   no event fires. (This is the spec choice — packs that want
   a no-op recall to still fire events can flip the
   `RecallSamePointFires` config in §7, deferred.)
5. Publish `recall.before` (cancellable). The pre-event carries
   the actor's id, the source room id, and the destination
   room id. If any subscriber flips the cancel flag, the
   teleport does **not** commit. The verb emits a generic
   "you can't recall right now" message so subscribers can
   write their own specific reason (cooldown active, in
   combat, drained, holy ground, etc.) without the engine
   stepping on them.
6. If uncancelled, the teleport commits:
   - The actor's room field is set to the destination.
   - The persisted `location` field is updated (already
     happens through the existing `SetRoom` path).
   - The destination room is rendered to the actor exactly as
     a movement arrival would render it.
7. Publish `recall.after` (non-cancellable post-fact). Same
   payload shape as `recall.before` minus the cancel flag.

### 3.2 Room-broadcast

- The source room sees a "vanishes" line; the destination room
  sees an "appears" line. Exact wording is policy (§7).
- Both lines are suppressed for the no-op case (§3.1 step 4).
- The actor's own view is the destination room render — they
  do not see the "you vanish" / "you appear" lines that other
  occupants see (this is the standard render contract).

### 3.3 player.moved interaction

The teleport changes rooms, which already triggers the
existing `SetRoom` path. That path publishes `player.moved`
exactly once with source = old room, destination = new room.
Recall does **not** publish a second `player.moved`. This
keeps the disposition evaluator and any other `player.moved`
subscribers from running their reset logic twice.

### 3.4 Acceptance — recall

- [ ] `recall` with no saved point emits the "no recall" line
      and does not change the actor's room.
- [ ] `recall` with a saved point that no longer resolves in
      the world emits the "recall point gone" line and does
      not change the actor's room.
- [ ] `recall` from the saved point itself is a no-op (no
      teleport, no events, no broadcasts), with the no-op
      confirmation line.
- [ ] `recall.before` fires before the room change; flipping
      the cancel flag aborts the teleport and the actor stays
      put.
- [ ] `recall.after` fires exactly once after an uncancelled
      teleport commits.
- [ ] The source room's other occupants see the "vanishes"
      line; the destination room's other occupants see the
      "appears" line.
- [ ] `player.moved` fires exactly once per successful recall
      (carried by the existing room-change path).

---

## 4. Saved-address invalidation

A recall point is a *string room id* on disk. The world
registry it resolves against is rebuilt from content on every
boot. Three failure modes are possible:

1. **Content drift** — the room id existed when `set recall`
   was last invoked but the content pack has since removed
   the room. Detected at recall time (§3.1 step 3). The save
   field is left alone — overwriting it to empty on a failed
   recall would silently lose the address if the room comes
   back in a future content patch.
2. **Pack reordering** — id collision rules in the namespaced
   pack-load layer prevent silent re-binding to a different
   room, so this case reduces to (1).
3. **Login-time auto-clear** — explicitly NOT in scope. The
   save loader does not validate the recall field on load.
   This keeps the load path simple and reserves all
   invalidation logic to the single recall codepath.

### 4.1 Acceptance — invalidation

- [ ] A saved recall pointing at a removed room round-trips
      through save/load unchanged; the field is not auto-
      cleared.
- [ ] The unresolved-target message at recall time names the
      missing room id at `info`-level in logs (operator-
      facing) but the actor-facing line stays generic.

---

## 5. Observability

| Event | Fields | When |
|---|---|---|
| `recall.set` | actor, room | `set recall` commits |
| `recall.before` | actor, from, to, **CancelFlag** | `recall` resolves a destination, before teleport |
| `recall.after` | actor, from, to | `recall` teleport committed |
| `recall.no_point` | actor | `recall` with empty save field |
| `recall.unresolved` | actor, missing_room | saved id no longer in world |

Routine invocations log at `debug` (recall is expected to be
a frequent verb). Unresolved-target events log at `info` so
operators can spot a content patch that pulled a room out
from under saved addresses. The cancellable `recall.before`
log entry includes whether the event was cancelled so
post-mortem traces can see why a teleport stalled.

### 5.1 Acceptance — observability

- [ ] Every observable transition emits exactly one log line.
- [ ] `recall.before` log entry records `cancelled=true/false`.
- [ ] `recall.unresolved` logs at `info` with the missing
      room id.

---

## 6. Persistence

The recall address persists as a single string field on the
player save. Empty string = "no recall point set" (the
default for fresh characters). The field is added behind a
save-version bump per the standard migration pattern
(`persistence.md` §7); a migration from the prior version
sets the field to empty on legacy saves, which is
indistinguishable from a fresh character.

There is no second field for "recall history" or "previous
recall point". The address is single-slot.

### 6.1 Acceptance — persistence

- [ ] Fresh character: recall field absent (`omitempty`) or
      empty string.
- [ ] After `set recall`, the field is non-empty and matches
      the room id captured.
- [ ] After a successful `recall`, the field is unchanged
      (the teleport does not consume or rebind it).
- [ ] Save-version migration from the prior version to the
      version that introduces the field is a no-op on the
      content (sets empty string) and round-trips.

---

## 7. Configuration surface

| Setting | Default | Meaning |
|---|---|---|
| `set recall` verb name | `set recall` | Canonical two-word form. Aliased forms (`setrecall`, `bind`) are policy decisions. |
| `recall` verb name | `recall` | Canonical one-word form. |
| Confirmation on set | "You bind your recall to this place." | Actor message after `set recall`. |
| Confirmation on idempotent set | "Your recall is already bound here." | Optional — same wording as the first set is also acceptable. |
| Departure broadcast | "$N vanishes." | Visible to other source-room occupants. |
| Arrival broadcast | "$N appears in a swirl of light." | Visible to other destination-room occupants. |
| Same-point no-op message | "You are already at your recall point." | Actor-only; no broadcast. |
| No-point message | "You have no recall point set. Use `set recall` somewhere first." | Actor-only. |
| Unresolved-target message | "Your recall point is no longer there." | Actor-only; the operator log line carries the room id. |
| Cancelled message | "You can't recall right now." | Generic so subscribers can write their own follow-up. |
| `RecallSamePointFires` | `false` | Whether the §3.1 step 4 no-op fires `recall.before` and `recall.after` for content layers that want to charge even no-op recalls. |
| Save field name | `recall` | YAML key on player save (`omitempty`). |

All bodies of text above are policy strings the renderer or
content pack may override; the spec only requires that the
verbs emit *some* message in each of those slots.

---

## 8. Open questions

The five Theme C pre-decisions (PD-1 through PD-5 in
`docs/themes/world-depth-plan.md`) settled the recall-relevant
ones (PD-3) before implementation, so the v1 surface is
explicitly costless and uses both pre- and post-events.

Remaining items, all explicitly *out of v1 scope*:

- **Multi-slot recall.** Characters get exactly one recall
  point in v1. A future enhancement might add named slots
  (`set recall home`, `recall home`). Pin in whichever
  milestone picks it up.
- **Cross-character recall.** Group-leader recall that drags
  followers is out of scope; the verb is per-character. Pin
  in whatever group/party milestone lands first.
- **Recall-from-anywhere restrictions.** Engine ships no
  restrictions on where `recall` can be invoked from
  (no "you can't recall from this room" rule baked into
  the engine). Content packs that want a sanctuary-only
  recall surface can subscribe to `recall.before` and
  cancel based on source-room tags. The deferred role-tag
  system (m6-5) is the natural home for that policy.
- **Recall-to-anywhere restrictions.** Same shape: the engine
  does not gate destinations. Content packs can subscribe
  and cancel based on destination tags.
- **Combat interaction.** Whether `recall` should be blocked
  while engaged in combat is a content decision in v1.
  Packs subscribe to `recall.before` and check
  `combat.engaged` via the existing combat surface. A
  future combat-feature update may add a built-in cancel
  for the common case.

---

## Cross-references

- `world-rooms-movement` — the room/exit/movement substrate
  recall teleports across; the `SetRoom` path recall
  reuses; the `player.moved` event recall does not
  re-publish.
- `persistence` — §4 player serialization (the save field
  added by §6 here) and §7 versioning + migration
  conventions.
- `commands-and-dispatch` — verb registration and dispatch
  pipeline both `set recall` and `recall` plug into.
- `mobs-ai-spawning` — `player.moved` consumer; recall
  honors its single-publish contract.
- `combat` — the deferred "block recall while engaged"
  open question in §8.
- `docs/themes/world-depth-plan.md` — Theme C live plan
  (PD-3 captures the v1 scope decision).
- `docs/specs/README.md` — spec layer placement and
  cross-cutting indexes (cancellable events table:
  `recall.before` belongs there once this spec lands).
