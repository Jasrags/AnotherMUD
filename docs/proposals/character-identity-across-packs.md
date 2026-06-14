# Proposal: Character identity across content packs — world-locked, not portable

**Status:** design-first; **open questions UNRESOLVED** (§7 — to be decided in a later
pass), no code yet
· **Type:** player-lifecycle + persistence design (introduces a character↔world
binding; one additive save field)
**Builds on:** the multi-pack reality already in the loader — namespaced `pack:id`
content references (`internal/pack`, `qualifyID`), the dependency-closure boot
(`ANOTHERMUD_PACKS` → `core` + leaf), and the versioned/migrated player save
(`internal/player/player.go`, currently **v22**).
**Motivated by:** the planned multi-ruleset arc — running a Wheel of Time pack, the
starter/default pack, and a Shadowrun pack, where a player may create a character
under one ruleset and later connect to a server running another. See the
`multi-ruleset-engine-state` memory and `docs/themes/wot-mechanics-epic.md`.
**Constraints honored:** additive save migration only (append-only chain, atomic
writes unchanged); keeps the existing within-world fail-soft restore; does **not**
require namespacing the global registries yet (that is deferred, §6/§7-Q6).

---

## 1. TL;DR

**Lock each character to a *world* (a leaf ruleset pack + its dependency closure),
and do not make characters portable across worlds.**

Today's engine *looks* portable — on login it silently drops unresolvable
references (`applyRace` → raceless, `applyClass` → classless, recipe cull, missing
room → start room) and rewrites the save in that degraded state. That fail-soft
behavior was designed for **content edits inside one world** (retire a recipe,
delete a room). Reused as a **cross-ruleset** policy it becomes silent character
destruction: a WoT channeler logging into a Shadowrun server returns raceless,
classless, with orphaned weaves and vanished gear — and the good save is
overwritten with the broken one.

The fix is small: **one additive save field (`WorldID`, save v23) plus a login
gate + roster filter.** No new persistence machinery; the within-world fail-soft
net stays exactly as-is, just scoped to its real job.

## 2. What's pack-agnostic vs. pack-specific

From the persisted `player.Save` struct (`internal/player/player.go`, v22):

| Pack-agnostic (would travel anywhere) | Pack-specific (meaningless without its content) |
|---|---|
| `Name`, `AccountID`, `ID`, `Gender`, `Roles` | `Race`, `Class[]`, `Background` — **global** registry ids |
| `StatsBase`, `Stats`, `Vitals`, `Pools` (raw numbers) | `Progression` (keyed by **track** id) |
| `Gold`, `Sustenance`, `Alignment`, `TrainsAvailable`, `FeatCredits` | `Abilities` (keyed by **namespaced** ability id) |
| `WimpyThreshold`, `Autoloot`, `MinimapEnabled/Size`, `PromptTemplate`, `ShowRoomData` | `KnownRecipes[]`, `KnownFeats[]` (namespaced) |
| | `Location`, `Recall` (namespaced room ids) |
| | `Inventory[]`, `Equipment{}` (namespaced item template ids) |
| | `VisitedRooms[]`, `SeenAreas[]` (namespaced room/area ids) |

The split is clean: **identity + raw mechanical state is portable; everything that
names content is not.** Account credentials live entirely separately
(`internal/account` — email→id index + bcrypt) and never move.

The subtlety that drives the design: **`Race`/`Class`/`Background`/`Track` are
stored un-namespaced and resolve against a *global* registry with last-pack-wins
precedence.** Rooms/items/abilities/recipes/feats are pack-qualified
(`wot:weave-firebolt`); races and classes are bare (`channeler`). That asymmetry is
the load-bearing constraint (see §6).

## 3. Locked vs. portable — and why locked

| | **Portable (today's de-facto behavior)** | **World-locked (recommended)** |
|---|---|---|
| Cross-ruleset login | "Works" — silently strips unresolvable race/class/abilities/items, rewrites the save degraded | Blocked at login; character only enters a server running its world |
| Failure mode | Invisible data loss; degraded save overwrites the good one | Visible, safe: "this character isn't available on this server" |
| Within-world content edits | Already handled by fail-soft restore | **Unchanged** — fail-soft stays, scoped to its real job |
| Engine work needed | None — but the behavior is a trap | One additive save field + a login filter |
| Player mental model | "Why did my Aes Sedai become a commoner?" | "My WoT characters are WoT characters" |

Portability across *full rulesets* isn't desirable even if free — a character's
stats, class, and gear only cohere inside the ruleset that defined them
(`Str 16 + channeler + wot:angreal` is nonsense in Shadowrun). The only honest
cross-ruleset move is **re-roll**, not **carry over**.

The binding unit is a **world**, not a single pack. A WoT character legitimately
spans `core` (shared baseline) + `wot` (the leaf). Stamp the character with the
**leaf world id** (`wot`); `core` is a **library** pack that is never a world on
its own.

## 4. Cross-world references (the portable branch, for completeness)

Recommendation is *against* cross-world portability, so this reduces to: **don't.**

Within a single world, the existing fail-soft restore is exactly right and is
**kept** — it is how you safely retire a recipe or delete a room without bricking
saves. The design only stops *relying on it as a cross-ruleset bridge*.

If a true cross-world transfer is ever wanted (a "convert my WoT character to
Shadowrun" ritual), it must be an **explicit, lossy, opt-in migration** that
re-rolls content-bound fields against the target world and preserves only the
pack-agnostic column from §2 — never an implicit consequence of logging in. That
is a separate feature, out of scope here.

## 5. Account → character, and the save format

**Account shape is unchanged** — it already fits:

- **One account → many characters** (`account.Characters []string`, by name).
- **Each character stamped with its `WorldID`.** Multiple characters per world is
  fine; one account can hold a WoT roster *and* a Shadowrun roster at once.
- **Roster filtered by the server's active worlds.** Logging into a `wot` server
  offers only WoT characters; other-world characters are listed-but-greyed ("not
  available on this world") — **not** deleted, **not** loginable.

**Save format change — additive, bump to v23:**

```
WorldID string  // v23 — the world (leaf ruleset pack) this character belongs to
```

- The append-only migration chain backfills existing saves: **infer `WorldID` from
  the `Location` namespace** (`starter-world:town-square` → `starter-world`),
  falling back to `starter-world` when `Location` is empty/unparseable. Pure,
  deterministic v22→v23 migration — same shape the chain already uses, no new
  persistence machinery, atomic writes unchanged.
- Creation stamps `WorldID` from the active leaf world at wizard time.

That is the entire storage cost. The login gate and roster filter are runtime
logic, not save-format changes.

## 6. Precedents / constraints in the existing code

1. **Namespacing already exists and works** (`pack:id`, `qualifyID`,
   dependency-closure load order in `internal/pack/loader.go`). The character save
   is already mostly world-aware by construction; `WorldID` makes implicit
   explicit.
2. **Fail-soft restore is load-bearing** (`resolveStartRoom` → start room,
   `applyRace` → raceless, recipe cull, missing item → not spawned). Keep it — it
   is the within-world safety net. This design *narrows its scope*, it does not
   remove it.
3. **The global-registry asymmetry is the real blocker.**
   `Race`/`Class`/`Track`/`Background` are un-namespaced, last-pack-wins. Fine when
   a server hosts **one world**; unsafe the moment two **full rulesets** share one
   process (e.g. `channeler` collides in a flat namespace). World-locking lets us
   ship single-world servers safely **now** and defer the global-id namespacing
   until co-hosting is actually needed (§7-Q6). Already flagged in the
   `multi-ruleset-engine-state` memory.

## 7. Open questions (UNRESOLVED — recommended defaults noted)

Deferred to a later decision pass. Defaults are recommendations, not locked.

1. **Granularity of the world stamp** — single leaf-pack id vs. full active-pack
   set vs. a declared `ruleset:`/`world:` manifest field.
   → *Default:* **leaf-pack id**, plus a manifest flag marking a pack `world` vs
   `library` (so `core` is never a valid stamp). Full-pack-set is too brittle; a
   separate ruleset field is more indirection than needed today.

2. **One world per process, or co-host multiple worlds?**
   → *Default:* build the roster/login gate to *support* multiple worlds, but ship
   and run **one world per process** for now. Co-hosting two full rulesets is
   blocked on namespacing the global registries — record as prerequisite, don't
   build yet.

3. **Character whose world isn't active on this server?**
   → *Default:* shown **greyed in the roster, login refused** with a clear message.
   Never auto-deleted, never silently degraded.

4. **Backfill `WorldID` for existing v22 saves?**
   → *Default:* derive from the `Location` room-id namespace; fall back to
   `starter-world`. Deterministic, no operator input.

5. **Can `core` (or any shared baseline) ever be a character's world?**
   → *Default:* **no.** Baseline/library packs are dependencies; worlds are the
   leaves that depend on them. Enforced by the manifest `world|library` flag.

6. **Namespace the global registries (race/class/track/background) as part of
   this?**
   → *Default:* **no — defer.** It is the prerequisite for co-hosting two rulesets
   in one process; single-world-per-process doesn't need it. Capture as the next
   domino, ship `WorldID` locking first.

---

## Appendix — key code references (as of save v22)

- Player save struct + migration chain: `internal/player/player.go` (`Save`,
  `CurrentVersion`, `playerMigrations`)
- Account model: `internal/account/account.go` (`Characters []string`, email index)
- Returning-character placement / fallback: `internal/session/session.go`
  (`resolveStartRoom` → `cfg.StartID`)
- Fail-soft race/class restore: `internal/session/session.go` (`applyRace`,
  `applyClass`)
- Recipe cull on missing id: `internal/recipe/known.go` (`Restore`)
- Namespacing / id qualification: `internal/pack/loader.go` (`qualifyID`),
  `internal/pack/manifest.go` (`DeriveNamespace`)
- Boot pack selection: `cmd/anothermud/main.go` (`ANOTHERMUD_PACKS`,
  `ANOTHERMUD_START_ROOM`)
- Creation choices from active registries: `internal/session/creation_flow.go`
  (`NewCreationFlow`, `raceOptions`/`classOptions`/`backgroundOptions`)
