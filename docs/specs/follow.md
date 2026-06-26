# Follow (the move-with-leader primitive)

A **follow** relationship: when a leader leaves a room, those following them move
along. It is the small movement primitive several larger systems sit on —
**party/grouping** (a party auto-travels together), **hireable mobs** (a hireling
trails its owner), and the **onboarding guide** (a guide NPC walks a newbie
through first steps) all reuse the move-with-leader mechanic this spec defines.
Build follow first; those consumers layer on it.

It is **greenfield** (no prior code), but it adds **no new movement machinery**:
a follower's move is a normal move through the existing path — the same exit
resolution, doors, darkness, movement-point, and hidden-exit gates a player's own
step passes — only the destination is chosen for them. *Spec ahead of code —
build pending.*

## 1. Overview

Every room change in the engine funnels through one chokepoint and announces
itself with a single event (`world-rooms-movement`): a player walks, flees,
recalls, or is teleported, and the same "moved from A to B" signal fires after.
A **mob** that changes rooms (AI wander today) emits the **mob counterpart** of
that signal. Follow listens to both: when a **leader** — a player *or* a mob —
arrives in a new room, each of their **followers** attempts to make the same
trip. (Followers are always players; a mob is only ever a leader.)

### The model: trail, don't teleport

A follower is **not** snapped to the leader's side. They **attempt the same
move** — traverse the exit the leader took, from their own room, subject to every
gate the leader faced. This keeps follow honest: a follower out of movement
points, blocked by a door they can't open, or facing an exit they haven't
discovered simply **doesn't make it** and the follow ends. There is no
"following" that walks through walls.

### The adjacency rule (special moves fall out for free)

A follower keeps up **only when the leader's new room is reachable by a
traversable exit from the follower's current room** — i.e. the leader took a
normal step to an adjacent room. This one rule handles the special move paths
without special-casing them:

- **Walk** (the common case): leader steps north; the follower steps north too.
- **Flee** (a random adjacent exit): adjacent, so a follower *can* keep up —
  they flee alongside (an accepted v1 consequence; the leader uses `lose` or the
  follower `unfollow`s to avoid being dragged).
- **Recall / teleport** (a jump to a **non-adjacent** room): not reachable by an
  exit, so the follow **breaks** — you cannot trail someone who vanishes across
  the world. The follower is told they lost the leader.

### Goals / non-goals

**Goals.** A `follow` / `unfollow` / `lose` command surface; a leader's move
pulls their followers through the normal move path (all gates apply); failure to
keep up ends the follow with a clear message; follow chains (A→B→C); cycle
prevention; clean teardown on logout/death; the relationship is **transient**
(never persisted).

**Goals (mob leader).** A player can `follow` a **mob** they can see (a
wandering NPC, a guide); when that mob changes rooms it emits a mob-move signal
and the trailing player is pulled exactly as for a player leader. The mob is
**not** notified (it has no session) and is never a *follower*. The bound
hireling case (a creature glued to its owner) is the **hireable-mobs** consumer,
not this — a hireling moves *with* its owner, it is not *led* by them, so it
deliberately does not emit the mob-move signal.

**Non-goals.** Party/group reward-sharing (XP, loot, quest credit) — that is the
**grouping** consumer, a separate spec that uses this primitive. Mob-as-follower
(a hireling that trails its owner) — the **hireable-mobs** consumer. Group chat,
formation/marching order, and "assist in combat" are grouping concerns, not
follow's.

## 2. Following and unfollowing

- **`follow <target>`** — begin following a **player or mob** you can **see** in
  your room (the visibility predicate, `visibility.md`). The target is resolved
  through the room-entity arg (keyword/partial/ordinal-matchable, mobs winning
  ties — the same resolution combat targeting uses). The target is **not** asked
  for consent (§7 open question); you simply trail them. A confirmation is shown
  to the follower; a **player** leader is told someone follows them (a mob leader
  is not notified).
- **`follow`** (no argument) — report who you are following, or that you follow
  no one.
- **`unfollow`** (alias `follow stop`) — stop following whoever you follow.
- **`lose`** — the leader's counter: shake **all** of your own followers at once
  (each is told they lost you). The follow-target's escape hatch for an unwanted
  trailer absent a consent model.

Following is **consent-free** by default (§7): anyone you can see is followable.
You can follow **one** leader at a time; a second `follow` replaces the first.

### Cycle and self rules

- You cannot follow **yourself**.
- You cannot create a **cycle**: a `follow` that would make you (transitively)
  follow someone who already follows you is refused. Chains (A→B→C) are allowed;
  loops (A→B→A) are not.

### Acceptance criteria

- [ ] `follow <visible target>` establishes the relationship and notifies both
      sides; an unseen / absent target is refused (the visibility predicate
      governs what is followable).
- [ ] `follow` with no argument reports the current leader or "no one".
- [ ] `unfollow` / `follow stop` ends the follower's relationship and notifies
      them; `lose` ends **every** relationship where the caller is the leader and
      notifies each follower.
- [ ] Following a second target replaces the first (one leader at a time).
- [ ] Following yourself is refused; a follow that would form a cycle is refused;
      a chain (A→B→C) is allowed.

## 3. Moving with the leader

When a leader changes rooms, each follower **attempts the same move**:

- The follower traverses the **exit the leader took** (the leader's new room
  must be reachable by a traversable exit from the follower's room — §1
  adjacency). The move runs the **full normal path**: exit/door resolution,
  the darkness gate, the **movement-point** spend (`movement-cost`), the
  hidden-exit knowledge gate (`hidden-exits`), and the room-entry/exit broadcasts
  and arrival render — exactly as if the follower had typed the direction.
- The follower's own move **re-emits the move signal**, so a **chain** resolves
  naturally: A moves → B follows → B's move pulls C.
- **Order:** the leader completes their move (and is shown their new room) before
  followers are pulled; each follower is then moved and shown their own arrival.
- **Mob leader:** when the leader is a mob, the mob-move signal (emitted after
  the mob relocates — AI wander today) drives the same pull. The mob does not
  spend movement points or pass player gates; only the *followers'* steps do. A
  player trailing a wandering mob is carried along its route until they can't
  keep up (combat, a gate) or the mob stops/dies.

### When a follower can't keep up

A follower that **fails** the move — the leader's destination isn't adjacent
(recall/teleport), a door is shut they can't open, the exit is hidden to them,
they're out of movement points, or they're otherwise blocked — **does not move**
and the **follow ends**, with a message naming the cause ("You lose sight of
<leader>."). The leader is not interrupted. (The relationship is dropped rather
than left dangling across rooms; re-issue `follow` once reunited.)

A follower **in combat** does not get pulled (they're committed to the fight);
the leader leaving thus ends the follow, like any other failure to keep up.

### Acceptance criteria

- [ ] A leader's normal step pulls each follower through the same exit; the
      follower spends movement points and passes/fails the same gates as a
      self-issued step.
- [ ] A chain (A→B→C) all relocates when A steps to an adjacent room.
- [ ] A non-adjacent leader move (recall/teleport) breaks every follow with a
      "lost sight" message; the leader still moves.
- [ ] A follower blocked (shut door / hidden exit / no movement points / in
      combat) is left behind and the follow ends with a cause message.
- [ ] The leader is shown their destination before followers are pulled.

## 4. Teardown

The relationship is **transient** — it lives only while both parties are online
and is never written to a save (a relog starts you following no one). It ends on:

- **`unfollow` / `lose`** (the explicit verbs, §2).
- **Failure to keep up** (§3).
- **Logout / link-death** of either party: a follower going offline simply
  unfollows; a leader going offline drops all of their followers (each notified).
- **Death** of the leader: death relocates the leader (respawn / corpse, a
  non-adjacent move), which breaks the follow by the §3 rule. A **mob** leader
  that dies emits no further move signal, so its trailing players are released
  explicitly on the mob-killed event (each told the trail went cold).

No dangling half-relationship may survive either party leaving the world: a
teardown clears **both** sides (the follower's leader pointer and the leader's
follower set).

### Acceptance criteria

- [ ] No save-surface field is added; a relog leaves the player following no one
      and with no followers.
- [ ] Either party logging out clears both sides of every relationship they were
      in, with no dangling pointer; the surviving party is notified.
- [ ] A leader's death (relocation) breaks the follow.

## 5. Interaction with existing systems

- **Movement** (`world-rooms-movement`, `movement-cost`): a follower's pull is a
  normal move — same exit/door resolution, the movement-point spend, and the
  fatigue/encumbrance gate. Follow adds **no** bypass; an exhausted follower is
  left behind.
- **Visibility / hidden exits** (`visibility.md`, `hidden-exits.md`): `follow`
  targets only a **visible** character; a follower who hasn't discovered the
  leader's exit can't take it (left behind). A leader hiding/sneaking after the
  fact does **not** auto-break an established follow (use `lose`).
- **Combat** (`combat.md`): a follower in combat isn't pulled (§3); flee, being an
  adjacent move, *can* carry a non-fighting follower (§1).
- **Recall** (`recall.md`) **/ teleport** (admin): non-adjacent jumps break
  follow (§1).
- **Session lifecycle** (`session-lifecycle.md`): logout / link-death teardown
  (§4).
- **Mobs-ai-spawning** (`mobs-ai-spawning.md`): the mob-move signal is the mob
  counterpart of the player move event, emitted after an AI behavior relocates a
  mob (wander today; a future patrol/guide route reuses it). It is what makes a
  mob followable (§3).
- **Future consumers:** **grouping** (party reward-sharing over this primitive),
  **hireable mobs** (a mob follower bound to its owner), and the **onboarding
  guide** (a guide a newbie can now `follow`, built directly on the mob-leader
  path) — each its own spec; this defines the shared move-with-leader mechanic
  they all need.

## 6. Configuration surface

| Setting | Meaning | Default |
|---|---|---|
| Max followers per leader | Cap on simultaneous direct followers (anti-abuse). | a small cap, or unbounded in v1 |
| Max follow-chain depth | Cap on transitive chain length (guards pull cost / pathological chains). | a small depth, or unbounded in v1 |

Follow has no timing or numeric knobs of its own beyond these guards; the move
itself is governed by `movement-cost`'s configuration.

## 7. Open questions

- **Consent.** v1 is **consent-free** (trail anyone visible; `lose` is the
  counter), matching the classic Diku/GoMud model. Should there be an **opt-out**
  (a `nofollow` toggle that refuses would-be trailers) to prevent stalking, or a
  full **invite/accept** model? Invite/accept is the natural fit for **grouping**
  (reward-sharing should be consensual); plain follow staying consent-free with a
  `lose`/`nofollow` escape is the lighter choice. Resolve when grouping is specced.
- **Flee dragging followers.** §1 lets a follower flee alongside a fleeing leader
  (flee is adjacent). Is that desired, or should flee — a panic escape — **shed**
  followers? Shedding needs the move to carry a "this was a flee" marker the
  follow reaction reads; deferred unless it plays badly.
- **Mount + follow.** Does a mounted leader pull an unmounted follower (who may be
  slower / blocked by terrain the mount crossed)? The movement gate already
  refuses impassable terrain per-mover, so a follower simply fails the gate and
  is left behind — but whether a *mounted* follower should match a mounted
  leader's pace is a `mounts.md` interaction to revisit when relevant.
- **Display.** Should the room description / `look` show "X is following Y", and
  should `who` mark party leaders? Deferred to the grouping consumer's UI.
