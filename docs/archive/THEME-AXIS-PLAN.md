> **ARCHIVED 2026-06-01 — superseded by [`docs/BACKLOG.md`](../BACKLOG.md).**
> The five themes this defined (A–E) have all shipped. Its durable value — the picking
> rubric, parallelism rules, warmup pattern, and anti-patterns — has been folded into
> `BACKLOG.md`, where themes are now derived by clustering open items rather than frozen
> as a fixed five. Kept for the original theme write-ups and reasoning.

---

# Theme-Axis Planning Method

A planning method for working down `docs/TAPESTRY-GAP-MATRIX.md`. The matrix groups
gaps by **category** (big-ticket / specced-but-unbuilt / unspecced / ops). This
document groups the same gaps by **user-visible theme** so we can ship coherent
slices instead of bouncing across categories.

**Companion to:** `docs/TAPESTRY-GAP-MATRIX.md` (the source-of-truth gap list).

---

## Why themes instead of bucket order

Walking the matrix top-to-bottom (section 1 → 4) stalls for three reasons:

1. **Cross-bucket dependencies.** GMCP packages (matrix §1.2) can't ship before
   telnet IAC negotiation (§1.3), which is a sibling not a predecessor in
   bucket order.
2. **Mixed-size items.** Section 1 has 8 multi-week projects; section 2 is a long
   tail of one-paragraph fixes; section 3 needs design pre-decisions before any
   spec can land. Working linearly mixes weeks with hours and stalls before
   reaching anything new.
3. **No user-facing throughline.** The buckets describe the *kind* of gap; users
   experience features as themes. "We added GMCP" is harder to explain than
   "we shipped modern-client support."

Themes pull items from across all four matrix buckets into a single arc with one
end-user outcome. Each theme is shippable on its own and produces something
demonstrable.

---

## The five themes

Each theme is presented with: its hook (the user-facing outcome), the matrix items
it pulls in, a suggested internal sequence, open pre-decisions, and a rough shape
estimate. Citations like `M§1.2` refer to sections of `TAPESTRY-GAP-MATRIX.md`.

### Theme A — Social MUD

> Players can talk to each other across the world, not just in their current room.

**Pulls from matrix:**
- §1.6 Chat channels / tells / notifications
- §1.7 Emotes
- §3 Notifications queue (the underlying primitive)
- §3 Channels, tells, who (the GMCP route)

**Internal sequence:**
1. Pre-decision: channels content-defined (per-pack) or engine-fixed
   (newbie/ooc/admin)?
2. Spec the notification queue (per-entity priority queue) — small, isolated.
3. Spec channels + tells on top of the queue.
4. Spec emotes (registry shape + actor/target/room substitution).
5. Implement queue → tells → channels → emotes.

**Why this order:** The notification queue is the substrate everything else
publishes through. Tells are the simplest channel (1:1) and exercise the whole
pipeline. Multi-recipient channels follow. Emotes are independent and can land
last or in parallel.

**Open pre-decisions:**
- Channel ownership model (engine vs. pack)
- History retention (per-channel buffer? persisted?)
- Ignore / block surface
- GMCP routing vs. plain-text-only first

**Shape:** 4-6 weeks. Two specs to write, then incremental implementation. Pure
engine work, no protocol-level changes required (works on bare telnet).

**Demo target:** Two players in different rooms chat over `ooc`; one tells the
other privately; one emotes; both see history on reconnect.

---

### Theme B — Modern Client

> Mudlet, MUSHclient, Blightmud, and browser clients see real HUDs and panels.

**Pulls from matrix:**
- §1.3 Telnet IAC negotiation (TTYPE, NAWS, ECHO, MSSP, GMCP)
- §1.2 GMCP package layer
- §1.4 WebSocket transport
- §2 networking-protocols MSSP variables
- §2 ui-rendering-help 256/truecolor (now actually consumable)

**Internal sequence:**
1. Telnet IAC: TTYPE + NAWS first (cheapest, immediate panel-width win).
2. MSSP variables next (one milestone, then we appear on MUD listings).
3. GMCP option negotiation + envelope.
4. GMCP packages in priority order: `Char.Vitals` → `Room.Info` →
   `Char.Items` → `Char.Combat` → `Char.Effects` → `Char.Experience` → rest.
5. WebSocket transport (parallel; doesn't block GMCP work since the
   `internal/conn.Connection` interface is clean).
6. 256-color / truecolor as a follow-up once clients advertise capability.

**Why this order:** Capability detection (TTYPE/NAWS) is the cheapest and improves
plain telnet UX immediately. MSSP gets us discoverable. GMCP is the headline.
Per-package work is incremental and each lands a visible slice. WebSocket is a
parallel track; the cleaner abstraction in `internal/conn` means it doesn't have
to wait.

**Open pre-decisions:**
- Per-client subscribe model: every package on by default vs. opt-in
- Payload shape: clone Tapestry's JSON or design our own thinner one
- Dirty-batching strategy for `Char.Vitals` (stampede vector — must be there at
  v1, not retrofitted)
- HTTP listener: same process as telnet vs. separate binary?

**Shape:** 6-10 weeks. Telnet-negotiation + MSSP is ~1-2 weeks; GMCP transport is
~1-2 weeks; each package is small but there are many; WebSocket is ~1-2 weeks
parallel. Touches `internal/conn`, `internal/server`, new `internal/gmcp` package.

**Demo target:** Mudlet client connects, requests `Char.Vitals` package, sees
HP/Mana/MV bars update in real time as the player takes damage.

---

### Theme C — World Depth

> The world has state beyond rooms and exits — doors that open, weather that
> changes, portals that expire, locations players can recall to.

**Pulls from matrix:**
- §1.8 Doors + locks
- §3 Portals / temporary exits
- §3 Weather (per-zone)
- §3 Recall / return-home

**Internal sequence:**
1. Pre-decisions: door home (exit property / room / entity?), weather granularity
   (per-area or per-room?).
2. Spec doors + locks (smallest, most contained).
3. Implement doors with open/close/lock/unlock verbs + key entities.
4. Spec portals (extends exits with TTL).
5. Implement portals + portal cleanup tick handler.
6. Spec recall (return-address service) + `recall` verb.
7. Spec weather (per-zone state, tick-driven evolution, room-render integration).
8. Implement weather.

**Why this order:** Doors are the simplest and exercise exit-state sync. Portals
reuse exits with TTL — they piggyback on door infrastructure. Recall is tiny.
Weather is largest and most cross-cutting; saving it for last lets the prior
work surface conventions.

**Open pre-decisions:**
- Door home (exit property vs. service-over-exit-pairs as Tapestry does)
- Weather model (coarse area enum vs. fine per-room w/ neighbor influence)
- Whether recall is a verb only or has cooldown / cost / scripting hooks

**Shape:** 4-6 weeks. Mostly new but bounded specs. Touches `internal/world`,
new `internal/door`, `internal/weather`, `internal/recall` (or a single
`internal/worldfx` umbrella).

**Demo target:** A locked door between two rooms; player uses a key item to
unlock; weather in the outer zone shifts from clear to storm on a tick; player
recalls to their saved location.

---

### Theme D — Content Authoring

> Pack authors can write scripts, hot-reload changes, and rely on safe typed args.

**Pulls from matrix:**
- §1.1 Scripting runtime (the big one)
- §1.5 Arg typing + resolution
- §2 scripting-and-packs hot reload
- §2 scripting-and-packs cross-pack reference validation
- §3 Schedule primitive (depends on scripting)
- §3 Admin verbs (depends on role system, parallel)

**Internal sequence:**
1. **Pre-decision: scripting language.** Three candidates: gopher-lua, goja (JS),
   Starlark. Decision criterion: content-author ergonomics, not engine
   ergonomics. Probably needs an actual content author in the room.
2. Spec the scripting sandbox + engine API surface obligations (memory cap,
   instruction cap, I/O denied, error attribution).
3. Spec arg typing + resolution (`commands-and-dispatch §5`).
4. Implement scripting runtime + minimal engine API (rooms, items, events).
5. Implement arg typing — staged migration of existing handlers.
6. Implement hot reload.
7. Schedule primitive on top of scripting.

**Why this order:** Scripting is the keystone. Arg typing can land in parallel
or after — they don't depend on each other but they're both content-authoring
quality-of-life. Hot reload requires the runtime to be stable first. Schedule
is a thin shim once scripting works.

**Open pre-decisions:**
- Scripting language (do not decide alone — find a willing pack author first)
- Sandbox shape (capability-based vs. allowlist of bindings)
- Script-event subscription model: callbacks vs. coroutines vs. message-passing
- Per-pack vs. per-script execution context
- Whether arg typing is a Context accessor or a generated typed struct per
  handler

**Shape:** 10-16 weeks. This is the largest theme. Scripting alone is 6-10
weeks; arg typing is 2-4; hot reload is 1-2; schedule is 1-2. Touches almost
every package downstream. Engine-debt theme (E) should land first if doing in
parallel becomes a problem.

**Demo target:** A pack author writes a quest script in <language>, the engine
hot-reloads it, the script subscribes to `mob.killed` events, the script
schedules a follow-up via `engine.schedule(300, …)`.

---

### Theme E — Engine Debt

> Close the import cycle, sync vitals to stat-block max, give mobs a real
> StatBlock, build the property registry. Pure cleanup that unblocks future
> themes.

**Pulls from matrix:**
- §2 abilities-and-effects mob effect-stat install (blocked by cycle)
- §2 progression vital re-clamp under max-affecting recompute
- §2 progression mob `StatBlock` consumer
- §2 mobs-ai-spawning stat derivation from race+class
- §2 economy-survival effect-id registry for consumables
- §2 persistence property registry + tagged-value envelope
- §2 quests `quest_grant` on item/room (needs property bag)

**Internal sequence:**
1. Hoist the interface that closes the entities↔progression↔stats cycle to a
   leaf package (same pattern as `internal/srckey`). This unlocks mob effects.
2. Add `StatBlock` to mobs + derive from race+class.
3. Subscribe vitals to max-affecting stat changes; re-clamp current HP.
4. Build the consumable `EffectTemplate` registry distinct from `Ability.Effects`.
5. Build the property registry on persistence (enables `world.Room.Property`).
6. Add `quest_grant` on item + room (now possible).

**Why this order:** Step 1 unblocks the most other work. Steps 2-3 close the
m8-1 and m9-4 deferrals. Steps 4-5 are independent and can run in parallel.
Step 6 lands once the property bag exists.

**Open pre-decisions:**
- None. This is debt closure against known-good designs.

**Shape:** 3-4 weeks. Smallest theme. High signal/noise — clears the backlog
that's been quietly blocking future work for several milestones.

**Demo target:** No user-visible demo. Internal: the import cycle is gone;
mob effects actually install stat modifiers; max-HP changes update current-HP;
a consumable's `effect_id` actually applies when consumed.

---

## Parallelism rules

- **Only one main theme runs at a time.** Themes are large arcs; splitting
  attention between two stalls both.
- **Ops (matrix §4) always runs in background.** Container build, observability,
  repo hygiene. Land any time without spec changes; doesn't block anything.
  Treat as filler work between theme commits.
- **Warmups can run between themes.** A 30-60 minute matrix §2 fix (prompt verb,
  command-help syntax polish, a tiny bug-close) is good for context
  recalibration when switching themes.

---

## Warmup pattern

Before committing to a new theme, take **one 30-90 minute slice** from matrix §2
to recalibrate:

- prompt verb (`ui-rendering-help §7.6` — schema exists, just needs a verb)
- doors spec sketch (write the spec without implementing, surfaces design)
- per-phase idle timeout on login
- container weight/volume caps (small, contained inventory work)
- 256-color renderer extension (just the renderer, no theme content)

The warmup serves two purposes: lower-stakes work to re-enter the codebase, and
a signal of which theme feels closest to where energy is. If the warmup wants to
keep going into a larger arc, follow it.

---

## Picking the next theme

Use this rubric (yes-strongly to one question wins):

| Question | If yes → start with |
|---|---|
| Are players actually trying to play right now? | Theme A (Social) — multiplayer feel matters most |
| Are MUD-client users asking for HUDs? | Theme B (Modern Client) |
| Does the world feel flat / static in playtest? | Theme C (World Depth) |
| Are content authors blocked / are we blocking on undecided language? | Theme D (Content Authoring) |
| Is technical debt blocking a feature you wanted to ship in another theme? | Theme E (Engine Debt) |

If multiple yeses, prefer the theme with the smallest scope to land a real win
before committing further: **E < C ≈ A < B < D**.

If no yeses, the answer is probably Theme E (close the debt while the picture
is clear) followed by Theme A (social play is the highest-leverage product
addition for a single-developer MUD).

---

## Tracking a theme

When a theme starts:

1. Create a `docs/archive/themes/<theme>-plan.md` with the internal sequence, current
   step, and open pre-decisions.
2. Add a top-line entry to `docs/ROADMAP.md` under a new milestone heading
   (e.g. `## M13 — Social MUD`).
3. Move any matrix items the theme covers into the new milestone in the ROADMAP
   so they get standard `[ ]/[x]` tracking.
4. Update `docs/TAPESTRY-GAP-MATRIX.md` snapshot date when a theme closes;
   strike or shrink the items the theme delivered.

When a theme ends:

1. Move closed items out of `docs/TAPESTRY-GAP-MATRIX.md`.
2. Archive `docs/archive/themes/<theme>-plan.md` (or leave for history).
3. Open question: pick the next theme via the rubric above.

---

## Anti-patterns to avoid

- **Cherry-picking across themes.** Doing one GMCP package, then one chat
  feature, then one door fix produces breadth without throughline. Pick a theme.
- **Spec'ing without an author.** Theme D's scripting decision is the canonical
  trap — don't pick the language alone.
- **Skipping the engine-debt theme indefinitely.** Each blocked deferral
  accretes interest. Theme E is the cheapest theme; it should land at least
  once every two or three other themes.
- **Running the ops stack as a "real" theme.** It's background work. If it
  takes more than a calendar week of foreground attention something's wrong
  with the scope.
- **Defining a sixth theme.** The five above already cover everything in the
  matrix. Adding "Quests v2" or "Combat polish" as a theme reintroduces the
  cherry-picking failure mode this method exists to prevent.

---

*End of plan. Update the rubric, the theme contents, and the parallelism rules
as we learn what works. The themes are not sacred — if the matrix shifts
substantially we should re-derive them, not retrofit.*
