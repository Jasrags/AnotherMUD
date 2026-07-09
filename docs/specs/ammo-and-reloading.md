# Ammunition & Reloading (holders · loading · ejection)

**Status:** Draft · **Scope:** the physical-ammunition model — loose rounds,
ammunition **holders** (clip / magazine / belt / drum / speed-loader), loading a
holder, loading a weapon, ejecting a spent holder, and the unified `reload` verb.
Extends `ranged-combat §3` (ammo as ordinary items) from a per-shot loose-round
model to a holder-fed one, ruleset-agnostic (a firearm's clip, a crossbow's
magazine, an autocannon's belt all reuse it). Shadowrun is the reference
consumer.

## 1. Overview

`ranged-combat §3` gave projectile weapons ammunition as ordinary items,
consumed one loose round per shot. Some weapons are **holder-fed**: rounds live
in a removable **ammunition holder** (a clip, a magazine, a belt), the holder
goes into the weapon, and firing draws from the *inserted holder*, not straight
from the pack. Reloading such a weapon **swaps holders** — the spent one is
**ejected** into the room. Other weapons are **internally fed** (a revolver's
cylinder, a break-action, a muzzle-loader): there is no removable holder, so they
take loose rounds directly — the model `ranged-combat §3` and the abstract-magazine
precursor already describe.

A weapon's **reload method** (a content-declared weapon attribute) decides which
family it is; the same `reload` verb serves both.

**Goals.** Make ammunition physical and tactical without making combat a
micromanagement chore: holders are real items you carry and swap, spent holders
litter the ground (recoverable, then decaying), and one verb — `reload` — covers
loading a weapon, loading a holder, and feeding an internal weapon. **Non-goals:**
per-round manual loading as the *required* path (it is an optional downtime
action, never forced mid-fight); recoil / firing modes / burst (owned elsewhere);
and cross-room supply.

**Related specs.** `ranged-combat` (the loose-round + range-band model this
extends), `inventory-equipment-items` (holders and rounds are ordinary items;
a holder is a constrained container), `masterwork` (a holder carries its rounds'
grade — see §8), `loot-and-corpses` (the ejected-holder decay reuses the
timed-decay pattern), `action-economy` (reloading may cost a timed action — §9).

## 2. The three tiers (the object model)

- **Rounds** — ordinary stackable ammunition items with an **ammunition kind**
  (`ranged-combat §3`). The base currency of ammo; bought loose, poured into
  holders, or fed straight into internally-fed weapons.
- **Ammunition holders** — items that **hold** rounds: a capacity, a current
  load, an accepted **round kind**, and a **fit** (which weapon family accepts
  them). A holder is a constrained container (it holds rounds only, up to its
  capacity). Clip / magazine / belt / drum / speed-loader are all holders,
  differing only in capacity and flavor.
- **Weapons** — a **holder-fed** weapon accepts a matching holder and fires from
  the inserted one; an **internally-fed** weapon has its own capacity and takes
  loose rounds directly. The weapon's **reload method** classifies it.

**Acceptance criteria**

- [ ] A holder declares its capacity, accepted round kind, and the weapon family
      it fits; it holds rounds only, never exceeding capacity.
- [ ] A weapon's reload method classifies it as holder-fed or internally-fed; a
      holder-fed weapon fires from its inserted holder, an internally-fed weapon
      from its own loaded rounds.

## 3. The unified `reload` verb — "top up the target from the tier below"

One verb; the **target's type** decides what feeds it:

| Target | Fed from | Effect |
|---|---|---|
| a **holder-fed weapon** (implicit: the wielded one) | a compatible **loaded holder** in the actor's inventory | insert it; the previously-inserted holder is ejected (§7) |
| an **internally-fed weapon** (implicit: the wielded one) | loose **rounds** of the matching kind | load rounds into the weapon up to its capacity |
| an **ammunition holder** (named) | loose **rounds** of the matching kind | load rounds into the holder up to its capacity |

So `reload` with no target acts on the **wielded weapon**; `reload <holder>` acts
on a **named holder** in inventory. The mental model is uniform: *reload the named
thing and it draws from whatever feeds it* — a weapon is fed by a holder (or, if
internal, by loose rounds); a holder is fed by loose rounds.

- Reloading tops up from what is available: a partial supply loads what it can
  (a partial holder, a partly-filled weapon) and reports the result.
- Reloading a target that is already full is a no-op with a clear message.
- Reloading with nothing available to feed it fails with a clear reason.

**Acceptance criteria**

- [ ] `reload` with no target reloads the wielded weapon: a holder-fed weapon
      takes a compatible loaded holder from inventory; an internally-fed weapon
      takes loose rounds.
- [ ] `reload <holder>` fills the named inventory holder from loose rounds of its
      kind.
- [ ] Every reload reports the resulting load vs. capacity; a full target is a
      no-op message; no feed available fails with a reason.

## 4. Loading a holder (rounds → holder)

- Filling a holder moves loose rounds of the holder's accepted kind from the
  actor's inventory into the holder, up to the holder's remaining capacity.
- A holder holds a **single round kind at a time** (§8): filling with a different
  kind than it already holds is refused (empty it first). This keeps a holder's
  contents homogeneous so its rounds carry one grade/type.
- Filling is the **downtime** path — it is never required mid-fight (a player
  carries pre-loaded spare holders for that, §6).

**Acceptance criteria**

- [ ] Filling a holder consumes matching loose rounds from inventory and raises
      the holder's load toward capacity; a short supply is a partial fill.
- [ ] A holder rejects rounds of a kind different from what it already holds.

## 5. Loading a weapon (holder → weapon, or rounds → internal weapon)

- **Holder-fed:** reloading selects a **compatible, loaded** holder from the
  actor's inventory (matching the weapon's family and accepted round kind) and
  **inserts** it. Any holder already inserted is **ejected** first (§7). With no
  compatible loaded holder available, the reload fails with a reason.
- **Internally-fed:** reloading feeds loose rounds directly into the weapon up to
  its capacity (the `ranged-combat §3` / abstract-magazine behavior). No holder is
  involved and nothing is ejected.
- Firing draws one round from the inserted holder (holder-fed) or the weapon's
  own load (internally-fed) per shot; an empty feed is a dry attempt
  (`ranged-combat §3` out-of-ammo behavior) — reload to continue.

**Acceptance criteria**

- [ ] Reloading a holder-fed weapon inserts a compatible loaded holder and
      ejects the prior one; with none available it fails with a reason.
- [ ] Reloading an internally-fed weapon loads loose rounds directly, ejecting
      nothing.
- [ ] Firing decrements the inserted holder (or internal load); an empty feed is
      a dry attempt, not a crash or a free shot.

## 6. Acquisition (buying ammo)

Behavior only; prices and stock are content. A vendor of ammunition may offer, in
any combination:

- **Loaded holders** — the primary buy for going armed: a holder that arrives at
  full capacity, ready to insert.
- **Loose rounds** — the refill currency, for topping up holders (§4) or feeding
  internally-fed weapons (§5).
- **Empty holders** — cheap spares, so a runner can carry more holders than they
  bought loaded and refill them in downtime.

**Acceptance criteria**

- [ ] A player can buy a loaded holder and immediately reload a compatible weapon
      with it (no separate fill step).
- [ ] A player can buy loose rounds and an empty holder, fill the holder (§4),
      and use it.

## 7. Ejection and decay of spent holders

- Reloading a holder-fed weapon **ejects** the previously-inserted holder **into
  the room**, carrying **its remaining rounds** (a partly-spent holder ejects
  partly-loaded, not empty). Occupants see the ejection.
- An ejected holder is an ordinary room item: recoverable (pick it up, refill it
  §4, reuse it) for a window, then it **decays** and is removed — reusing the
  timed-decay pattern of `loot-and-corpses` so a firefight does not permanently
  litter a room. The decay lifetime is configurable (§10).

**Acceptance criteria**

- [ ] Reloading ejects the prior holder into the room with its remaining rounds
      intact; a fully-spent holder ejects empty, a partial one ejects partial.
- [ ] An ejected holder is pickable and refillable within its lifetime, then
      decays and is removed.

## 8. Round kind, fit, and grade (compatibility + masterwork)

- **Round kind** gates loading: a holder accepts one kind; a weapon (or its
  holder) fires that kind. Mismatched rounds do not load (`ranged-combat §3`).
- **Fit** gates insertion: a holder declares the **weapon family** it fits; a
  weapon accepts only holders that fit it (a pistol holder does not enter an SMG).
- **Grade carries through the holder.** Because a holder is homogeneous (§4), the
  grade/type of its rounds (`masterwork` ammo, or a special round type) travels
  with the holder: a shot fired from the holder uses the holder's round grade.
  This **resolves masterwork/special ammo for holder-fed weapons** — previously
  a holder's rounds were an untyped count.

**Acceptance criteria**

- [ ] A weapon accepts only holders whose fit matches its family and whose round
      kind it fires; a mismatched holder does not insert.
- [ ] A holder filled with graded/special rounds confers that grade/effect on
      each shot fired from it (masterwork ammo works through a holder).

## 9. Persistence

- A holder's current load (and round grade/kind, §8) persists with the holder
  item, wherever it lives — inventory, a container, the ground, or **inserted in a
  weapon**.
- An inserted holder persists as **held by its weapon** (an inserted-holder
  relationship), so a loaded holder-fed weapon stays loaded across a relogin, and
  reloading it after a relogin ejects the same holder.
- Ejected holders on the ground follow ordinary room-item persistence (they do
  not survive a restart, like other dropped items and their decay timers).

**Acceptance criteria**

- [ ] A partly-loaded holder keeps its round count (and grade/kind) across a
      relogin, whether carried or inserted in a weapon.
- [ ] A weapon reloaded before logout is still loaded (with the same holder) on
      return.

## 10. Configuration surface

| Setting | Meaning | Default |
|---|---|---|
| Holder capacity | Rounds a holder type holds (§2). | content per holder |
| Holder fit / round kind | Which weapon family a holder enters and which round kind it feeds (§8). | content per holder/weapon |
| Weapon reload method | Holder-fed vs internally-fed classification, and (holder-fed) which holder family (§2, §5). | content per weapon |
| Internal weapon capacity | Rounds an internally-fed weapon holds (§5). | content per weapon |
| Ejected-holder lifetime | How long a spent holder lingers on the ground before decaying (§7). | policy duration |
| Reload action cost | The timed cost of a reload, by reload method (§9 open). | policy (see open questions) |
| Ammo prices / stock | Cost of loaded holders, loose rounds, empty holders (§6). | content per vendor |

All numeric magnitudes live here; the prose names behaviors, not values.

## 11. Decisions and open questions

**Decided:**

- **Tier B-lite (holders are real, and come loaded).** Clips/magazines/belts are
  items; the primary buy is a **loaded** holder; `reload` swaps a loaded holder
  and ejects the spent one; refilling from loose rounds is an *optional downtime*
  action, never forced in combat. (Rejected: the fully-abstract "rounds straight
  into the gun" model — the shipped precursor — for holder-fed weapons; and the
  fully-manual "fill every holder round-by-round before use" model as the
  *required* path.)
- **Ejected holders are recoverable, then decay** (§7) — not destroyed on eject
  (holders are reusable), not persistent-forever (rooms would silt up with brass).
- **One verb, `reload`, tops up the target from the tier below** (§3) — no
  separate `fill` / `insert` / `eject` verbs; the revolver (internally-fed) case
  falls out of the same verb with no holder.
- **Homogeneous holders** (§4, §8) — one round kind per holder — which is what
  lets a holder carry its rounds' grade and unblocks masterwork ammo for
  holder-fed weapons.
- **Internally-fed weapons keep the loose-round model** (§5) unchanged — the
  revolver / break-action / muzzle-loader path is the shipped behavior.

**Still open (non-blocking):**

- **Reload as a timed action.** Reloading (holder swap, or fill) plausibly costs a
  timed action per reload method (a removable clip is fast; filling a holder is
  slow). Reuse `action-economy`'s busy-state timer, or leave reload instant for a
  first slice? (The shipped precursor is instant.)
- **Compatibility strictness** — fit by broad **weapon family** (a "heavy-pistol"
  clip) vs. exact weapon id vs. a shared "holder family" that spans several
  weapons. Lean: weapon family.
- **Mixed-ammo holders.** Homogeneous is assumed (§8); allowing a hand-packed mix
  (some tracer, some regular) is deferred — niche, and it breaks the
  one-grade-per-holder simplification.
- **Auto-selection order** when several compatible loaded holders are carried
  (fullest first? a specific-holder `reload <holder> into <weapon>` override?).
  The shipped SR-M3f-1 picks the fullest carried holder but does **not** compare
  against the holder already inserted — so `reload` with a mostly-full holder
  seated and only near-empty spares carried will still eject the good one. A
  "don't swap for a worse holder" guard (or an explicit-only swap) is open.
- **Speed-loaders and belts** as holder sub-behaviors (a speed-loader loads an
  internally-fed cylinder in one action; a belt is a large holder) — same model,
  flavor/timing differences, phased after clips.

<!-- Scope: physical ammunition — loose rounds + ammunition holders (clip/magazine/belt/drum/speed-loader) + the unified reload verb (load weapon / load holder / feed internal weapon) + ejection & timed decay of spent holders + grade-through-holder; extends ranged-combat §3 from loose-per-shot to holder-fed, ruleset-agnostic, Shadowrun as reference consumer · Spec style: narrative + acceptance criteria · Detail level: behavior only · Status: partly shipped — holder-fed core (holders as items, the unified reload, insertion + fire-from-holder, ejection to the room, persistence) shipped in the Shadowrun pack (SR-M3f-1); ejected-holder DECAY, buying loaded/empty holders, and grade-through-holder are SR-M3f-2 (planned). Internally-fed / abstract-magazine precursor also shipped. -->
