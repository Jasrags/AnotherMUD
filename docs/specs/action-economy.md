# Action Economy (timed actions · the busy state)

A **generic per-actor timed-action substrate**: the engine's first shared notion
of *"this actor is occupied doing X until tick T, and cannot start another timed
action until then."* It generalizes the three ad-hoc occupation trackers the
engine already grew piecemeal — the **flee cooldown** (`combat.md`), the **cast /
weave warmup** (`abilities-and-effects.md`, the One Power interrupt game), and the
**timed craft** (`crafting-and-cooking.md` §3) — into one primitive that new
*action-economy-shaped* features can consume instead of re-rolling their own.

It exists because two specced features have been **blocked on exactly this**: the
**crossbow load actions** and **don/doff timers** in the special-weapons /
armor-depth tail (`special-weapons.md`, `armor-depth.md` §7, BACKLOG §2) both
need "a per-actor *busy* state + the tick scheduler" the engine did not model.
This spec builds that state once; those features become consumers.

Governed by EPIC **Decision 0** (translate WoT onto the existing tick/chance
model; no d20 action-economy rewrite). The tabletop's standard / move / full-round
/ free action grid is **deliberately not ported** — see §1. *Spec ahead of code —
build pending; sliced so each piece ships independently.*

## 1. Overview

### The model: an occupation timer, not a d20 action grid

The d20 source models a turn as a budget of typed actions (a standard + a move +
swift + free, or one full-round) spent within a 6-second round. That grid only
means something in **turn-based** play where a turn is an atomic allotment. This
engine is **real-time tick/chance**: there are no turns, combat resolves on a
cadence (`combat.md` §3), and a player types verbs whenever they like.

Porting the action grid would be bookkeeping with no payoff (Decision 0). What
**does** translate is the consequence the grid produces: *some actions take time,
and while you are doing one you cannot do another.* A crossbow is slow because
**reloading occupies you**; donning a hauberk is slow because **strapping it on
occupies you**. The meaningful choice — commit to a slow action and be exposed
while it completes — survives; the per-action-type accounting does not.

So the substrate is a single **occupation timer** per actor: *busy doing `Kind`
until tick `ReadyAt`.* One at a time. Starting a new timed action while busy is
refused. When the timer elapses, the action **completes** (its consumer-defined
effect fires). Some timed actions can be **interrupted** before they complete.

### The shape, borrowed from what already works

The two best existing analogs define the shape:

- **Timed craft** (`crafting-and-cooking.md` §3) is the **two-phase** pattern:
  `BeginCraft` runs read-only gates and returns a token; the command layer records
  a `PendingCraft{ReadyAt}`; a tick sweep calls `CompleteReady` when `now >=
  ReadyAt`, which performs the *actual* mutation (consume inputs, produce output)
  against the actor's **current** state. No state is reserved at begin — a
  lazy-completion model where an action that can no longer complete simply refuses
  cleanly at completion, losing nothing.
- **The cast tracker** (`abilities-and-effects.md`, the One Power) is the
  **interruptible** pattern: a central per-entity map of in-flight casts, advanced
  each round, **cleared by `Interrupt`** when the caster is hit, dropped on logout,
  never persisted.

This spec's substrate is the **union of those two shapes**, lifted out of their
domains: tick-stamped completion like craft, optional interruption like cast,
central per-entity storage like cast, transient like both.

### Goals / non-goals

**Goals.** A leaf substrate (no dependency on combat, items, session, or any
consumer) that records *one in-flight timed action per actor*; gates a second
action while one is in flight; completes due actions on a tick sweep, routing the
completion to the action's consumer; supports interruption (movement, manual
cancel) for actions that allow it; and is queryable so the command layer can
refuse action-taking verbs with a uniform "you are busy" while occupied.

**Non-goals.** No d20 action-type grid (standard / move / full-round / swift /
free). No multiple simultaneous timed actions per actor (one slot). No
persistence — an in-flight action at logout/crash is lost (§6). No migration of
the existing flee-cooldown / cast / craft trackers onto this substrate in v1 —
they keep working as-is; they are recorded migration candidates (§7), not v1
scope. No attack-of-opportunity / "provokes" mechanic (the source ties reload and
spellcasting to AoO; the engine has no AoO and this spec does not add one).

## 2. The action record and the tracker

A **timed action** is the small transient record of one actor's occupation:

- a **kind** — an opaque, consumer-owned tag (e.g. a reload kind, a don kind) the
  completion sweep routes on;
- a **ready-at tick** — the engine tick the action completes on (`begin tick +
  duration`);
- an **interruptible** flag — whether movement / a manual cancel aborts it
  (§5);
- a small **label** — a human string for the "you are busy <label>" refusal and
  the completion/interrupt notices;
- optional **consumer payload** — opaque identifiers the consumer needs at
  completion (e.g. which item, which slot), captured at begin so completion need
  not recompute world-coupled state off the tick goroutine (mirrors
  `PendingCraft.StationTier`).

The **tracker** is process-wide: per-entity state in one id-keyed map guarded by a
mutex, mirroring the cast tracker. An entity has **at most one** in-flight action.

### Operations

- **Begin** — record an action for an actor. **Refuses** (returns "already busy")
  when one is in flight; the caller is expected to gate first, but the refusal is
  the safe degenerate. (Compare `SetPendingCraft` returning false.)
- **Active** — return the in-flight action without mutating it (false when idle).
- **IsBusy** — boolean form of Active, for the dispatch gate.
- **CompleteReady** — for a given actor and `now`, if an action is in flight and
  `now >= ReadyAt`, **claim it** (clear-before-complete, single-winner) and return
  it for routing; otherwise return "nothing due." Clearing before the consumer
  runs guarantees a failed completion never loops (compare craft `CompleteReady`).
- **BusyEntities** — the ids of every actor with an in-flight action, so the tick
  sweep iterates only the occupied (not every logged-in actor every tick).
- **Interrupt** — clear an actor's in-flight action and return it (false when
  none), for the movement / manual-cancel / forced-abort path. **Honors the
  interruptible flag**: a non-interruptible action is *not* cleared by the
  movement/cancel callers (it still completes on its timer); a forced drop
  (logout, death) clears regardless.
- **Drop** — unconditionally remove an actor's action (logout / death cleanup).

### Acceptance criteria

- [ ] A fresh tracker reports every actor idle (`IsBusy` false, `Active` false).
- [ ] `Begin` on an idle actor records the action; `Active`/`IsBusy` then report
      it; a second `Begin` while in flight is refused and leaves the first intact.
- [ ] `CompleteReady` returns "nothing due" before `ReadyAt` and the claimed
      action exactly once at/after `ReadyAt`; a second `CompleteReady` after the
      first returns "nothing due" (single-winner clear).
- [ ] `Interrupt` on an **interruptible** in-flight action clears it and returns
      it; on a **non-interruptible** action via the interruptible path it is a
      no-op and the action remains in flight; `Drop` clears either kind.
- [ ] `BusyEntities` returns exactly the occupied ids; empty when none.
- [ ] All operations are safe under `-race` with concurrent begin / complete /
      interrupt on different actors and on the same actor.

## 3. The completion sweep (tick handler)

A tick handler runs at a fine cadence and, for each occupied actor whose timer has
come due, **routes the completed action to its consumer by kind**. The substrate
does not know what a reload or a don *is* — it returns the claimed action; a
**router** owned by the wiring layer switches on `Kind` and calls the consumer's
completion (the crossbow service marks the weapon loaded; the armor path performs
the deferred equip). This keeps the leaf substrate free of combat/item imports,
exactly as the craft sweep lives in `session` and calls into `crafting`.

The sweep iterates **only `BusyEntities`** (typically empty or a handful), so its
per-tick cost is proportional to occupied actors, not to all logged-in players.

Cadence is configurable; it should be fine enough that a sub-second action feels
responsive but need not be every tick (a timed action's granularity is the sweep
cadence). The completion delivers a message to the actor and, where the consumer
wants it, a room notice (compare the craft sweep's "finishes some careful work").

### Acceptance criteria

- [ ] An action with `ReadyAt = N` completes on the first sweep at tick `>= N`,
      not before; its consumer effect is applied exactly once.
- [ ] The sweep visits only occupied actors; with nobody occupied it does no
      per-actor work.
- [ ] An action whose consumer can no longer complete it (the precondition lapsed
      — e.g. the item was dropped) refuses cleanly at completion, applies no
      effect, and does not re-queue (clear-before-complete).
- [ ] A consumer with no registered router entry for a kind is a logged no-op, not
      a panic (defensive — every shipped kind registers a router).

## 4. The dispatch gate ("you are busy")

While an actor has an in-flight timed action, **action-taking verbs are refused**
with a uniform message naming the occupation ("You are busy reloading."). The
refusal is a **pre-handler check** in the command layer, opt-in per command via a
flag (mirroring the existing per-command gates `RequireInCombat` /
`RequireNotInCombat`): a command marked as an *action* is gated; passive/parser
commands (look, score, say, channels, quit) are **not** — a busy player can still
look around, talk, and disconnect.

Manually issuing the *cancel* verb (§5) is always allowed while busy. Re-issuing
the same begin verb while busy is refused by the gate (and `Begin` would refuse
anyway — defense in depth).

### Acceptance criteria

- [ ] A command flagged as an action, issued while busy, is refused with the
      "you are busy <label>" message and does not run its handler.
- [ ] Passive commands (look / score / say / quit / the cancel verb) run normally
      while busy.
- [ ] An action command issued while idle runs normally.

## 5. Interruption

Some occupations break if you move or choose to stop; others run to completion
regardless:

- **Movement** interrupts an **interruptible** in-flight action (you can't keep
  strapping on a breastplate while walking to the next room). The mover is told
  what was interrupted. A non-interruptible action survives movement (or movement
  itself is refused — a consumer choice; the substrate supports both by exposing
  the flag, the consumer/gate decides).
- **A manual cancel verb** (`stop`/`cancel` — naming a §7 open question) clears an
  interruptible action and tells the actor. On a non-interruptible action it
  reports that the action cannot be stopped.
- **Forced drop** — logout, link-death, and death **always** clear the action
  (via `Drop`), interruptible or not; it is simply lost (no completion, no
  refund), matching how an in-flight cast/craft is lost today.

Whether **being hit** interrupts a timed action is a **per-consumer** decision and
is *not* a substrate behavior: the cast tracker interrupts on hit (the One Power
interrupt game) by calling `Interrupt` from the combat hit path; a crossbow reload
deliberately does **not** (you can reload while being shot at — that exposure is
the point). The substrate provides `Interrupt`; the consumer wires (or declines to
wire) the hit path. v1 consumers (reload, don) do **not** interrupt on hit.

### Acceptance criteria

- [ ] Moving rooms interrupts an in-flight interruptible action and notifies the
      actor; a non-interruptible action is unaffected by the interruptible path.
- [ ] The cancel verb stops an interruptible action and reports it; on a
      non-interruptible action it reports the action cannot be stopped.
- [ ] Logout / death clears any in-flight action regardless of the flag, with no
      completion effect.
- [ ] Being hit does **not** interrupt a v1 reload or don (no consumer wires the
      hit path for them).

## 6. Persistence (none)

An in-flight timed action is **transient** and never persisted, like the cast and
craft trackers. A player who logs out / crashes mid-action loses it (no save
version bump). Because the lazy-completion model reserves nothing at begin (no item
removed, no slot vacated until completion), nothing is lost with it — the world
state is exactly as it was before the action began. On reconnect the actor is idle.

### Acceptance criteria

- [ ] No save-surface field is added; player save version is unchanged.
- [ ] An actor logging out mid-action reconnects idle; no item/slot/state was
      consumed by the abandoned action.

## 7. Consumers (the motivating features)

The substrate ships with two consumers, each its own slice on this seam. Both are
specced in their home docs; this section records only how they ride the substrate.

### 7.1 Crossbow load actions (`special-weapons.md`, ranged tail)

A crossbow holds **one loaded shot**; firing consumes the loaded state, after
which the weapon must be **reloaded** before it can fire again — and reloading is
a timed occupation (a light crossbow loads faster than a heavy one). This is the
mechanical distinction between a crossbow (slow, powerful) and a bow (fire each
round) the engine has lacked.

- The weapon carries a transient **loaded** flag (in-flight state, not a save
  field — like the action itself).
- **Fire while loaded** → resolves the shot (the existing ranged path), clears
  loaded.
- **Fire while unloaded** → refused, prompting a reload.
- **Reload** → a `Begin` of a reload-kind action whose duration is the weapon's
  load time; on completion the router sets the weapon loaded. Reload is **not**
  interrupted by being hit (§5); movement interrupts it (you lose the half-loaded
  shot — interruptible).
- The 1-handed-crossbow and repeating-crossbow refinements (the source's −4 and
  the repeater's magazine) are deferred follow-ups noted in `special-weapons.md`.

### 7.2 Don/doff timers (`armor-depth.md` §7)

Today (`armordon.go`) slow armor (medium/heavy tier) **cannot be changed in
combat** — a binary Decision-0 gate that drops the timer bookkeeping. The timed
slice **layers a short occupation onto donning/doffing** so that even out of
combat, strapping on a hauberk takes a beat, and the act exposes you: the equip /
unequip mutation is **deferred to completion** (two-phase, like craft) rather than
applied instantly.

⚠️ **Decision-0 tension (open).** The existing combat gate may already *be* the
meaningful choice the repo wanted ("you can't armor up once a fight is on you"),
in which case a wall-clock don timer is bookkeeping Decision 0 told us to drop.
The reconciliation, if built: don/doff timers are **short** (seconds, real-time
tempo) and exist mainly to (a) make the act interruptible/exposed and (b) host the
source's **hasty-don** (faster, worse fit) and **helper-assist** (halved time)
choices — which *are* meaningful. **Recommend confirming scope before building
7.2**; the substrate and 7.1 do not depend on it. If built, donning is
interruptible (movement aborts it) and the in-combat gate from `armordon.go` is
retained for slow armor.

### Acceptance criteria

- [ ] (7.1) A loaded crossbow fires once and becomes unloaded; firing it again is
      refused until a reload completes; reload occupies the actor for the weapon's
      load time and is gated by the busy state.
- [ ] (7.1) Reloading is interrupted by movement and **not** by being hit.
- [ ] (7.2, if built) Donning slow armor is a timed occupation whose equip lands
      at completion; movement interrupts it; the in-combat slow-armor gate is
      retained.

## 8. Configuration surface

| Knob | Governs | Default |
|---|---|---|
| `ANOTHERMUD_ACTION_SWEEP_CADENCE` | tick cadence of the completion sweep (§3) | a fine cadence (sub-second), e.g. matching the combat cadence |
| `ANOTHERMUD_CROSSBOW_RELOAD_TICKS` *(consumer 7.1)* | default crossbow reload duration when a weapon declares none | a small number of seconds |
| `ANOTHERMUD_DON_TICKS` *(consumer 7.2, if built)* | default don/doff duration for slow armor | a small number of seconds |

Per-weapon load time and per-armor don time are **content** (item template fields),
falling back to the knob default — values themselves are not specced here (spec
convention: numbers live in content / the config table, not the narrative).

## 9. Open questions

- **Cancel verb naming + collision.** `stop` (collides with nothing today?) vs
  `cancel` vs reusing an existing verb. Does it cancel *only* the timed action, or
  also disengage combat / clear the action queue? Recommend: a single `stop` that
  cancels the in-flight timed action only, leaving combat/queue alone.
- **Movement vs non-interruptible.** When an actor with a non-interruptible action
  tries to move, does movement *refuse* ("you can't leave mid-X") or does it
  *proceed and let the action complete in the new room*? The substrate supports
  both; pick per consumer. (v1 consumers are interruptible, so moot for v1.)
- **Migrating the existing trackers.** Should flee-cooldown / cast / craft be
  re-pointed onto this substrate once it proves out? They have subtly different
  shapes (flee is a cooldown-after, not an occupation-during; cast counts rounds,
  not ticks; craft stores on the actor, not the central map). A migration is
  plausible but is **explicitly out of v1 scope** — revisit only if a fourth/fifth
  consumer makes the duplication hurt.
- **Multiple queued actions.** v1 is strictly one-at-a-time with no queue. A
  "reload then fire" convenience (queue the fire behind the reload) is a possible
  later affordance; deferred.
- **GMCP surface.** Should an in-flight action publish a GMCP progress signal
  (start / remaining / complete) for modern clients to render a cast-bar? Deferred;
  the substrate's `Active` exposes enough to add it later.
