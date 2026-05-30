# Deferred-Items Backlog

A consolidated, point-in-time snapshot of the **open** deferred items across
all milestones (M0→M12), distilled from the per-milestone
`m<N>-deferred-fixes.md` memory files. The memory files remain the source of
truth (full context, file:line, fix-when triggers); this is the scannable
index.

**Generated:** 2026-05-28 (after cluster 1 + cluster 2 + the ItemInstance
mutex landed). Regenerate by re-scanning the memory deferral files.

Excludes everything marked RESOLVED/FIXED/CLOSED (~25 items, including the
recent cluster 1, cluster 2, and ItemInstance resolutions).

---

## Open HIGH / CRITICAL

**None.** The last HIGH (`ItemInstance.Properties()` unguarded map, `m11-5`)
was closed; `m9-1` CRITICAL (Drop/autosave race) and `m5 H1` (GetHandler
TOCTOU) were fixed pre-commit earlier.

---

## Open MEDIUM

### Persistence / data integrity
- `m5 M1` — `syncInventoryToSaveLocked` can silently lose items on a partial sync
- `m5-9c #1` — per-instance item property persistence (charges/fill don't round-trip)
- `m5-9c #4` — finite-source `fill_supply` decrement TOCTOU
- `m5-6 M1` — duplicate persisted entity-ids leak modifiers
- `m9-1 #2` — Persist diffs the full ability snapshot every autosave
- `m9-1 #4` — takeover `Restore` can clobber concurrent manager state

### Combat / death hardening (mostly doc-contract + missing-test; no known live bug)
- `m7-2 #1` — combat `EventSink` contract is documentation-only
- `m7-2 #2` — unbounded combat-list growth (DoS surface)
- `m7-3 #1` — `removeFromListLocked` aliases the backing slice
- `m7-5 #2` — `DeathCheck` cancellation contract is doc-only
- `m7-5 #4` — concurrent killing-blow unit test missing
- `m7-5 #5` — `VitalsState` YAML decode is unbounded
- `m7-5 #6` — bus `Publish` re-entrancy contract on `productionCombatSink`
- `m7-6 #3` — `FleeOutcomeFailedUnknownRoom` overloaded with Mover-refusal
- `m7-6 #4` — `Heartbeat.Tick` reaches into `Manager.cooldowns` (encapsulation)
- `m7-6 #5` — mob flee announce uses generic "Something bolts away!"

### Security
- `m7-followup #2` — `EventKill` has no authorship sentinel

### Effects / abilities / mobs
- `m8-1 #2` — vital re-clamp under a max-affecting recompute (no consumer yet)
- `m9-3 #1` — `queue.Pop` allocates a fresh slice on every shrink
- `m9-4 #1` / `m9-5 #1` — integer hit-chance math diverges from spec's float model; passive gain omits the §3.5 stat factor
- `m9-4 #2b` — `connActor.Alignment()/EquippedTags` hold `a.mu`, called every pulse from the tick goroutine
- `m9-6 #1` — damage/heal handler + death bridge are integration-only (no unit coverage)
- `m11-5` — consumable `effect_id` application unwired (no effect-template registry)

### Alignment / progression
- `m8-2 #2` — `DeductExperience` can't de-level (spec open question)
- `m8-2 #3` — `XPFormula` not loadable from YAML
- `m8-4 GetEligible` — no production consumer (M12.3 used `All()`, not `GetEligible`)
- `m8-5` — alignment history runtime-only; admin-bypass role tag (no role system); alignment seed not re-applied on takeover/link-dead

### World / spawn / creation
- `m6-5 #2` — `mob.aggro` has no engine subscriber
- `m6-6 #2` — scheduler `deltaTicks` coupled to handler cadence
- `m6-6 #5` — `SpawnRule.ResetInterval` decoded but unused
- `m10-1` — `IsKnown/Resolve` render-cache TOCTOU (boot-only today)
- `m10-9` — `quest_grant` on a room (needs a room *property* bag — cluster 2 added room *tags*, not properties)
- `m12-2` — MOTD enqueue not implemented; trigger-keyed flow registry (single nil-able flow); any-room spawn-room last-resort
- `m12-3` — §5 structured flow-step events / GMCP wizard panel

---

## Open LOW (compressed)

- `m0` DT-1..DT-4 — flaky test sleep, `logging.Default` mutable global, telnet goroutine-per-Read, telnet Write ctx-cancel
- `m1` — fmt.Errorf cleanup, busy-poll test, `world.Room` exported fields + live-pointer return
- `m5 M2/M3/M4` — migrate-bump verify, doc inventory/Placement invariant, Env-omits-Templates note
- `m5-8` — HolderID namespace (players-as-entities)
- `m5-9a` — give-mid-logout dangle
- `m5-9c #3` — fill-from-carried-container open question
- `m6-5 #3/#4/#5` — players-no-general-tags (partially addressed cluster 2), anonymous struct return, evaluator unsubscribe
- `m6-6 #3/#4/#4a/#6` — O(rooms×occupants), spawn subscription handle, two mob.spawned publishers, tracker key GC
- `m7-1 #2/#4` — MobInstance.stats mutex (mooted by cluster 1's StatBlock), vitalsDescriptor bare floats
- `m7-2 #3` — SplitCombatantID helper
- `m7-5 #3/#7` — process-lifetime bus subs, mob.killed placement.Remove best-effort
- `m7-6 #6/#7` — flee-cooldown upper bound, WimpyThreshold no-bump forward-compat
- `m7-followup #3/#4` — AttachCombat sync, SetRoom/SendToRoom announce ordering
- `m8-3` — ApplyRacialFlags alignment=0 skip, StatCaps no upper bound
- `m8-4` — stat-growth no-gate (doc), class-swap path absent
- `m8-5` — bucket round-trip on Set/Shift, ResolveBuckets snapshot-at-call
- `m8-6` — cap-tier ladder hardcoded, no `training.complete` event
- `m9-1 #3` — AbilitySnapshot lowercase ability ids
- `m9-2 #5` — HasFlag lock fast-path
- `m9-5 #2/#3` — PassiveScalingBonus unwired, mob passives inert (no mob prof map)
- `m10-9` — quest_advance pickup payload (needs scripting)
- `m11-2` — shop name prefix-on-full-name resolution
- `m11-4` — combat-wake wakes target only, logout TOCTOU (harmless), rest-verb furniture id
- `m11-5` — regen persists only on autosave, regen gated off in combat
- `m12-2/m12-3` — actor visible before character.created publishes, Option.Description not surfaced

---

## Accepted design / open questions (not really debt)

Cap-tier ladder hardcoded; class-swap path absent; alignment history
runtime-only; abilities resolve combat-only; regen gated off in combat; shop
prefix resolution; `safe` vs `safe-room` tag-string divergence (combat checks
`"safe-room"`, training `"safe"` — a room must declare both).

---

## Caveats / verify-before-acting

1. **Likely stale-open M8 items.** Several were written as "X is a nop *until*
   M8.4/M8.6/M9," and those milestones have since landed (e.g. `m8-2 #1` class
   LevelUp subscriber, `m8-3` StatCaps / CastCostModifier consumers, `m6-4`
   MobInstance race). The memory files weren't always updated to mark them
   resolved — re-verify against current code before treating as real work.
2. **`m7-1 #2`** (MobInstance.stats mutex) is effectively mooted by cluster 1 —
   mobs now use a `*progression.StatBlock` with its own lock.
3. For M1–M8 entries this index reads from file headers + cross-references; a
   few may have been quietly resolved in a later sweep. Post-M9 entries are
   high-confidence.
