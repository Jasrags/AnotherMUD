# AnotherMUD — Primer for External Design Work

**Purpose:** a self-contained orientation for an AI (or person) **designing a
feature spec for AnotherMUD without repository access.** Paste this whole
document as context before drafting a spec or system design. It exists so that
designs come back already shaped to this engine, instead of assuming a generic
MUD and needing rework.

If you only remember five things:

1. **AnotherMUD is a custom Go engine.** It is *not* ROM, CircleMUD, CoffeeMUD,
   DikuMUD, Evennia, or any off-the-shelf codebase. Do **not** recommend a
   codebase, compare candidates, or assume any named MUD's object model. The
   codebase is decided and built.
2. **The setting is a placeholder.** Specs are behavior-only and
   **setting-agnostic**. The world content (regions, cultures, lore) lives in
   *content packs*, not in specs. Do not bake a specific setting (Wheel of Time
   or otherwise) into a spec; provide the *mechanism* and let content fill it.
3. **A lot already exists.** Before proposing to "build" a system, check the
   *What exists* lists below — most substrate (items, mobs, combat, progression,
   effects, economy, quests, sessions, scripting, GMCP) is already built.
4. **Specs are behavior-only.** No numbers, no library names, no Go code in the
   spec body. Numbers go in a "Configuration surface" table. Follow the house
   spec format (below) exactly.
5. **Some things are deliberately greenfield.** A short list of systems do *not*
   exist and have design freedom (e.g. no furniture system, no faction, no
   gathering nodes). Knowing which is which prevents both reinventing and
   false-assuming.

---

## 1. What AnotherMUD is

A from-scratch MUD engine written in **Go (module `github.com/Jasrags/AnotherMUD`,
`go 1.26`)**. Players connect over **telnet or WebSocket**; the world is text,
rendered with optional ANSI color. It is **spec-driven**: every system has a
behavior specification under `docs/specs/` that is the source of truth, written
to be language-agnostic and implementation-free. The Go code fills the specs in
milestone by milestone.

It has a (now-stale-named) heritage: an earlier C#/.NET incarnation called
"Tapestry" is sometimes used as a *reference implementation* to port behavior
from. The engine namespace in content is `tapestry-core` — treat that string as
a placeholder, not a setting.

**Maturity:** the engine is well past prototype. Milestones M0–M17 are complete,
including five cross-cutting "themes" (social, modern-client/GMCP, world-depth,
content-authoring/scripting, engine-debt). M18 (small command/UI polish) is in
progress. In short: the core MUD loop, world, entities, combat, progression,
economy, quests, sessions, scripting, and modern-client support all work.

---

## 2. Architecture in one page

The runtime is built on a few load-bearing primitives:

- **Tick loop.** A game loop ticks at a fixed cadence (≈100 ms). Systems register
  tick handlers at multiples of the base tick (e.g. autosave, AI, spawn resets,
  effect expiry, weather, the in-game clock, GMCP flushes). Time-based behavior
  hangs off this loop, not off wall-clock timers.
- **Event bus.** A typed in-process bus carries **cancellable** events (a
  subscriber can veto: e.g. a pre-move, pre-recall, death-check) and
  **non-cancellable** events (facts that already happened). Decoupled reactions
  ride the bus rather than direct calls. New reactive behavior almost always
  means "publish/subscribe a bus event."
- **Entity store.** Live mobs and items are tracked in a store with lookup
  by id, by tag (a double-buffered index swapped at the tick boundary), and by
  type. Rooms reference entities via a placement index; containers via a
  contents index.
- **Sessions.** Each connection is an actor (`connActor`) managed by a session
  manager (online roster, flood control, idle sweep, link-dead + reconnect,
  takeover). The actor is the player's in-world presence and the seam most verbs
  act through.
- **Content packs.** Game content is data: YAML files in a pack
  (`content/core/`), discovered and loaded at boot into registries (rooms,
  areas, items, mobs, abilities, effects, quests, races, classes, tracks, slots,
  themes, help, weather zones, plus Lua scripts). Unqualified ids resolve to the
  current pack's namespace; `pack:id` crosses packs.
- **Persistence.** Player and account state save to disk with atomic
  tmp→bak→rename writes. Player saves are versioned with a migration table.
  In-world transient state (sessions, spawn tracking, weather, temporary exits)
  is deliberately *not* persisted.

**Binding conventions** (every system follows these):
- `ctx context.Context` is the first parameter on anything doing I/O, ticking, or
  cancellable work.
- Structured logging via `log/slog`, the logger carried on `ctx`.
- Errors wrap with context (`fmt.Errorf("doing X: %w", err)`) and use
  package-level sentinels.
- A `Clock` interface abstracts time; engine code never calls the wall clock
  directly. (There are *two* clocks: a real/wall `Clock` driving the tick loop,
  and a separate in-game hour/day **game clock** that content reasons about.)

**Package map** (`internal/…`, so you know what exists and where it lives):
`tick`, `eventbus`, `clock`, `gameclock`, `logging` (foundations) ·
`world`, `entities`, `item`, `mob`, `slot`, `keyword`, `spawn`, `ai`, `portal`,
`weather`, `property` (world + things) · `stats`, `srckey`, `progression`,
`combat`, `effect` (character mechanics) · `command`, `economy`, `quest`/
`queststore`/`questwatch` (action + interaction) · `account`, `player`, `login`,
`session`, `wizard` (player lifecycle) · `chat`, `notifications`, `emote`
(social) · `render`, `ansi`, `help` (presentation) · `conn`, `server`, `telnet`,
`gmcp`, `mssp` (networking) · `pack`, `script`, `scripting` (content + scripting)
· `persistence`.

---

## 3. What exists — already BUILT (do not re-propose)

Treat all of this as working substrate you can integrate with:

- **Networking / client:** telnet with full IAC option negotiation (TTYPE,
  NAWS, MSSP, GMCP); WebSocket transport; ANSI color at 16 / 256 / truecolor
  selected per detected client capability; a GMCP package layer (Char.Vitals,
  Room.Info, Char.Items, Char.Combat, Char.Effects, Char.Experience,
  Comm.Channel, Char.Login/Status).
- **World:** rooms, areas, exits; **doors + locks**; **temporary portals /
  keyword exits**; **weather** (per-zone, evolves on the in-game clock,
  terrain-gated); an **in-game clock** (hour/day + time-of-day periods); a room
  **`terrain`** property; a **room property bag** + a property registry; **room
  tags**.
- **Entities:** the entity store (track/untrack, by-id/tag/type, tag-index swap,
  in-place re-tag); item instances; mob instances; room↔entity placement;
  container↔item contents.
- **Items / inventory:** item templates + registry; instances with property
  bags; equipment **slots**; equip/unequip with stat modifiers; container ops;
  **stacking**; **keyword resolution** (`sword`, `2.ring`, `all.gem`).
- **Progression:** stats; races; classes; **tracks** (XP / levels, multi-track);
  **alignment**; **training**; **proficiency with use-based skill-up**;
  **abilities** (active + passive); **effects** (an effect manager + an
  EffectTemplate registry, applied to players *and* mobs); an action queue and
  ability-resolution pipeline. A "crafting" side-track is already anticipated.
- **Combat:** engage/disengage, a round/heartbeat, hit/miss/damage, flee, death
  (with player recovery), vitals, auto-attack, a passive-ability evaluator.
- **Mobs / AI:** mob templates; area-driven spawning with resets; AI behaviors
  (stationary/wander); disposition reactions; **mob loot** (drop list at spawn);
  mob proficiencies; mob StatBlock so effects modify mobs.
- **Economy / survival:** a **gold** currency property + auto-conversion;
  **shops** (buy/sell/value at vendor NPCs); a **sustenance pool** (one pool;
  `eat`/`drink`/`use` all feed it — there is no separate thirst); a **rest**
  state machine; a **consumable** pipeline (food/drink/potions → sustenance +
  an effect via `effect_id`).
- **Quests:** definitions, prerequisites, stages, objectives, rewards; an
  auto-tracking watcher; map markers; persistence; `quest_grant` on item and on
  room entry.
- **Commands:** a command registry + dispatcher; **typed arguments** (an
  `ArgDefinition` system with resolvers for items, inventory, players, rooms,
  doors, ordinals, bulk, with a `bypass_visibility` flag); builtins; a `prompt`
  verb.
- **Scripting:** a sandboxed **Lua** runtime (gopher-lua); pack script discovery;
  a bus bridge + minimal engine API (`subscribe`, `log`, `schedule`); hot
  reload.
- **Social:** a per-entity notification queue; **tells** (with an offline inbox
  that delivers on next login); **channels** (e.g. `ooc`); **emotes**.
- **Player lifecycle:** accounts (bcrypt); versioned player saves with
  migrations; a login state machine; an interactive **character-creation**
  wizard; sessions (manager, flood gate, idle timeout, link-dead/reconnect,
  takeover); **recall** (bind + return).
- **Presentation:** a theme/color renderer with semantic color tags; **prompts**
  (token-substituted status line); **panels**; a **help** service with topics.

---

## 4. What exists only as a SPEC (contract written, code not yet built)

These have a `docs/specs/*.md` behavior contract but **no Go implementation yet**.
If your feature depends on one, treat it as a known contract you can rely on,
but note it as a build dependency:

- **roles-and-permissions** — a flat `HasRole(name)` capability model (not a tier
  ladder); the basis for all privilege gating.
- **admin-verbs** — commands marked admin, dispatcher-gated on a role; baseline
  admin verb set; audit trail.
- **item-decorations** — **rarity tiers** (ordered, colored, decorated) and
  **essence** (a colored item glyph); rarity tiers are the project's standard
  **quality-tier** vocabulary.
- **tag-observers** — `entity.tag_added/removed` bus events for reactors.
- **who** — the connected-player roster verb.
- **crafting-and-cooking** — recipes, crafting skills, tiered stations, the
  quality roll, cooking↔sustenance. (Already designed; MVP build pending.)

---

## 5. What is GREENFIELD — does NOT exist and has design freedom

Do **not** assume these exist; if a design needs one, call it out as new
substrate / a dependency. (Several were checked against the Tapestry reference
and have *no* implementation there either — i.e. genuine greenfield, not a port.)

- **No general furniture system.** (Crafting's stations were modeled by *reusing*
  the temporary-entity + room-tag substrates specifically to avoid building one.)
- **No gathering / resource nodes** (foraging/harvesting). Ingredient-type
  sourcing currently means mob loot or authored placement.
- **No faction / standing / reputation** (alignment is the only standing-like
  axis today).
- **No visibility rules.** A `bypass_visibility` *seam* exists in arg resolution,
  but the actual hidden/invisible/sneak *rules* are unbuilt — `CanSee` is
  effectively "always true."
- **No biomes** as a system (only the `terrain` room property + weather zones).
- **No banking, no mail-with-attachments, no auction house, no direct
  player-to-player trade.** Player exchange today is one-way `give` + NPC shops;
  there is no gold-at-risk mechanic (death costs no gold).
- **No role/permission enforcement yet** (the help "tier" is a no-op stub until
  roles-and-permissions is built).

---

## 6. How to write a spec that fits (house conventions — follow exactly)

Every AnotherMUD spec has the same shape. Match it so the result drops straight
into `docs/specs/`:

- **Title line + a Status/Scope/Audience header paragraph.**
- **Behavior-only.** Describe *what* the system does and *why*, never *how*. No
  Go code, no library names, no specific algorithms tied to an implementation.
- **No magic numbers in the body.** Every tunable value (caps, durations,
  weights, tiers, rates) goes in a **"Configuration surface"** table near the
  end, referenced from the prose as a named setting. The only numbers allowed
  inline are interoperability constants (e.g. telnet byte codes), which this kind
  of feature rarely has.
- **Numbered sections, each organized around an operation,** each ending with a
  short **Acceptance criteria** checklist phrased so it reads like tests
  (`- [ ] …`).
- **An "Overview" §1** (concepts + goals), usually with a "What this is *not*"
  subsection that rules out scope creep.
- **An "Open questions" section** that *preserves* design tensions rather than
  hiding them. For each, give a recommendation and a chosen default so the spec
  isn't blocked, but flag it for sign-off. Record *rejected* alternatives too
  (why a tier ladder was rejected, etc.).
- **A "Cross-references" footer** pointing at the other specs it touches (by
  name + section).
- **Setting-agnostic.** Refer to regions/cultures/items generically; the concrete
  world is content. If geography matters (e.g. "regional recipes"), spec the
  *mechanism* ("learnable only from source X") and note that X is content.

A good recent example to imitate in structure and tone is `recall.md` (small,
self-contained) or `roles-and-permissions.md` (a system with state + operations
+ config + open questions).

---

## 7. Integration cheat-sheet — "if you're designing X, it probably uses…"

| If your feature involves… | Reuse this existing system | Spec to cite |
|---|---|---|
| Quality / rarity / tiers of an item | rarity tiers | `item-decorations` |
| A learnable skill that improves with use | proficiency on a progression track (use-based gain) | `progression` |
| A timed buff / debuff / status | the EffectTemplate + effect manager | `abilities-and-effects` |
| Hunger / food / drink | the sustenance pool + consumable pipeline | `economy-survival` |
| Buying / selling / vendor / gold sink | currency + shops | `economy-survival` |
| Randomized outcome | the engine `Roller` (tick-context RNG) | `combat` / `progression` |
| A new content type (recipes, etc.) | a pack-loaded registry + YAML | `scripting-and-packs` |
| A privileged/admin action | role gating | `roles-and-permissions` / `admin-verbs` |
| A new verb | the command registry + typed args | `commands-and-dispatch` |
| Reacting to something happening | publish/subscribe a bus event | the spec that owns the event |
| Time-of-day / seasonal behavior | the in-game game clock | `time-and-clock` |
| Terrain / weather gating | room `terrain` property + weather | `world-rooms-movement` |
| Per-character saved state | the player save (versioned + migrated) | `persistence` |
| Offline delivery to a player | the notification queue (delivers on login) | `notifications` |
| A modern-client HUD/panel | a GMCP package | `networking-protocols` (transport) |

---

## 8. Anti-patterns (things that signal a design hasn't been shaped for us)

- Recommending or comparing MUD codebases (ROM/Circle/CoffeeMUD/custom). The
  codebase is decided.
- Baking a specific setting/lore into the spec body instead of leaving it to
  content.
- Proposing to build something in §3 that already exists (a stat system, an
  effect system, a shop system, a quest system…).
- Putting concrete numbers, formulas with fixed constants, or library choices in
  the spec prose instead of the configuration-surface table.
- Assuming a greenfield system from §5 exists (furniture, gathering, faction,
  visibility rules, banking/mail/auction).
- Designing a synchronous polling loop where the engine would use a tick handler
  or a bus event.
- Persisting things the engine deliberately keeps transient (sessions, weather,
  spawn tracking).

---

## 9. Where to send the result

Produce a single behavior spec in the house format above (Overview → numbered
operation sections with acceptance criteria → Configuration surface → Open
questions → Cross-references). If a phased build plan is also wanted, keep it
separate from the spec (the spec is the timeless contract; the plan is the
sequence). Flag any greenfield dependency (from §5) explicitly, and resolve open
decisions with a recommended default rather than leaving them blocking.

*This primer reflects the engine as of M18 / all five themes shipped. The
in-repo `CLAUDE.md` "Repository status" line is stale (it predates most of this);
trust this document for current state.*
