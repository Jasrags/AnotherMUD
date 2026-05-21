# Inventory, Equipment, and Items — Feature Specification

**Status:** Draft · **Scope:** Item templates, item instantiation, the
inventory operations (pick up / drop / give / put / fill), equipment
slots and wear/remove, stacking, keyword resolution, rarity/essence
presentation, and container access rules · **Audience:** Anyone
reimplementing or porting this feature in any language.

This document describes *what* the inventory/items feature must do,
not *how* to implement it. Specific slot lists, rarity tiers, essence
catalogs, weight units, and container capacities are policy and live
outside this spec.

---

## 1. Overview

Items in Tapestry are first-class entities, not flat data records.
Every concrete sword, potion, sack, or coin in the world is a runtime
entity with its own id, properties, tags, keywords, and (when
applicable) contents of its own. Item templates are content-defined
recipes for building those entities; the live entities live in the
world graph the same way mobs and players do.

The feature has six loosely-coupled responsibilities:

1. **Templates and item registry** — content-defined descriptions of
   what an item is, instantiated on demand.
2. **Inventory operations** — moving items between rooms, entities,
   and containers (pick up, drop, give, put, fill).
3. **Equipment** — wearing/wielding items in named slots, with stat
   modifiers as a side effect.
4. **Stacking** — grouping equivalent items for display without
   merging the underlying entities.
5. **Keyword resolution** — turning user-typed item references into
   concrete entity selections, including ordinals and `all` syntax.
6. **Presentation registries** — rarity tiers and essence types that
   format item names with color tags and decorators.

### Core concepts

- **Item template** — a content-defined record carrying id, name,
  base type, tags, keywords, free-form properties, and stat
  modifiers. The recipe for instantiating an item entity.
- **Item instance** — a runtime entity built from a template. Has a
  fresh id distinct from the template id, carries the template id as
  a property, and lives in the world's tracked-entity set.
- **Contents** — the per-entity ordered list of items held inside
  another entity (player inventory, mob inventory, container
  contents).
- **Equipment** — the per-entity map of slot keys to equipped item
  entities. Slot keys are either the slot name or, for multi-cap
  slots, `name:index`.
- **Slot** — a content-defined named position (e.g. `wield`, `head`,
  `finger`) with a maximum capacity (e.g. two rings on two fingers).
- **Carry weight** — a per-entity numeric ceiling on the summed
  weight of held items. Optional; absence means no limit.
- **Rarity tier** / **essence type** — content-defined display
  categories that decorate item names with color tags.

### Goals

1. Provide an item registry keyed by stable template id.
2. Instantiate items deterministically given the template inputs.
3. Provide inventory and equipment operations that respect tagged
   restrictions (no-get fixtures, public containers), capacity
   ceilings (carry weight, container capacity and weight limit),
   and well-defined failure reasons.
4. Apply and reverse equipment stat modifiers cleanly, tagged by a
   source that survives save/load.
5. Group equivalent items for display purposes without merging
   them at the entity level.
6. Resolve user-typed item references consistently (exact keyword
   → prefix → name substring; ordinal selection; `all` and
   `all.<keyword>`).
7. Emit observable events at every meaningful transition.

### Non-goals

- The command surface (`get`, `drop`, `wear`, `wield`, `give`,
  `put`, `fill`, `inventory`, `equipment`) — handled by the
  commands feature, which calls into the operations defined here.
- Currency handling. The pick-up and give operations consult an
  auto-conversion hook (so picking up coins increments a wallet
  property instead of producing item clutter), but the currency
  feature itself is separate.
- Consumable charge mechanics beyond fill semantics. Eat/drink/use
  belongs to the consumables feature.
- Item generation policies (loot rolls). The mob-spawning feature
  resolves loot tables and uses this feature's item registry to
  instantiate the results.
- Crafting / smithing / item modification.
- Persistent item state across save/load. Persistence is owned by
  another feature; this spec specifies which properties are
  transient versus durable (§2.4).

---

## 2. Templates and instantiation

### 2.1 Registration

Item templates register into a single registry keyed by stable
template id. The system MUST expose:

- Register a template (replacing any prior registration for the
  same id).
- Look up a template by id.
- Test whether a template id exists.
- Enumerate all templates and count them.

The registry MUST NOT require content to declare types at
registration; the template carries its own type string used when
the entity is built.

### 2.2 Template content

A template MUST carry:

- A stable id.
- A display name.
- A base entity type (typically `item`, but content may use other
  types — see §2.5 for the container case).

A template MAY carry:

- A tag list applied at instantiation.
- A keyword list (used by §6 keyword resolution).
- A free-form property bag merged onto the entity.
- A modifier list mapping stat names to integer values.

### 2.3 Instantiation

Creating an entity from a template MUST:

1. Build a fresh entity of the template's type with the template's
   name and a freshly-assigned runtime id.
2. Apply tags. The tag matching the entity's own type MUST NOT be
   re-applied (it is implicit).
3. Apply keywords.
4. Copy properties, with two rules:
   - The reserved key `room_id` MUST be filtered out — templates
     do not get to dictate where the instance is created.
   - Values that arrive as untyped map structures MUST be
     normalized into typed string-keyed dictionaries, recursively.
     (This handles YAML/JSON loader artifacts so downstream code
     always sees clean shapes.)
5. Set the template id property to the source template id so
   listeners (loot, quests, persistence, stacking) can identify
   the recipe later.
6. Build stat modifiers from the modifier list, **tagged by the
   freshly-assigned entity id** in the source key (so two swords
   from the same template never collide), and store them in a
   transient `modifiers` property on the entity for later equip-
   time application.

Instantiation MUST be deterministic given the template inputs and
the entity-id generator: the same template, instantiated twice,
produces structurally identical entities differing only in id.

### 2.4 Persistence shape

- The template id property persists with the entity.
- The `modifiers` property is **transient** — it is rebuilt from
  the template on reload, not stored. Applied stat-block changes
  on the *holder* still need to survive save/load; see §3.4.
- Tags, keywords, and the property bag persist with the entity per
  the persistence feature's rules.

### 2.5 Items vs containers

Containers (sacks, chests, fountains) are item templates whose
entity type may be `container` (or whose template carries the
container-related properties: `container_capacity`,
`container_weight_limit`, `public`, `fill_source`, etc.). The
inventory feature treats containers as items for pick-up/drop/give
purposes but recognizes them by type when applying put/fill
semantics (§4.4, §4.5).

**Acceptance criteria**

- [ ] Templates register by stable id; later registrations replace
      earlier ones for the same id.
- [ ] Instantiation produces a fresh entity carrying the template
      id property and a runtime id distinct from the template id.
- [ ] Stat modifiers from the template are stored as transient
      data; the holder's stat block is *not* mutated at
      instantiation.
- [ ] `room_id` from the template is silently dropped.
- [ ] Property values are normalized to typed dictionaries
      recursively.
- [ ] Two instances of the same template have non-colliding
      modifier source keys.

---

## 3. Equipment

### 3.1 Slot registry

Slots are registered at startup, both by the engine (e.g. baseline
body slots) and by packs. A registration carries:

- A slot name in snake_case. The system MUST reject hyphens and
  any non-snake_case identifier at registration time.
- A human-readable display label.
- A non-negative capacity (`max`).
- A scope tag (engine or pack name) for diagnostics.

Slot lookup is case-insensitive on name. Iteration over all slots
MUST preserve registration order.

### 3.2 Slot keys

When a slot has capacity 1, the slot key on an equipped item is
the slot name itself (e.g. `wield`).

When a slot has capacity greater than 1, slot keys carry an index
suffix: `name:0`, `name:1`, ..., `name:(max-1)`. The lowest free
index MUST be chosen at equip time so equipped items pack from
the start of the indexed range.

### 3.3 Equip

To equip an item into a named slot on a holder:

1. **Resolve the slot.** If the named slot is not registered, fail.
2. **Verify the item is in the holder's contents.** Equip only
   moves items from inventory to equipment, not from anywhere
   else. Fail if the item is not in contents.
3. **Auto-swap when full.** If the slot is at capacity, unequip
   the item currently occupying the first occupied slot key
   (slot name for cap-1 slots, `name:0` for multi-cap slots).
   The unequipped item becomes the operation's *displaced* result
   and is returned to the caller alongside success. If unequip
   fails (because no item is actually there despite the count),
   the equip operation fails as a whole.
4. **Select the slot key.** For cap-1 slots, the slot name. For
   multi-cap slots, the lowest unoccupied index.
5. **Move the item.** Add an entry from slot key to item in the
   holder's equipment; remove the item from the holder's contents.
6. **Apply stat modifiers.** Read the item's transient `modifiers`
   list (§2.3 step 6). For each modifier, add a stat modifier on
   the holder's stat block sourced by `equipment:<item entity id>`
   so the set can be reversed unambiguously later.
7. **Emit `entity equipped`** with the holder id, location room
   id, item id, and base slot name (no index suffix).

The operation returns `(success, displaced item or none)`.

### 3.4 Unequip

To unequip a slot key from a holder:

1. **Resolve the equipped item.** If no item is at the slot key,
   fail.
2. **Move the item.** Remove the equipment entry; add the item to
   the holder's contents.
3. **Remove stat modifiers** by source `equipment:<item entity
   id>`. This is a single source-keyed removal, not a per-modifier
   match, so the operation is symmetric with §3.3 step 6.
4. **Emit `entity unequipped`** with the holder id, item id, and
   *base* slot name (strip any `:index` suffix). The base name is
   what listeners (UI, persistence) care about; the index is an
   internal disambiguator.

The operation supports a `silent` mode that suppresses the event;
used during cleanup paths (e.g. dying entity drops everything)
where the caller will emit a more meaningful event of its own.

### 3.5 Persistence of equipment

Equipment is stored on the entity (slot key → item entity).
Applied stat modifiers on the holder's stat block persist with the
holder. On reload, the holder's modifiers are present from save;
the item's transient `modifiers` property is rebuilt from the
template; nothing further is applied. This is the symmetric
counterpart to §2.4 and is the reason source keys (§3.3 step 6,
§3.4 step 3) must be stable across save/load — they MUST be
derived from the item entity id, not from a transient runtime
handle.

**Acceptance criteria**

- [ ] Slot names must be snake_case; non-conforming names are
      rejected at registration.
- [ ] Equipping an item not in the holder's contents fails.
- [ ] A full slot triggers auto-swap, displacing the first
      occupied sub-slot and returning the displaced item.
- [ ] Multi-cap slot keys use `name:index` with indices packed
      from zero.
- [ ] Stat modifiers are sourced by item entity id so reversal is
      unambiguous.
- [ ] The `entity equipped` / `entity unequipped` events carry the
      base slot name without index suffix.
- [ ] Silent unequip suppresses the event.

---

## 4. Inventory operations

### 4.1 Common rules

Every operation in this section is a state-mutating, observable
transition. Each operation MUST:

- Validate preconditions atomically (no partial mutation on
  failure).
- Emit a single observable event on success (unless explicitly
  silenced by a caller).
- Return a structured result indicating success and, where
  applicable, a failure reason keyword from a fixed set.

The currency feature exposes a `try-auto-convert(holder, item)`
hook that the pick-up and give operations consult after the item
arrives in the new holder's contents. When auto-conversion
returns true the item entity has been consumed by the currency
feature, the operation reports success immediately, and no
inventory-feature event is emitted (the currency feature emits
its own event). When false the operation continues normally.

### 4.2 Pick up

To move an item from the actor's current room into the actor's
contents:

1. **Tag gate.** Reject items carrying the `fixture` or `no_get`
   tag. No event, no mutation.
2. **Weight check.** If the actor has a positive max-carry-weight
   property, compute the actor's current carry weight plus the
   item's weight. If the total exceeds the ceiling, fail. (A
   missing or zero max indicates "no limit".)
3. **Remove from room.** If the item has a location room, remove
   it from that room's entity list.
4. **Add to contents.**
5. **Currency auto-convert.** If the convert hook consumes the
   item, return success now.
6. **Emit `entity item picked up`** with actor id, item id,
   location room, item name, and source template id.

### 4.3 Drop

To move an item from the actor's contents into the actor's current
room:

1. Fail if the item is not in the actor's contents.
2. Remove from the actor's contents.
3. Add to the actor's current room.
4. Emit `entity item dropped` with actor id, item id, room, and
   name.

Dropping is *not* gated by weight, by tags, or by no-drop
restrictions in this spec; gates are policy that can be layered
on top.

### 4.4 Give

To move an item from one holder's contents into another holder's
contents:

1. Fail if the item is not in the giver's contents.
2. Remove from giver; add to recipient.
3. Currency auto-convert into the recipient. If consumed, return
   success.
4. Emit `entity item given` with giver id, recipient id, item id,
   item name, room, and template id.

### 4.5 Put in container

To move an item from the actor's contents into a container entity:

1. **Container check.** The target's type MUST be the container
   type. Otherwise return `not_container`.
2. **Accessibility check.** The container is accessible if any of:
   - The container is in the actor's contents (carrying their own
     pouch), OR
   - The container is in the actor's current room and has no
     parent container, OR
   - The container's `public` property is true (e.g. a town
     mailbox).
   If none apply, return `not_accessible`.
3. **Capacity check.** If the container declares a positive item-
   count capacity and is already at that count, return `full`.
4. **Weight check.** If the container declares a weight limit,
   sum the current contents' weights plus the item's weight. If
   over the limit, return `too_heavy`. (Items with no weight
   property contribute zero.)
5. **Cancellable pre-event.** Emit a `container item adding` event
   carrying container id, item id, and actor id. If any listener
   sets the cancel flag, return `cancelled`. Listeners use this
   to enforce ad-hoc rules (lock states, quest gates).
6. **Transfer.** Remove from actor; add to container.
7. Emit `container item added` with the same payload as the pre-
   event.

The `container item added` event is the canonical signal for
scripts; the cancellable pre-event is for veto only and SHOULD
NOT be treated as a state change.

### 4.6 Fill item

Filling is the rechargeable analogue of pouring: a *source*
(fountain, well, holy spring) refills a *target* (waterskin,
flask, phylactery) up to its max charges.

1. **Target must be fillable.** The target MUST declare a
   `max_charges` property. Otherwise return `not_fillable`.
2. **Source fill type.** The source's `fill_source` property
   names the liquid produced. If absent, fall back to "water"
   when the source carries a `fill_source` tag; otherwise return
   `no_fill_source`.
3. **Source supply.** If the source has a `fill_supply` property
   and the value is at most zero, return `source_empty`.
   Otherwise it represents remaining fills.
4. **Mixed-liquid guard.** If the target already carries a
   `fill_type` different from the source's, AND the target's
   current charges are positive, return `mixed_liquids`. Empty
   targets accept any liquid; matching liquids top up freely.
5. **Fill to max.** Set target charges to its max and target
   fill type to the source's fill type.
6. **Decrement source supply** if a supply value was declared.
7. Emit `item filled` with the actor id, source id, target id,
   and fill type.

Fill is the only operation that mutates a property on a *source*
entity that the actor does not own; this is intentional. Sources
that should be infinite (fountains) simply omit the supply
property.

**Acceptance criteria**

- [ ] Each operation atomically validates before mutating.
- [ ] Pick up rejects `fixture`/`no_get`; respects max carry
      weight; honors currency auto-conversion.
- [ ] Drop requires the item to be in contents.
- [ ] Put in container handles all five failure reasons:
      `not_container`, `not_accessible`, `full`, `too_heavy`,
      `cancelled`.
- [ ] The container pre-event is cancellable; listeners can veto.
- [ ] Fill handles all four failure reasons: `not_fillable`,
      `no_fill_source`, `source_empty`, `mixed_liquids`.
- [ ] Successful operations emit exactly one observable event
      (unless silenced).
- [ ] Currency-consumed pickups/gives suppress the inventory
      event.

---

## 5. Stacking

Stacking is a **presentation** operation, not a state mutation.
The system groups an entity's contents into stack entries so UIs
can render "3 healing potions" instead of three identical lines.
The underlying entities are NOT merged; each item retains its
distinct id, position in contents, and any per-instance state.

### 5.1 Stack key

Two items stack iff their stack keys match.

- Items without a template id never stack with anything. Each
  gets a singleton stack keyed by `notemplate:<entity id>`.
- Items with a template id stack by `<templateId>|<essence>`
  where essence is the item's essence property (empty string if
  absent).
- The stacking service exposes an extensibility hook
  (`add-key(propertyName)`) so packs can require additional
  properties to match before items stack. Each registered extra
  key contributes `|<value>` to the stack key (empty string if
  absent).

The order of contents is preserved in the stack list: the first
item of each unique stack key determines that stack's position
among the entries.

### 5.2 Stack entry

A stack entry carries:

- The shared template id (if any).
- A display name (the first item's name).
- A quantity (count of items in the stack).
- A rarity key (first item's rarity property).
- An essence key (first item's essence property, normalized to
  none if empty).
- The list of contained item ids in stack order.

### 5.3 Rarity and essence formatting

The rarity registry exposes formatted tag strings for use in text
output:

- **Inline format** renders a visible rarity tag wrapped in a
  color tag (e.g. `<item.rare>[rare]</item.rare>`). Hidden or
  decoratorless tiers render the empty string inline.
- **Padded format** renders a fixed-width tag column suitable for
  column-aligned lists. The padding width is computed once across
  all visible tiers (decorators plus the widest display text),
  then reused. Items with no rarity render whitespace of the same
  width, so columns stay aligned.

The essence registry exposes a single inline formatter:
`<essence.<key>>(<glyph>)</essence.<key>>`. Essence is treated as
a small symbolic mark, not a column entry.

Both registries are content-defined. Adding a new tier or essence
does not require engine changes; the formatter recomputes widths
on the next call.

**Acceptance criteria**

- [ ] Stacking is read-only on the contents list; iterating
      stacks does not mutate any entity.
- [ ] Items without a template id never stack.
- [ ] Extra stacking keys participate in the stack key in the
      order they were registered.
- [ ] Stack ordering preserves first-seen position within
      contents.
- [ ] Rarity padded width is computed from visible tiers' max
      decorators + max display text.
- [ ] Items with no rarity render padding-width whitespace in the
      padded formatter.

---

## 6. Keyword resolution

Player input refers to items by short keywords (`sword`, `red
potion`, `2.ring`, `all.gem`, `all`). This feature provides a
shared resolver used by every command and operation that takes
an item argument. The resolver works against any iterable of
entities (room contents, holder contents, container contents,
the actor's equipped items).

### 6.1 Single-entity selection

Given a non-empty keyword string, the resolver returns the first
matching entity or none. Selection rules apply in this order:

1. **Ordinal selector.** If the keyword has the form
   `<positive integer>.<keyword>`, the integer is a 1-based
   ordinal selecting among matches for `<keyword>`. Returns none
   if the ordinal is out of range. A leading `0.` or negative
   prefix does NOT take this path; it falls through to step 2.
2. **Exact keyword match.** The first entity whose registered
   keywords contain the input.
3. **Keyword prefix match.** The first entity carrying a keyword
   that starts with the input (and is longer than the input, so
   exact matches don't satisfy "prefix").
4. **Name substring match.** The first entity whose display name
   contains the input as a case-insensitive substring.

Matching is case-insensitive throughout.

### 6.2 Multi-entity selection

Given a non-empty keyword string, the resolver may also return a
list of all matches:

- The keyword `all` (case-insensitive) returns every entity.
- The keyword `all.<keyword>` returns every entity matching
  `<keyword>` by the same exact-then-prefix-then-substring rules
  as §6.1.
- Any other keyword returns every entity satisfying
  exact-or-prefix-or-substring against the input.

Multi-entity selection MUST preserve iteration order of the input
sequence.

### 6.3 Empty input

Empty or whitespace keywords return none (single-select) or an
empty list (multi-select). Callers MUST NOT rely on empty input
matching "everything".

**Acceptance criteria**

- [ ] The ordinal selector handles ranges correctly and returns
      none on out-of-range.
- [ ] Match precedence is exact → prefix → substring.
- [ ] Prefix matches require the candidate keyword to be longer
      than the input.
- [ ] `all` returns every entity; `all.<keyword>` filters by
      keyword.
- [ ] Empty input never matches anything.

---

## 7. Observable events

The features publish at least these events. Each event carries
enough payload that observers can act without querying further
state in the common case.

| Event | When |
|---|---|
| entity equipped | an item was placed in a slot (§3.3) |
| entity unequipped | an item was removed from a slot (§3.4) |
| entity item picked up | an item moved from room into a holder (§4.2) |
| entity item dropped | an item moved from a holder to a room (§4.3) |
| entity item given | an item moved between holders (§4.4) |
| container item adding | cancellable pre-event for put-in-container (§4.5) |
| container item added | an item was placed into a container (§4.5) |
| item filled | a fillable item was topped up from a source (§4.6) |

The `container item adding` event is the only cancellable one;
listeners that set the cancel flag prevent the put operation from
proceeding.

The currency feature consumes pick-up and give operations via the
auto-convert hook and emits its own events; those are out of
scope here.

---

## 8. Configuration surface

The following are externally configurable and not fixed by this
spec.

| Policy | Where it applies |
|---|---|
| Slot list and per-slot capacity | §3.1 |
| Default max carry weight (or none) | §4.2 |
| Container capacity / weight limit defaults | §4.5 |
| Rarity tiers (key, order, display text, decorators, color, visibility) | §5.3 |
| Essence catalog (key, glyph, color) | §5.3 |
| Reserved tag names (`fixture`, `no_get`, `public`, `fill_source`) | §4 |
| Extra stacking keys | §5.1 |
| Currency auto-conversion hook | §4.1 |

---

## 9. Open questions / future work

- **No-drop / soulbound restrictions.** Drop is unconditional;
  there is no `no_drop` tag analogue to `no_get`. Quest items
  that should not be droppable would need either a tag check
  layered in here or in the drop command handler.
- **Equip restrictions.** Equip rejects items not in contents
  and full slots (with auto-swap), but does not enforce class
  restrictions, alignment restrictions, level requirements, or
  cursed/equipped-by-restricted-tag rules. Currently those are
  expected to be enforced in the command layer or via tags.
- **Auto-swap semantics on multi-cap slots.** Auto-swap always
  displaces sub-slot `:0`. A "least-recently-equipped" or
  "lowest-modifier" policy might serve players better but
  requires per-slot bookkeeping.
- **Stat modifier source keying.** Source keys derive from the
  item entity id. This means renaming or migrating an item id
  invalidates the modifier reversal. A more robust key (e.g.
  `equipment:<entity id>:<slot key>`) would survive any future
  feature that wants to apply *different* modifiers to the same
  item in different slots.
- **Container nesting.** The accessibility check requires the
  container to be in the actor's inventory, in the actor's room
  without a parent, or marked public. Containers held inside
  another container ("a pouch inside a chest") are accessible
  only if the outermost ancestor satisfies the rule — but the
  check today inspects only one level. Deep nesting may not
  behave as expected.
- **Fill-from-container.** Fill assumes the source is a static
  entity in the room (a fountain). Filling a flask from a
  bottle (each loses one charge per pour) is not specified.
- **Stack rendering vs commands.** Stacking is read-only, but
  user commands like `drop 3 potion` would naturally want to
  speak in stack quantities. Mapping stack quantities back to
  underlying item ids is a command-layer concern; the resolver
  in §6 does not understand quantities.
- **Currency auto-convert as a side channel.** The hook makes
  pick-up's success message silently disappear for coins, which
  is intentional today but easy to miss when porting. A
  documented post-condition (no event from this feature when
  auto-conversion fires) is the canonical contract.
- **Rarity / essence presentation lives in the engine.** Both
  registries are engine-level today but produce pack-specific
  output. The renderers could move closer to the UI feature.

---

<!-- Generated: 2026-05-21 · Scope: ItemTemplate + ItemRegistry + EquipmentManager + InventoryManager + StackingService + KeywordMatcher + SlotRegistry + RarityRegistry + EssenceRegistry + ContainerProperties · Spec style: narrative + acceptance criteria · Detail level: behavior only -->
