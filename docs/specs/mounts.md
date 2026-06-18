# Mounts and Barding — Feature Specification

**Status:** Draft (spec; build pending) · **Scope:** Rideable **mount**
entities a character owns and rides; the owner/controller relationship; the
`mount` / `dismount` verbs; mounted travel (the mount becomes the metered
mover); barding as mount-worn armor; tack and saddlebag carry; stabling, feed,
and the mount economy; and a conservative v1 mounted-combat boundary ·
**Audience:** Anyone reimplementing or porting this feature in any language.

This document describes *what* the mounts feature must do, not *how* to
implement it. Specific prices, speeds, carry weights, barding penalties, feed
intervals, and stabling fees are policy that lives in configuration or content
(see §11).

Mounts are a **greenfield system**: nothing rideable exists in code today. The
`equipment.md` Mounts/barding/saddle tables are pure flavor until this lands —
their prices may be authored now as plain items, but the ride/speed/barding
mechanics are the deferred system this spec defines. This feature layers on
[mobs-ai-spawning](mobs-ai-spawning.md) (the mount is an owned, world-resident
creature), [world-rooms-movement](world-rooms-movement.md) +
[movement-cost](movement-cost.md) (mounted travel), [armor-depth](armor-depth.md)
(barding), [inventory-equipment-items](inventory-equipment-items.md) (saddlebags
as a container), and [economy-survival](economy-survival.md) (purchase, feed,
stabling as gold sinks).

---

## 1. Overview

A **mount** is a rideable creature a character owns. While a character rides it,
the mount carries the rider from room to room and **becomes the metered mover** —
mounted travel spends the *mount's* travel resource instead of the rider's, so a
fresh mount covers more ground than legs do before it tires. A mount can be
armored (**barding**), saddled for cargo (**tack**), stabled and fed between
adventures, and lost — to death, abandonment, or sale.

### Core concepts

- **Mount** — a rideable creature with an **owner** (the character who controls
  it), its own travel resource and carry capacity, its own armor (barding)
  slots, and a **temperament** that governs how readily it enters danger. A
  mount is a world-resident creature, not an inventory item: it occupies a room,
  can be looked at, can die.
- **The ride relationship** — the transient state binding a rider to a mount.
  Exactly one rider in v1. While the relationship holds, the rider and mount
  move together and the mount is the metered mover.
- **Mounted travel** — room-to-room movement while ridden: the mount's travel
  resource is spent, not the rider's; the mount's speed and barding load govern
  the rate; some terrain a mount cannot enter at all.
- **Barding** — armor worn by the mount (reusing [armor-depth](armor-depth.md)),
  adding the mount's defense at the cost of a speed penalty, a don/doff delay,
  and reduced cargo.
- **Tack** — the saddle and saddlebags: the mount's **cargo surface** (a
  container, reusing [inventory-equipment-items](inventory-equipment-items.md)),
  distinct from what the rider carries on their own person.
- **Upkeep** — the recurring gold sinks that keep a mount: purchase price,
  **stabling** between sessions, and **feed**. A neglected mount is a design
  lever, not a hard-coded punishment (§11).

### Goals

1. Make a mount a meaningful, owned asset: acquired, maintained, ridden, and
   losable, with real gold sinks.
2. Let a ridden mount **extend travel range** by bearing the movement cost the
   rider would otherwise pay, reusing the movement-cost machinery rather than a
   parallel system.
3. Express the source material's mount roster (riding horses, warhorses,
   ponies, mules) as content differences (speed, carry, temperament, combat
   tolerance) over one mount model.
4. Reuse existing systems — armor-depth for barding, containers for saddlebags,
   economy for upkeep, movement-cost for travel — rather than reinventing them.
5. Never strand a rider because their mount is exhausted, dead, or stuck.

### Non-goals

- **The move primitive.** Resolving an exit, the closed-door check, and
  relocating an entity remain [world-rooms-movement](world-rooms-movement.md)
  §3.3; mounted travel re-points *who* the metered mover is (§5), it does not
  change the primitive.
- **A new movement-metering model.** Mounted travel reuses the
  [movement-cost](movement-cost.md) pool/gate with the mount as the metered
  party; this spec does not define a second travel-resource mechanism.
- **Full tactical mounted combat.** v1 scopes a deliberate, minimal combat
  boundary (§7); the charge bonus, Ride-skill control contests, and
  mount-maneuvering are deferred (§12).
- **Flight and aerial mounts.** Flying mounts (and the "heavy barding disables
  flight" rule) are out of scope until a flight/movement-Z system exists (§12).
- **Pack trains and hauling vehicles.** Carts, wagons, sleds, and leading a
  string of cargo animals are a separate hauling concern (§12); v1 is a single
  ridden mount with its own saddlebags.
- **Breeding, training, or mount leveling.** A mount does not gain experience or
  progress; its capabilities are content-fixed.

---

## 2. The mount entity

### 2.1 What a mount is

A mount is a **world-resident creature** with an **owner**: a creature that, in
addition to the ordinary creature surface (a room location, a name, the ability
to be looked at, hit points, death), carries:

- an **owner** — the character entitled to ride and command it; a mount has at
  most one owner;
- a **travel resource** — the mount's own renewable movement budget, spent by
  mounted travel (§5), analogous to a character's movement pool but the mount's;
- a **carry capacity** — what the mount can bear (rider plus cargo), resolved
  through the same carry-capacity quantity characters use
  ([movement-cost](movement-cost.md) §4.4);
- **barding slots** — armor slots the mount's barding occupies (§8);
- a **temperament** — a content trait governing how readily the mount enters
  danger (§7.2): a steady war-trained mount tolerates combat; a skittish riding
  animal balks.

A mount is **not** an inventory item and is never carried, stacked, or stored in
a container. It is led, ridden, or stabled (§9).

### 2.2 Ownership

A character acquires ownership by purchase (§3) or content grant. Ownership is
**durable character state** (it persists, §10) and is **exclusive** — a mount
has one owner at a time. Only the owner may ride or command a mount; another
character attempting to mount a mount they do not own is refused (transfer and
shared mounts are §12). Ownership survives the owner's logout: a logged-out
character still owns their stabled mount.

**Acceptance criteria**

- [ ] A mount has a single owner; a non-owner cannot ride or command it.
- [ ] A mount is a world creature (look-able, hit-pointed, killable), never an
      inventory item.
- [ ] Ownership persists across the owner's logout and the server restart.
- [ ] A mount exposes a travel resource, a carry capacity, barding slots, and a
      temperament, all content-set.

---

## 3. Acquiring and keeping a mount

### 3.1 Purchase

Mounts are bought from a content-defined vendor (a stablemaster / horse trader)
through the existing shop path ([economy-survival](economy-survival.md) §3): the
mount's price is its purchase cost, and a successful buy transfers ownership to
the purchasing character and places the mount in the world under that character
(stabled, §9). Tack and barding are ordinary purchasable items.

### 3.2 Stabling

A character may **stable** an owned mount at a stabling access point: the mount
is held safely between sessions for a recurring **stabling fee** (a gold sink,
§11). A stabled mount is out of play — it is not in a room, cannot be ridden
until retrieved, and is not at risk. Retrieving (un-stabling) a mount returns it
to the world at the access point, ready to ride.

### 3.3 Feed and upkeep

A mount consumes **feed** as a recurring upkeep cost. v1 models upkeep as a
**gold/resource sink**, not a survival mini-game: feed is paid as part of
stabling (or as a purchasable consumable applied to the mount), and a mount whose
upkeep lapses degrades by a content-defined policy (§11) rather than by a
hard-coded death. The depth of upkeep consequence (a hungry mount tires faster,
refuses to travel, or eventually leaves) is policy, deliberately soft in v1.

**Acceptance criteria**

- [ ] A mount is purchasable through the shop path; a successful buy transfers
      ownership and places the mount under the buyer.
- [ ] Stabling holds a mount safely out of play for a recurring fee; retrieving
      it returns it to the world ready to ride.
- [ ] Feed/upkeep is a recurring gold/resource sink; lapsed upkeep degrades the
      mount by configurable policy, not by a fixed hard-coded death.

---

## 4. Riding: mount and dismount

### 4.1 Mounting

A character mounts an owned mount that shares their room with the `mount` verb.
Mounting MUST:

- be refused, with a clear message, when: the target is not a mount, is not
  owned by the actor, is not in the actor's room, the actor is already mounted,
  or the actor is in a state that forbids it (e.g. incapacitated by a
  [condition](conditions.md), or already in a posture the policy disallows);
- emit a **cancellable** `mount.before` event (§9-style content hook — the
  parallel of [recall](recall.md) §3.1's `recall.before`) so the content layer
  can gate or charge mounting (a cost, a cooldown, a skill gate); a listener that
  cancels aborts the mount with no state change;
- on success, establish the ride relationship (§4.3), announce the action to the
  room, and leave the rider and mount co-located.

### 4.2 Dismounting

A mounted character dismounts with the `dismount` verb, ending the ride
relationship and leaving both rider and mount in the current room. Dismounting is
always available to a conscious rider — it is part of the never-strand guarantee
(§6): a rider can always get off and walk. Certain events force a dismount (§7.3).

### 4.3 The ride relationship

While mounted:

- the rider and mount are **co-located** and move together (§5); relocating one
  relocates the other;
- the **mount is the metered mover** for travel (§5);
- the rider remains a full participant in everything else — they can talk, look,
  use items, fight (§7), and be targeted — subject to the posture rules a mounted
  rider implies (policy);
- the relationship is **transient** (not persisted as a live ride; see §10): a
  logout/restart resolves the rider and mount to a persisted resting state, it
  does not preserve "mid-ride."

**Acceptance criteria**

- [ ] `mount` is refused for a non-mount, an unowned mount, a mount in another
      room, an already-mounted actor, or a forbidding actor state, each with a
      clear message.
- [ ] `mount` emits a cancellable `mount.before`; a cancel aborts cleanly with no
      state change.
- [ ] A successful `mount` binds exactly one rider to the mount, announces to the
      room, and leaves them co-located.
- [ ] `dismount` is always available to a conscious rider and leaves both in the
      current room.
- [ ] While mounted, relocating the rider relocates the mount and vice versa.

---

## 5. Mounted travel

### 5.1 The mount becomes the metered mover

While a character is mounted, player-volition travel spends the **mount's**
travel resource, not the rider's. The rider's own movement pool
([movement-cost](movement-cost.md) §2) is **not** charged for a mounted step;
the mount's travel resource is charged instead, through the same cost gate
([movement-cost](movement-cost.md) §3) with the mount as the metered party.

This is the central reuse: mounted travel re-points *who pays* from rider to
mount; the move primitive, the step-cost derivation (§5.2), the never-strand rule,
and the difficulty hint are the movement-cost machinery unchanged.

### 5.2 Speed, barding load, and step cost

The cost of a mounted step is the destination terrain cost
([movement-cost](movement-cost.md) §4.1–§4.2) as borne by the mount, modulated by:

- the mount's **speed** — a faster mount has a larger/cheaper travel budget, so
  it crosses more rooms before tiring (content-set per mount type, §11);
- the mount's **barding load** — barding adds a movement surcharge to the mount,
  the mounted analogue of a character's encumbrance surcharge
  ([movement-cost](movement-cost.md) §4.4): heavier barding slows the mount
  (§8);
- the mount's **cargo load** — saddlebag weight against the mount's carry
  capacity contributes a surcharge the same way (§8.3).

A mount's superior speed is the point of riding: under ordinary load it should
out-travel walking, so mounted journeys are the long-distance mode and legs the
local one. (The exact advantage is balance policy, §11/§12.)

### 5.3 Terrain a mount cannot enter

Some destinations a mount cannot enter at all (a cramped tunnel, a building
interior, terrain a content flag marks as **mount-impassable**). Attempting to
ride into a mount-impassable destination is **refused** with a clear message;
the rider may `dismount` and proceed on foot. Temperament (§7.2) is a separate,
softer gate (a skittish mount balks at *danger*, not at *geometry*).

### 5.4 The exhausted mount

When the mount's travel resource is short of a step's cost, the step is refused
exactly as a winded character's is ([movement-cost](movement-cost.md) §3.4): the
party does not move, a clear message explains the mount is blown, and the rider
may `dismount` and walk (spending their own pool) or wait for the mount to
recover. The mount's travel resource regenerates out of combat on the regen
heartbeat, like a character's pool.

**Acceptance criteria**

- [ ] A mounted step charges the mount's travel resource and does **not** charge
      the rider's movement pool.
- [ ] A faster mount travels farther per resource than a slower one; heavier
      barding/cargo shortens that range.
- [ ] A mount-impassable destination refuses the mounted step with a clear
      message; the rider can dismount and proceed.
- [ ] An exhausted mount refuses the step (no relocation, clear message); the
      rider can dismount and walk or wait for regen.
- [ ] The move primitive, never-strand rule, and difficulty hint behave as in
      movement-cost, with the mount as the metered mover.

### 5.5 Following and leading

A character may **lead** an un-ridden owned mount on foot, so the mount follows
them room to room without being ridden (the mount is present but the rider walks
and pays their own movement cost). Leading vs riding vs stabled are the three
ways a mount travels with its owner (§9). The follow relationship a led mount
uses is the same owned-followable-entity relationship a future player/NPC follow
or hireable-companion system would use (§12) — built once, here.

**Acceptance criteria**

- [ ] An owner can lead an un-ridden mount so it follows them between rooms.
- [ ] A led mount does not meter travel — the walking owner pays their own
      movement cost; the mount simply follows.

---

## 6. Never-strand guarantee

A rider is **never trapped** by their mount. The guarantees:

- a conscious rider may always `dismount` and continue on foot;
- an exhausted, balking, or mount-impassable situation always leaves the
  dismount-and-walk option open (§5.3, §5.4, §7.2) — the mount never blocks the
  rider's *own* movement, only the *mounted* step;
- if a mount dies or is removed while ridden, the rider is set down safely in the
  current room (a forced dismount, §7.3), never deleted, displaced, or stuck.

**Acceptance criteria**

- [ ] No mount state (exhausted, balking, impassable, dead) can leave a rider
      unable to move on foot.
- [ ] A rider unseated by mount death/removal lands safely in the current room.

---

## 7. Mounted combat (v1 boundary)

v1 scopes mounted combat **deliberately small**: a mount is primarily transport,
and combat is handled by clear, minimal rules rather than a tactical mounted-
combat layer. The charge bonus, Ride-skill control contests, and mounted
maneuvering are deferred (§12).

### 7.1 Fighting from the saddle

A mounted rider **may** fight: they engage, take rounds, and are targeted as
normal ([combat](combat.md)), with no mounted-specific bonus or penalty to the
rider's own attacks in v1. The rider is not forced to dismount merely because a
fight begins.

### 7.2 Temperament and entering danger

A mount's **temperament** governs whether it will *carry its rider into* danger:

- a **war-trained** mount (a warhorse) tolerates combat — the rider may ride into
  a hostile room and fight mounted;
- a **skittish** mount (an ordinary riding horse or pony) **balks** at entering a
  room with active hostiles, or when the rider initiates combat from its back:
  the mount refuses the mounted step / forces the rider to dismount, with a clear
  message. A **steady** working animal (a mule) is hardy and willing where a
  horse balks (the source material's "mules enter dangerous places horses won't").

Temperament gates only *danger* entry, never ordinary travel. The exact
tolerance ladder (which temperaments balk at what) is content/config policy
(§11). The Ride-skill *contest* that would let a rider override a balk is deferred
(§12).

### 7.3 Targeting a mount, and forced dismount

The mount itself can be **targeted and killed** while ridden (it is a world
creature, §2.1). When a ridden mount dies — or is otherwise removed — the rider
is **forced to dismount**, landing safely in the current room (§6). A forced
dismount also occurs when temperament balks (§7.2) and may occur on a content-
defined event (a failed Ride contest, once that lands). Fleeing
([combat](combat.md)) while mounted uses the mount's movement to withdraw.

**Acceptance criteria**

- [ ] A mounted rider may fight with no mounted-specific bonus/penalty in v1, and
      is not auto-dismounted merely because combat starts.
- [ ] A skittish mount refuses to carry its rider into a hostile room / forces a
      dismount when combat is initiated from its back; a war-trained mount does
      not; a steady working animal tolerates danger a skittish one won't.
- [ ] A ridden mount can be targeted and killed; its death forces the rider to a
      safe dismount in the current room.
- [ ] Temperament gates danger entry only, never ordinary travel.

---

## 8. Barding and tack

### 8.1 Barding as mount-worn armor

**Barding** is armor the **mount** wears, reusing [armor-depth](armor-depth.md):
it occupies the mount's barding slots (§2.1), contributes the mount's armor class
and per-type resistance through the same defense/mitigation channels a
character's armor uses, and stacks/reverses on equip/unequip the same way. The
barding's bonus and check/penalty are sized for the mount, and a barding piece is
fitted to a mount's **size** ([size-and-wielding](size-and-wielding.md)) — barding
for a large mount differs in price/weight from a medium one (content, §11).

### 8.2 The barding speed penalty and don/doff

Barding **slows the mount**: it adds a movement surcharge to the mount's travel
(§5.2), the mounted analogue of armor's effect on a character. Heavier barding
slows more (a tiered penalty, policy §11). Barding takes **time to don and
remove** — a delayed action like a character donning heavy armor
([armor-depth](armor-depth.md) §7) — and so cannot be put on or stripped off
instantly mid-crisis. ("Heavy barding disables flight" is noted but moot until a
flight system exists, §12.)

### 8.3 Tack: saddle and saddlebags

The **saddle** and **saddlebags** are the mount's **cargo surface**: a container
([inventory-equipment-items](inventory-equipment-items.md) §4) attached to the
mount, holding goods separately from the rider's own inventory. Saddlebag cargo
counts against the mount's carry capacity and contributes a movement surcharge
when heavy (§5.2). A **barded** mount carries less cargo — barding consumes much
of the carry budget, so a war-barded mount bears little beyond its rider (the
source material's "lead a second mount for cargo" — and a pack train is §12).

**Acceptance criteria**

- [ ] Barding equips to the mount's barding slots and contributes the mount's
      AC/resistance through the armor-depth channels, stacking and reversing like
      character armor.
- [ ] Barding adds a movement surcharge to the mount (heavier = slower) and takes
      a non-instant time to don/remove.
- [ ] Saddlebags are a container attached to the mount, distinct from the rider's
      inventory; their weight counts against the mount's carry capacity.
- [ ] A barded mount's available cargo capacity is reduced by the barding load.

---

## 9. The mount in the world

At any moment an owned mount is in exactly one of three states:

- **Ridden** — bound to a rider in the ride relationship (§4.3), co-located and
  moving together.
- **Led / present** — un-ridden but in the world, in a room (following its owner
  if led, §5.5, or simply standing where it was left).
- **Stabled** — held out of play at a stabling access point (§3.2), not in any
  room and not at risk.

A mount left **present** in a room when its owner leaves (rides off on another
mount, walks away without leading it, or logs out) is handled by a content
policy that MUST NOT silently delete an owned asset (§10): the v1 default returns
an owner's loose mount to a **stabled/parked** resting state rather than leaving
it indefinitely abandoned in the world. (Richer "your horse wanders / can be
stolen / waits where you left it" behavior is §12.)

**Acceptance criteria**

- [ ] An owned mount is always exactly one of ridden / present / stabled.
- [ ] An owner's mount left behind is never silently deleted; the v1 default
      resolves a loose owned mount to a safe resting state.

---

## 10. Persistence

A mount is **durable owned state**, persisted as the character's property so an
owned mount survives logout and restart. What persists:

- **ownership** — the link from a character to each mount they own;
- the mount's **durable attributes** — its type/identity, its barding and tack,
  its saddlebag contents, and any upkeep/condition state the policy keeps;
- the mount's **resting location state** — stabled vs. parked — resolved from the
  three world-states (§9) to a persistable resting state on logout/restart.

What does **not** persist (transient, re-resolved on load):

- the **live ride relationship** — a logout/restart does not preserve "mid-ride";
  the rider and mount resolve to a resting state (the mount stabled/parked, the
  rider on foot) and the player re-mounts after login;
- the mount's **travel-resource current** beyond what the pool model already
  persists (the mount's pool follows the same persist-current / re-derive-max
  rule as a character's, [movement-cost](movement-cost.md) §6) — policy may
  choose to refill a rested mount on load.

The exact save shape (a mount sub-store, a mount list on the player save, or a
world-level owned-mount registry) is an implementation choice constrained only by
the durability and exclusivity contracts above; see
[persistence](persistence.md). Whatever the shape, it is **additive** and
versioned/migrated like other save changes.

**Acceptance criteria**

- [ ] Ownership, the mount's identity, its barding/tack, and its saddlebag
      contents round-trip across logout and restart.
- [ ] The live ride relationship is **not** persisted; a returning player starts
      un-mounted with their mount in a resting state.
- [ ] No owned mount is lost across a restart.

---

## 11. Configuration surface

The following are externally configurable and not fixed by this spec.

| Policy | Where it applies |
|---|---|
| Mount roster — per-type speed, carry capacity, temperament, hit points, price | §2, §3.1 (content) |
| Mount travel-resource baseline max and regen amount/cadence | §5.1, §5.4 |
| Mounted step-cost advantage (how much faster than walking) | §5.2 |
| Mount-impassable terrain flag | §5.3 (content) |
| Temperament tolerance ladder (which temperaments balk at what danger) | §7.2 |
| Barding roster — per-piece bonus, resistance, size fit, price/weight | §8.1 (content) |
| Per-tier barding movement surcharge | §8.2 |
| Barding don/doff time | §8.2 |
| Saddle/saddlebag capacity; cargo-vs-barding carry tradeoff | §8.3 (content) |
| Stabling fee and cadence | §3.2 |
| Feed/upkeep cost, cadence, and lapsed-upkeep degradation policy | §3.3 |
| Loose-mount resolution policy (parked vs stabled vs wanders) | §9 |
| `mount.before` content gating (cost / cooldown / skill gate) | §4.1 |
| User-facing copy for mount/dismount, refusals, balking, exhaustion | §4–§7 |

---

## 12. Open questions / future work

- **Mount as a specialized creature vs. a new entity type.** This spec describes
  the *behavior* of an owned, rideable creature; whether the implementation
  extends the existing mob/creature substrate
  ([mobs-ai-spawning](mobs-ai-spawning.md)) with an owner + ride surface, or
  introduces a distinct `Mount` entity, is an implementation fork deliberately
  left open. The owned-followable-entity relationship (§5.5) should be built so a
  player/NPC **follow** system and **hireable companions** can share it.
- **Mounted combat depth.** v1 (§7) is intentionally minimal: fight-from-the-
  saddle with no bonus, temperament-gated danger entry, killable mount. The
  source material's **charge** (double damage from a mount on a charge), the
  **Ride-skill control contest** (a check to keep a skittish mount in a fight, to
  override a balk, to stay seated when the mount is hit), and mounted reach/height
  advantages are deferred. The Ride contest specifically waits on a **Ride skill**
  in [skills](skills.md).
- **Multi-seat / howdah.** v1 binds one rider. Two-up riding, a howdah, or a
  driver-plus-passenger are deferred.
- **Pack trains and hauling vehicles.** Leading a string of cargo animals, and
  carts/wagons/sleds (a separate hauling-capacity concern with its own
  road/terrain rules), are out of scope; v1 is one ridden mount with saddlebags.
- **Flight and aerial mounts.** Flying mounts and the "heavy barding disables
  flight" rule wait on a flight / vertical-movement model
  ([world-rooms-movement](world-rooms-movement.md) has no Z-axis traversal today).
- **Mount transfer, theft, and shared mounts.** v1 ownership is exclusive and
  non-transferable in-play. Selling/gifting a mount, a mount left in the world
  being stolen, and stable-shared mounts are deferred (and couple to the trade
  systems).
- **Upkeep depth.** v1 keeps feed/upkeep a soft gold sink (§3.3). A real hunger/
  condition model for mounts (a starved mount tires, sickens, or leaves) is a
  later balance choice, parallel to character sustenance.
- **Mount AI when present but un-ridden.** A loose or led mount's behavior (does
  it flee combat, defend its owner, wander) reuses [mobs-ai-spawning](mobs-ai-spawning.md)
  but its disposition profile is unspecified here.
- **Balance.** Every number — the mounted speed advantage, barding penalties,
  stabling/feed costs, mount prices — is policy (§11) tuned against the live world
  once mounts are playable, mirroring the movement-cost balance note
  ([movement-cost](movement-cost.md) §9).
- **Client surfacing.** A mounted state, the mount's travel resource, and barding
  are not exposed to a structured client channel (GMCP) today; a future HUD may
  want them.
```
