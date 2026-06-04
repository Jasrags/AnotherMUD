# Deferred-Items Backlog

A consolidated, point-in-time snapshot of the **open** deferred items across
all milestones (M0→M22), distilled from the per-milestone
`m<N>-deferred-fixes.md` memory files. The memory files remain the source of
truth (full context, file:line, fix-when triggers); this is the scannable
index.

**Generated:** 2026-05-28 (M0→M12 body); **post-M12 section added 2026-06-02**
(M13→M22, from the memory index); **tab-completion deferrals added 2026-06-03**.
Regenerate by re-scanning the memory deferral files.

Excludes everything marked RESOLVED/FIXED/CLOSED. Note: several M0→M12 items
below were later resolved by M14 (Engine-Debt) and the M9.x mob-effect sweep —
see the per-item RESOLVED tags and the caveats at the bottom.

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
- ~~`m8-1 #2` — vital re-clamp under a max-affecting recompute~~ **RESOLVED (M14.1 OnMaxChange)**
- `m9-3 #1` — `queue.Pop` allocates a fresh slice on every shrink
- `m9-4 #1` / `m9-5 #1` — integer hit-chance math diverges from spec's float model; passive gain omits the §3.5 stat factor (mob effect-stat half **RESOLVED 2026-06-01**)
- `m9-4 #2b` — `connActor.Alignment()/EquippedTags` hold `a.mu`, called every pulse from the tick goroutine
- `m9-6 #1` — damage/heal handler + death bridge are integration-only (no unit coverage)
- ~~`m11-5` — consumable `effect_id` application unwired~~ **RESOLVED (M14 effect-template registry)**

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
- ~~`m10-9` — `quest_grant` on a room~~ **RESOLVED (M14)**; `quest_advance` pickup payload still needs scripting (LOW)
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

## Post-M12 (M13→M22) — open items

Indexed 2026-06-02 from the memory files (high-confidence; these are recent).
Most post-M12 deferrals are LOW polish; the MEDIUMs worth tracking:

### Open MEDIUM
- `m14` — property-registry has no save-pipeline integration (substrate-only;
  also tracked in `BACKLOG.md` §1).
- `m16-5` — WebSocket `insecure_skip_verify` footgun; production-readiness
  (TLS termination, rate-limit, per-IP connection cap) deferred.
- `m17-2c` — door arg resolves single-token, not slurp (M17.2d adapter path).
- `m19-4h` — admin `set` **player** property (needs `connActor.Properties/
  SetProperty` + a save bag) and the `set tag` kind (no runtime tag mutator) —
  both substrate-blocked.
- `m22 #2` — atomic `Contents.MoveAll(from,to)` for the mob→corpse bulk move
  (the get-from/loot/decay paths are already single-winner-safe; this is tidy-up).
- `tabcomplete-p2` (tab-completion Phase 1/2, post-M22) — WS GMCP `Input.Complete`
  `req.Line` uncapped (~64KB ceiling vs telnet's 1KB) before `strings.Fields`/
  `CompleteLine` (`session/gmcp_complete.go` — truncate to ~512B); link-dead
  reattach doesn't nil the old conn's GMCP/completion handlers before the swap
  (`session/linkdead.go`, latent); swallowed `SendGmcp` error (`gmcp_complete.go`);
  `tabcomplete off` ignores `SetCharMode`'s return (`command/tabcomplete.go`). HIGH
  char-mode buffer-DoS was **RESOLVED in-review 2026-06-03** (cap at MaxLineBytes).

### Open LOW (compressed)
- `m13` — accepted: actorSink kind-switch (extract on 3rd kind), `Store.Load`
  names-cache lock (sync.Map if profiling).
- `m14` — `MaxChangeListener` 2-arg signature; Race `BaseStatMods` (feature, not debt).
- `m15` — 7 LOW accepted; future World-Depth slices in `docs/themes/world-depth-plan.md`.
- `m15-4b₂b` — 3 LOW room-render ambience (order-test gap, Ambience lock cost,
  RenderRoom 5-arg signature).
- `m16-4e/f/g/h` — GMCP package polish: slices.Equal reinvent, redundant Flags
  copy, Permanent+Remaining quirk, JSON-marshal silent return, actorSink kind
  switch; note `session.go` is ~3100 lines (pre-existing, split candidate).
- `m16-6a/b` — capability/ANSI-tier polish: cache 4×, HTML fg-only, plain
  ignores tier (4 + 3 LOW).
- `m17-1a/b/c` — scripting sandbox polish: symlink follow, no size cap,
  per-Compile LState, re-entrancy-if-publish-lands, edge coverage (13 LOW).
- `m17-2a/c` — arg-typing polish: 7 LOW + bulk return shape, door msgs unsmoked.
- `m19-4a` — announce attribution config (§8), audit refused gate attempts, announce
  text through ANSI markup (3 LOW).
- `m19-4b` — inspect: player properties not rendered, ambiguous target not listed
  as candidates (2 LOW).
- `m19-4d` — `recall` doesn't emit `player.moved` (questwatch/AI-reset skip recall
  arrivals); fix next `recall.go` touch.
- `m19-4e` — `purge` has no recursive container/carried-contents cleanup (orphan
  risk, matches death-path).
- `m19-4h` — pack-scoped props need a qualified `pack:name`.
- `m22` — loot/corpse RNG would need a lock if a death/spawn is ever signalled off
  the tick goroutine; `getFromRoom` is 70 lines (mostly comments); zero-weight loot
  entries pass decode silently.
- `phase0-tabcomplete` — bulk `all.<kw>` prefix-vs-`Matches` mismatch; preposition-as-
  partial yields spurious completions; `completeVerb` hand-rolls its RLock.
- `tabcomplete-p2` — `applyCompletion` slice invariant unguarded; lineedit echo tests
  use `time.Sleep`; tests poke `server.charMode` past the mutex; anonymous
  `GmcpActive()` interface; redundant `CharModeActive()` wrapper; `candidateLine`
  string-concat per item.

### Accepted (not debt), post-M12
- `m17-2d3` — §5 verb NON-FITS kept hand-parsed by design: `unequip` (no `equipped`
  arg type), `fill` (source scope), `buy`/`sell`/`value` (resolve in ShopService),
  and now `get`/`look` (container scope conditional on a sibling arg, M22.3b).
- `m16` (closed), `m17.1` (closed), `m13`/`m14`/`m15` themes LANDED.

---

## Accepted design / open questions (not really debt)

Cap-tier ladder hardcoded; class-swap path absent; alignment history
runtime-only; abilities resolve combat-only; regen gated off in combat; shop
prefix resolution; `safe` vs `safe-room` tag-string divergence (combat checks
`"safe-room"`, training `"safe"` — a room must declare both).

---

## Caveats / verify-before-acting

1. **M14 (Engine-Debt) + the M9.x mob-effect sweep resolved several M0→M12
   items.** Confirmed closed (struck through above): `m8-1 #2` vital re-clamp
   (M14.1), `m11-5` consumable effect-id (M14), `m10-9` quest_grant-on-room
   (M14), `m9-4`/`m9-5` mob effect-stat (2026-06-01). Re-verify any remaining
   M8/M9 item against current code before treating it as real work.
2. **Likely stale-open M8 items.** Several were written as "X is a nop *until*
   M8.4/M8.6/M9," and those milestones have since landed (e.g. `m8-2 #1` class
   LevelUp subscriber, `m8-3` StatCaps / CastCostModifier consumers, `m6-4`
   MobInstance race). The memory files weren't always updated to mark them
   resolved — re-verify before treating as real work.
3. **`m7-1 #2`** (MobInstance.stats mutex) is effectively mooted by cluster 1 —
   mobs now use a `*progression.StatBlock` with its own lock.
4. **Role system now exists (M19).** M0→M12 notes that say "no role system"
   (e.g. `m8-2` xp self-grant, `m8-5` admin-bypass role tag) are now buildable
   — the `HasRole` gate landed with M19.
5. The M0→M12 body reads from file headers + cross-references; a few may have
   been quietly resolved in a later sweep. The Post-M12 section (M13→M22) and
   post-M9 entries are high-confidence.
