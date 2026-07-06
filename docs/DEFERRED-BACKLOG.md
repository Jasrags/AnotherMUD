# Deferred-Items Backlog

A consolidated, point-in-time snapshot of the **open** deferred items across
all milestones (M0→M27 + light/dark), distilled from the per-milestone
`m<N>-deferred-fixes.md` memory files. The memory files remain the source of
truth (full context, file:line, fix-when triggers); this is the scannable
index.

**Generated:** 2026-05-28 (M0→M12 body); **post-M12 section added 2026-06-02**
(M13→M22, from the memory index); **tab-completion deferrals added 2026-06-03**;
**M23→M27 + light/dark section added 2026-06-10**; **WoT EPIC S1 equipment-debt
section added 2026-06-17**; **WoT EPIC feats-catalog section added 2026-06-18**;
**river-passage greenfield deferral added 2026-06-19** (from the WoT geography
arc — M5 Baerlon road / M6 Whitebridge corridor).
Regenerate by re-scanning the memory deferral files.

> **Not yet folded in (2026-06-20):** the **M28 visibility**, **movement-cost**,
> **mounts (core-v1)**, **M29 trade/auction**, and **M30 faction + reputation**
> deferrals are not in the body below. Their open items live in the per-feature
> memory files — `visibility-deferred-fixes`, `movement-cost-deferred-fixes`,
> `mounts-build-log`, `auction-house-deferred-fixes`, `faction-s8-build-log`,
> `reputation-build-log` — which are the source of truth until the next regen.

> **Not yet folded in (read the memory files directly):** the M28 **visibility**
> arc (`visibility-deferred-fixes` — HIGH secret-door op-gating + S6b refinements),
> the M29 **player-trade trio** (`auction-house-deferred-fixes` — cap read-then-add
> race, StatusRefunded, container listing), **mounts** core-v1
> (`mounts-build-log` — impassable-terrain vocab validation), and the WoT **S9
> class** bonus-feat gap (`feats-deferred-fixes` — MEDIUM, blocks martial classes).
> These shipped after this snapshot; the per-topic memory files are authoritative.

> **Not yet folded in (2026-07-06):** work shipped since the 2026-06-20 body — the
> **follow → grouping → hireable-mobs** arc (`follow-arc`, `grouping-arc`,
> `hireable-mobs-build-log` — grouping §9 XP-split/quest-credit/need-greed, hireable
> §11 formations/patrol-routes), the **action-economy** busy-state arc
> (`action-economy-deferred-fixes` — MEDIUM raw-string replay wrong-same-keyword),
> **subdual-damage** (`subdual-damage.md` §8 deferred — natural armor + separate-pool/
> coup-de-grace), and the **Shadowrun MVP** slices (`sr-m2-deferred-fixes` — MEDIUM-A
> non-atomic named-pool liveness; `sr-m3c-deferred-fixes` — stun→Physical overflow,
> `armor` channel input, body-derived Physical hp_max). The per-topic memory files
> are authoritative until the next regen.

Excludes everything marked RESOLVED/FIXED/CLOSED. Note: several M0→M12 items
below were later resolved by M14 (Engine-Debt) and the M9.x mob-effect sweep —
see the per-item RESOLVED tags and the caveats at the bottom.

---

## Open HIGH / CRITICAL

**No active bugs.** One HIGH item is open but **latent / gated**; the other was just resolved:

- ~~`area-legibility` — check-then-act race in `areaTransitionBanner` across 4 separate
  `connActor.mu` acquisitions~~ **RESOLVED 2026-06-10** — collapsed into one atomic
  `(*connActor).AreaTransition()` under a single lock; interface shrank to one method;
  `-race` concurrency test added; go-reviewer APPROVE. (`area-legibility-deferred-fixes`.)
- `room-coordinates-gmcp-wireshape` — GMCP `Room.Info` flat `x/y/z` is a **deliberate
  placeholder**, not validated against a live Mudlet mapper (`internal/gmcp/gmcp.go`,
  `session/gmcp_room.go`). **Fix-by:** before announcing Mudlet graphical-mapper support
  — pin the exact schema against a real client (human-in-the-loop). Not a code bug.

The last *active* HIGH (`ItemInstance.Properties()` unguarded map, `m11-5`)
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
- ~~`m7-3 #1` — `removeFromListLocked` aliases the backing slice~~ — **NOT A LIVE BUG (verified 2026-07-06).** The in-place `append(list[:i], list[i+1:]...)` shift is safe because no un-copied `m.lists` slice ever escapes `m.mu` — every accessor snapshots (`OpponentsOf`/`AllCombatants` copy, `PrimaryTargetOf` returns a value, `DisengageAll` copies before its mutation loop). A latent footgun only, now documented as an INVARIANT comment on `removeFromListLocked`/`PromoteTarget` (a future accessor returning the raw slice would break it → copy-on-out or fresh-slice build). No code behavior change.
- `m7-5 #2` — `DeathCheck` cancellation contract is doc-only
- `m7-5 #4` — concurrent killing-blow unit test missing
- ~~`m7-5 #5` — `VitalsState` YAML decode is unbounded~~ — **MISCHARACTERIZED (verified 2026-07-06).** `VitalsState` is two scalar ints (`HP`, `MaxHP`) — no array/collection to bound. The load path `restorePlayerVitals` already clamps out-of-bounds values (`HP > MaxHP`, `MaxHP < 1`). Nothing to fix.
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

## M23→M27 + light/dark — open items

Indexed 2026-06-10 from the post-M22 deferral memories. **No active bugs;** the
two latent HIGHs are in the "Open HIGH" section above. **Note `m22 #3` (unbounded
corpse growth) is now RESOLVED** — M22.5 shipped `corpse.Service.DecaySweep`.

### Open MEDIUM
- `m24 #1` — `Save.VisitedRooms` grows unbounded; PD-10 prune-on-load unbuilt
  (`internal/session/visited.go`). Correctness-harmless (renderers intersect with
  live rooms); pure save/IO growth. **Fix-by:** world past ~500 rooms — filter
  `VisitedRooms` against the live world at login.
- `room-color #1` — room-line hostility coloring uses **nil viewer tags**
  (`builtins.go hostileMarker`), so only *statically* hostile mobs redden;
  rule/alignment-gated hostility won't color until player tags thread into
  `DispositionHook`. **Fix-by:** when player tags get threaded (same change
  unblocks the hook's own §5.3 tag-rule matching).
- `m26` deferred (Engine Debt III feeders, triggers unfired): §6.2 passive
  **scaling-bonus consumer** (no content sets `max_bonus` yet); **property-registry
  save-pipeline** (`property.Wrap/Unwrap` have no production caller — same item as
  `m14` MEDIUM; fix when a save first needs a content-declared property).
- **§3.4 tag-indexed movement reads — deferred 2026-06-10 with a sharpened trigger.**
  Investigated: the `mob` baseline tag already exists (`entities.TagMob`), but a clean
  O(mobs-in-room) read has no proportionate implementation — `Store` holds the tag index,
  `Placement` holds room membership, and they only meet in the `Evaluator`. A true win
  needs a **per-room tag index** (`Placement.byRoomTag` synced on Place/Remove +
  cross-synced with the Store tag index on `Retag`) — new always-maintained cross-object
  state, overkill for ~4 rooms / a few mobs. `sweepRoom` stays as-is (correct + idiomatic).
  **Fix when** room occupancy grows enough that the per-entry scan shows up in a profile.

### Open LOW (compressed)
- `m25` — equipment-slots: `no_remove` tag hardcoded (→ §8 config); multi-cap
  companions + `body`/`legs` robe footprint unshipped (§9 edges); score sheet shows
  a 2h spanner in both slots (intentional).
- `area-legibility` — the C1 way-back note uses a `→` glyph through byte-based
  width math (cosmetic; see `render-panel-width-multibyte`).
- `render-panel-width-multibyte` — `render.VisibleLength` is byte-based, so
  multi-byte glyphs drift `render.Panel` borders. Worked around in `score`; real
  fix = rune-aware `VisibleLength`/`truncateVisible` when a panel needs non-ASCII.
- `light-and-darkness` — area/zone override tiers unwired (pending biomes hook);
  room-loose light sources don't burn fuel; light-effect floor unwired (no content
  effect yet); reduced-light prose strings hardcoded (§11).
- `crafting` — well-fed re-eat refresh; recipe-scroll name keyword collision
  (`buy dagger` ambiguous → use `buy scroll`/`buy rusty`); Phase 7 regional recipes
  remain geography-gated.
- `m24` — `world.LocalWindow` vertical-expand-then-filter + `queue[1:]` backing-array
  + `Window.Contains` O(n); `wrapMarkupLine`/`mapLegend` micro-allocs.

---

## WoT EPIC S1 — equipment debt effort

Consolidated 2026-06-17 from the equipment review across the WoT S1 build logs
(`armor-depth`, `size-wielding`, `masterwork`, `m25` slots) + the `equipment.md`
load-readiness pass (commit `ce4c0e1`). Source-of-truth memory file:
`equipment-debt-roundup`. Three tracks; only **Track A** is actionable now.

### Track A — actionable now (small, no design)
- **A1 (MEDIUM) — armor check-penalty over-counts in mixed proficiency. ✅ SHIPPED 2026-06-17.**
  `attackerArmorPenalty` applied the SUM of every worn piece's `armor_check`
  whenever non-proficient in *any* piece. Fixed via
  `connActor.NonProficientArmorCheckPenalty` (per-piece, grade-reduced, live);
  the dead `ArmorCheckPenaltyTotal` was removed. (`armor-depth-build-log`.)
- **A2 (MEDIUM) — sized weapon + static `companion_slots` silently discarded. ✅ SHIPPED 2026-06-17.**
  Non-fatal boot warning (`sizedCompanionConflicts` → `slog.Warn` in `Load`) when
  an item declares both. No shipping item trips it (verified). (`size-wielding-build-log`.)
- **A3 (LOW) — two-weapon penalty consts not env-wired. DEFERRED (reclassified — not cheap).**
  `DefaultTwoWeapon{Main,OffHand}Penalty` (`internal/combat/stats.go`) are consumed
  at STAT-BUILD time (`connActor.Stats` + `MobInstance.Stats`), which take no config —
  unlike the round-loop `SecondaryOffHandPenalty`. Clean env-wiring needs a combat
  config struct threaded into both hot `Stats()` methods (the codebase has no
  boot-mutable-package-var precedent; env is read only at the composition root).
  Low value (the two-weapon feats already tune the effective penalty). **Fix-by:**
  when a `combat` tuning-config seam is introduced for another knob, fold these in.

### Track B — keep deferred (YAGNI; trigger-gated)
`no_remove` tag → §8 config; multi-cap companion slots untested; **spanning robe
body/legs — PARTIAL: a `body` slot landed 2026-06-17 (`content/core/slots/body.yaml`)
and body armor uses it; the `legs` slot + a body+legs spanning robe remain unshipped
(no content needs them)**; score sheet shows 2h spanner in both slots (intentional);
`cappedDexAC` equip-snapshot cap staleness; no graded ARMOR/TOOL content; armor §7
hasty-don escape (combat gate shipped); mobs skip `RangedDamageBonus`/`strRating`
cap (pre-existing G concern).

### Track C — greenfield, spec-first (in `BACKLOG.md` §2)
- **Mounts & barding (Large) — ✅ SPECCED 2026-06-17** → `docs/specs/mounts.md`
  (behavior contract; build pending). v1: ridden mount becomes the metered mover
  (reuses movement-cost), barding reuses armor-depth, saddlebags are a container,
  stabling/feed gold sinks, conservative combat boundary; charge/Ride-contest/
  multi-seat/flight deferred to Open Questions.
- **Grenadelike weapons + room hazards — ✅ SPECCED JOINTLY 2026-06-17** →
  `docs/specs/area-effects.md` (behavior contract; build pending). One spec: the
  shared multi-target *area-effect primitive* (payload to everyone in a region),
  consumed by thrown grenades (direct+splash+ignition) and placed hazards
  (on-enter/on-tick, durable world store). The igniting oil flask becomes a
  hazard (bridge). Sub-room footprints / full hidden-trap system / rocket-stack
  demolition / cross-room lobbing / ally friendly-fire deferred to Open Questions.

**Track C is now fully specced (C1 mounts + C2/C3 area-effects); both await build.**

---

## WoT EPIC — feats catalog buildout

The `docs/wot/feats.md` catalog is being filled bucket-by-bucket (source-of-truth
memory file: `feats-catalog-build-log`). Shipped 2026-06-18: Bucket A (Alertness /
Sharp-Eyed / Stealthy), Bucket C (Power Attack stance, save v27; Cleave + Great
Cleave on-kill seam), Bucket B (Weapon Specialization / Dodge). Open deferred
items from those phases:

- **(Bucket D — bigger) Whirlwind Attack — needs a multi-target round + missing
  prereqs.** "Hit every foe in reach" violates the round loop's single-primary-
  target assumption (`runAutoAttack` / combat §4 / Decision 0 "one swing, one
  target, stop on kill") — it needs the loop restructured into a multi-target
  sweep (~150 lines) plus a spec decision (does a mid-sweep kill halt the
  remaining targets?). Separately, 3 of its 4 d20 prereqs don't exist (Combat
  Expertise — a defensive toggle stance; Mobility — AoO; Spring Attack — move-and-
  attack, Decision-0 action-economy), only Dodge shipped. **Fix-by:** its own
  milestone once a multi-target round and the prereq feats land — not a small add.

- **(MEDIUM) Power Attack trade is main-hand only.** The stance applies its to-hit
  penalty to `s.HitMod` (main hand) but the off-hand profile (`s.OffHand`) is built
  *before* the Power Attack block in `connActor.Stats()` (`internal/session/session.go`),
  so a dual-wielder in Power Attack loses main-hand accuracy while the off-hand swing
  takes neither the penalty nor the damage. d20 applies Power Attack to all melee
  attacks that round. Surprising in play (flagged by go-review). **Fix-by:** when the
  off-hand×stance interaction is next touched, or before a dual-wield + Power Attack
  build is balanced — fold the same trade into the `OffHandProfile` (penalty to
  `HitMod`, the damage half — likely ½× like the off-hand Strength rule).
- **(LOW) `DefaultPowerAttackTrade` not env-wired** (`internal/combat/stats.go`).
  Same class as the A3 two-weapon-penalty knob above (consumed at stat-build time,
  no config seam). **Fix-by:** fold in when a `combat` tuning-config seam lands.
- **(LOW) `FeatSkillBonus` recomputes all feat bonuses per call** — used by
  `PerceptionBonus`/`HideScore`/`SneakDifficulty` (`internal/session/`). Cold paths
  today (player commands, not the tick). **Fix-by:** if any becomes tick-driven, cache
  `skillPerception`/`skillStealth` on `featWeaponBonuses` like the weapon-category
  bonuses and read lock-free.
- **(LOW, content) canonical prereqs relaxed for demo reachability** — Power Attack
  (Str 13), Dodge (Dex 13), Weapon Specialization (Armsman 4th + same-weapon Weapon
  Focus) omit/relax their d20 prereqs so a starting/demo character can take them; the
  prereq engine also can't match Weapon Focus at weapon-param granularity. **Fix-by:**
  when starting-stat budgets support the gates, or a param-aware feat prereq lands.
- **Bucket-shrink note (not debt, triage):** Weapon Finesse and Quickness/Run moved to
  Bucket D — no consumer exists (to-hit is the `hit_mod` stat with no Str term to
  substitute; no speed/movement-rate consumer). They unblock only after the attack
  formula incorporates an attribute / a movement-speed subsystem lands.

Still open from the prior S9 class work: the **class bonus-feat gap** (`feats-deferred-fixes`
— no `bonus_feat_levels` mechanism; the multiclass feat-credit over-earning fix shares
that hook — do both together).

---

## Greenfield — river passage & navigation (deferred)

Surfaced 2026-06-19 from the WoT geography arc (M5 Baerlon road / M6 the
Whitebridge corridor over the Arinelle). **Today rivers are content, not
mechanics:** there is no water terrain or biome, no swimming, no
current/drowning, no boats-as-vehicles, and no river you can travel *on*. The
Arinelle, the Taren, and the White River are described scenery rendered in room
prose; a player meets them three ways, all plain land rooms + ordinary exits:

- **a described boundary** — a bank room sits *beside* the water (e.g.
  `the-west-bank`, `the-whitebridge-docks`, `the-white-river-bank`); the river is
  prose, not enterable;
- **a fixed crossing** — a bridge or "ferry" is a normal room exit. The White
  Bridge is one room with `east`/`west` exits; the Taren ferry is a free `north`
  move (the `taren-ferryman` / Whitebridge `riverman` are flavor mobs — their
  prose mentions a price, but there is **no toll/fare mechanic**);
- **following the river** — a chain of bank rooms running *parallel* to the
  water, never on it.

`world.Exit` can only model `Target` / `Door` / `Hidden` — there is no water,
current, fare, or vehicle concept an exit can carry. This is fine today and
nothing needs more.

**Eventually we want real river passage + navigation** (a barge down the
Arinelle, river fast-travel, unbridged fording). Greenfield, **spec-first**.
Rough shape from the 2026-06-19 design discussion:

- a **water terrain/biome** (`terrain: water`) for river/lake rooms;
- a **river-travel mode** — most naturally modeled like **mounts**
  (`docs/specs/mounts.md`): a boat becomes a *metered mover* that re-points the
  `movement-cost` points/gate from the walker to the vessel; or a simpler **paid
  barge-route** fast-travel link between two dock rooms;
- optionally **swim checks** (a Reflex/Strength `saves.md` contest to ford
  unbridged water, drowning on failure) if unbridged crossing should carry risk.

**Seam already in place:** the Whitebridge **docks + riverman** (M6) are the
intended hang-point for an Arinelle route south toward Illian. **Fix-by:** when a
navigable-river / water-travel feature is wanted — first deliverable is a new
spec slice; promote to `BACKLOG.md` §2 (greenfield systems, alongside mail /
banking) when scheduled.

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
