# Engine vocabulary — the content ↔ engine contract

This is the **ABI between content packs and the engine**: the string
identifiers a pack writes in YAML (`tags:`, `properties:`, and ids) that the
engine code branches on. It is derived from code, not a behavior spec — when
this doc and the cited code disagree, **the code wins** (file:line pointers are
given so you can check).

Why it exists: a world pack (the demo `starter-world`, or `wot`) authors rooms,
items, mobs, and recipes that lean on these strings. Knowing the vocabulary up
front avoids silent no-ops (a misspelled tag the engine never reads) and load
errors (an unregistered room property). See `docs/PRIMER.md` for the
setting-agnostic stance and `docs/themes/wot-world-plan.md` (M0.4) for context.

> **Scope note:** this catalogs the *most-used, code-confirmed* vocabulary. It
> is not exhaustive — new reserved keys land with new features. The
> authoritative sources are `internal/pack/properties.go` (the validated room
> property registry) and the per-feature `const` blocks cited below.

---

## 1. Ids, namespacing, and collisions

Two registry conventions decide what happens when two packs define the same id.
This matters when a world pack reuses or overrides engine-baseline content.

### Namespaced (`pack:id`) — collision is a hard load error

Concrete content. A bare id in YAML is qualified to the **defining pack's**
namespace at load (`town-square` in `starter-world` → `starter-world:town-square`);
cross-pack references use the full `pack:id` form. A true id collision across
packs fails the boot (`ErrDuplicateID`).

| Category | Registry |
|----------|----------|
| Rooms, Areas | `internal/world` (`TryAddRoom`/`TryAddArea`) |
| Items | `internal/item` (`TryAdd`) |
| Mobs | `internal/mob` (`TryAdd`) |
| Recipes | `internal/recipe` (`TryAdd`) |
| Node templates, Forage tables, Node spawn tables | `internal/gathering` |
| Weather zones | `internal/weather` |
| Biomes | `internal/biome` (pack biomes register under their bare terrain id; engine baselines `outdoors`/`indoors`/`underground` cannot be shadowed) |

### Bare-global (no namespace) — collision resolves by **priority** or errors

Mechanical/baseline content keyed by a bare id shared across all packs. A world
pack can **reuse** these (just reference the bare id) or **override** them.

| Category | On collision |
|----------|--------------|
| Abilities (`internal/progression/ability.go`) | **priority-override** — higher `priority` replaces, equal/lower is a silent no-op |
| Races, Classes (`internal/progression`) | priority-override |
| Quests (`internal/quest`), Loot tables (`internal/loot`) | priority-override |
| Effects (`internal/effect`) | **duplicate id = load error** (not overridable) |
| Slots (`internal/slot`) | duplicate id = load error |
| Channels (`internal/chat`), Emotes (`internal/emote`) | duplicate id **or** display-name/verb = load error |

**Practical consequence for a world pack:** `core:basic-strike` and
`wot:basic-strike` do **not** coexist — abilities are a shared bare-id space, so
`wot` only wins by declaring a higher `priority`; otherwise its definition is a
silent no-op. Reuse core's ability/effect/slot ids, or override on purpose —
never collide by accident.

---

## 2. Reserved property keys (`properties:`)

### 2a. Room properties — **registry-validated** (unregistered key = load error)

Every key in a room's `properties:` bag is checked against the property
registry at load (`validateRoomProperties`, `loader.go`). An unknown key fails
the boot. The engine baseline (`internal/pack/properties.go`,
`RegisterEngineBaselineProperties`):

| Key | Type | Meaning |
|-----|------|---------|
| `quest_grant` | string | Quest id auto-accepted on room entry (also valid on items). |
| `light` | string | Ambient override: `black`/`gloom`/`dim`/`lit` (light-and-darkness §2.4). |
| `dark_blocked` | bool | Room opts into the darkness movement hazard — a mover who can't see it at all is refused (§5.4). |
| `craft_stations` | map[string]int | Per-discipline station tier this room provides (`{smithing: 2}`); gates craft attempts + sets the quality ceiling (crafting §4). |

A pack adds its own room properties via `property.Registry.RegisterPack` in its
boot code (not used by the demo today). `key_for`/`rarity`/`essence`/`lit`/`fuel`
are registered but apply to **items**, not rooms (see `AppliesTo` in
`properties.go`).

### 2b. Item properties — **free-form** (not validated; engine acts on specific keys)

Item `properties:` are **not** validated against the registry — you may carry
any key for your own scripting. The engine only reads these:

| Key | Type | Meaning | Consumer |
|-----|------|---------|----------|
| `value` | int | Base worth; buy/sell price = `value × markup/discount`. Also the §8 craft-economy guardrail input. | `internal/economy/shop.go` |
| `rarity` | string | Decoration tier key (`common`/`uncommon`/…); colors the name. | `internal/decoration` |
| `essence` | string | Decoration essence key; glyph + color. | `internal/decoration` |
| `weapon_damage` | string | Damage dice `NdM` (e.g. `1d6`); validated at load. | `internal/combat` |
| `eligible_slots` | []string | Equipment slots the item may occupy (validated against the slot registry). | equip path |
| `companion_slots` | []string | Extra slots occupied when equipped (footprint, e.g. a greatsword taking `offhand`). | equip path |
| `consume_method` | string | `eat`/`drink` — the verb that consumes it. | `internal/economy` consumables |
| `sustenance_value` | int | Hunger restored on consume (economy-survival §6). | consumables |
| `effect_id` | string | Effect template id applied on consume (bare-global effect id). | consumables → effects |
| `quality_effects` | map[string]string | Rarity-tier → effect id; a craft stamps the highest tier ≤ rolled quality onto the output (crafting §6). | `internal/crafting` |
| `recipe` | string | A scroll/book teaches this (namespaced) recipe id; `read` learns + consumes it. | `internal/command/read.go` |
| `requires_skill` | string | Discipline id gating shop purchase. | `internal/economy/shop.go` |
| `requires_skill_level` | int | Min proficiency for `requires_skill` (default 1). | shop |
| `light` | string | Level this source contributes when lit. | `internal/light` |
| `lit` | bool | Source on/off; an **instance** property — survives pickup/drop. | `internal/light` |
| `fuel` | int | Remaining fuel; absent = permanent, 0 = spent. | `internal/light` |
| `key_for` | string | Door id this item unlocks. | door system |

Resource-node instances (spawned, not authored directly) also carry
`node_template` / `node_charges` / `node_yield_table` / `node_required_tool`
(`internal/gathering/node.go`) — authored on the **node template**, not loose
items.

---

## 3. Engine tags (`tags:`)

Strings in an entity's `tags:` list the engine branches on. Author these to opt
a room/item/mob into behavior. (Tags the engine *sets itself* are marked
**engine-managed** — don't author them.)

| Tag | On | Meaning | Const |
|-----|----|---------|-------|
| `safe-room` | room | Combat engagement is refused here. | `combat.TagSafeRoom` |
| `no-kill` | mob | Cannot be attacked. | `combat.TagNoKill` |
| `no-flee` | mob | Cannot flee combat. | `combat` (flee.go) |
| `skill_trainer` | mob | Teaches abilities (paired with a `trainer:` block). | `progression.TagSkillTrainer` |
| `shop` | mob | Is a vendor (paired with a `shop:` property block). | `economy.TagShop` |
| `no_sell` | item | Cannot be sold to a shop. | `economy.TagNoSell` |
| `no_remove` | item | Cannot be unequipped once worn. | `command` (equipment.go) |
| `fuel` | item | Can feed a built campfire (`build campfire` consumes one). | `command` (build.go) |
| `persistent` | spawn rule | Treat `count` as a ceiling, not a target — the mob is always present. | `spawn.TagPersistent` |
| `darkvision` | race flag | Viewer sees in darkness (raises the light floor). | `light.DarkvisionFlag` |
| `resource_node` | item (node) | Marks a harvestable node; **engine-managed** (set when a node spawns). | `gathering.NodeTag` |
| `no_get` | item (node) | Cannot be picked up; **engine-managed** on nodes. | `gathering.NoGetTag` |
| `mob` | mob | Marks any live mob for tag-index enumeration; **engine-managed**. | `entities.TagMob` |
| `alignment_evil` / `alignment_neutral` / `alignment_good` | entity | Alignment bucket; **engine-managed** (set from the `alignment` value). | `progression.alignment` |

**Tool tags are author-defined.** A gathering node's `required_tool` names a tag
(e.g. `mining-tool`, `woodcutting-tool`); any item carrying that tag satisfies
the gate. The engine has no fixed list — you pick the tag string and use it
consistently on the tool item and the node template.

---

## 4. Quick authoring guide (world-pack cheatsheet)

- **A vendor:** mob with `tags: [shop]` + a `properties: { shop: { sells: [...] } }` block. Sell ids are fully qualified.
- **A trainer:** mob with `tags: [skill_trainer]` + a `trainer:` block listing taught abilities.
- **A safe town room:** `tags: [safe-room]`.
- **A forge/kitchen:** room `properties: { craft_stations: { smithing: 2 } }`.
- **A weapon:** item `weapon_damage: "1d8"`, `eligible_slots: [wield]`, `properties: { value: N }`.
- **A torch/lantern:** item with `light` + `lit` + `fuel` properties and the `light` eligible slot.
- **A gathering tool:** item `tags: [mining-tool]` (any tag); the node template's `required_tool` names the same tag.
- **A recipe scroll:** item `properties: { recipe: "<pack>:<recipe-id>" }`; players `read` it.
- **A reused engine ability/effect:** reference its bare id (e.g. `smithing`, `well-fed`); to change it, redefine with a higher `priority` (abilities) — effects/slots can't be overridden, only added.
