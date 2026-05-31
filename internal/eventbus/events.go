package eventbus

import (
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// Event-name constants. Spec text uses spaces ("entity equipped");
// the bus uses dots ("entity.equipped") because identifiers carry
// better through code. The mapping is one-to-one and lives here.
const (
	EventItemPickedUp     = "entity.item_picked_up"
	EventItemDropped      = "entity.item_dropped"
	EventEntityEquipped   = "entity.equipped"
	EventEntityUnequipped = "entity.unequipped"
	EventItemGiven        = "entity.item_given"
	// Cancellable pre-event fired before a put-in-container commits.
	// Spec inventory-equipment-items §4.5 step 5 — listeners can flip
	// the cancel flag to veto (locks, quest gates, etc.).
	EventContainerItemAdding = "container.item_adding"
	// Post-fact notification fired after a successful put commits.
	// Spec §4.5 step 7. Payload mirrors the pre-event.
	EventContainerItemAdded = "container.item_added"
	// Post-fact notification fired after a successful fill commits
	// (spec inventory-equipment-items §4.6 step 7). Fill has no
	// cancellable pre-event: the spec lists no veto hook and the
	// operation does not change ownership.
	EventItemFilled = "item.filled"
	// Post-fact notification fired after a mob has been instantiated
	// and placed in a room (spec mobs-ai-spawning §3.1 step 10).
	// No cancellable pre-event: spawn-time policy decisions belong
	// in the spawn config layer, not in subscriber veto.
	EventMobSpawned = "mob.spawned"
	// Post-fact notification fired after a player has entered a new
	// room (spec mobs-ai-spawning §5.2 — used to clear per-room
	// reaction state). Publishes on movement, login spawn, and
	// link-dead reconnect. Not cancellable.
	EventPlayerMoved = "player.moved"
	// Post-fact notification fired when a disposition evaluator
	// dispatches a fresh hostile reaction (spec
	// mobs-ai-spawning §5.5). Combat's engagement listener
	// subscribes; engagement's own duplicate guard absorbs
	// repeated emissions.
	EventMobAggro = "mob.aggro"
	// Post-fact notification fired on a fresh `wary` reaction
	// (spec §5.5). No engine subscriber today; content/quest
	// listeners may consume.
	EventMobWary = "mob.wary"
	// Post-fact notification fired on a fresh `friendly` reaction
	// (spec §5.5). Same listener story as EventMobWary.
	EventMobFriendly = "mob.friendly"
	// Post-fact notification fired by the area-tick scheduler at
	// each area's configured cadence (spec mobs-ai-spawning §3.7).
	// The spawn manager subscribes and runs §3.6 area-reset on the
	// signal. Carries a monotonic tick count + current player
	// count so subscribers can adapt without re-querying.
	EventAreaTick = "area.tick"
	// Cancellable pre-event fired before death-related disengagement
	// (spec combat §6.1). Listeners (resurrection skills, phylactery
	// items, soulbound effects) flip the embedded cancel flag to veto
	// the death; cancellers are responsible for restoring the victim
	// to a non-dead state. Carries the resolved killer attribution
	// (may be empty per §6.2) so listeners can react asymmetrically.
	EventDeathCheck = "combat.death_check"
	// Post-fact notification fired when an uncancelled death-check
	// commits (spec combat §6.3 step 1). Alignment / quest /
	// achievement listeners subscribe here.
	EventKill = "combat.kill"
	// Post-fact notification fired in addition to combat.kill when
	// the victim is a mob (spec combat §6.3 step 2). Carries the mob
	// template id so loot / XP / template-keyed quest credit can
	// match without a round-trip to the entity store. M7.5 wires the
	// spawn manager's untrack subscriber here so area respawn fires
	// on combat deaths.
	EventMobKilled = "mob.killed"
	// Post-fact notification fired after the player-death subscriber
	// has restored a dead player to a playable state (spec combat
	// §6.4: "Player death recovery ... is owned by another feature
	// subscribing to the vital-depleted or death events"). Carries
	// the player id + the from/to rooms so renderers, quest hooks,
	// and future XP-loss listeners can react.
	EventPlayerRespawned = "player.respawned"
	// Post-fact notification fired when a combatant successfully
	// flees through an exit (spec combat §5.2 step 3). Carries the
	// chosen direction and both rooms so renderers / quest hooks can
	// react without a follow-up world lookup.
	EventFlee = "combat.flee"
	// Post-fact notification fired when a flee attempt is refused
	// because the entity carries the no-flee tag (spec §5.2 step 1).
	EventFleePrevented = "combat.flee_prevented"
	// Post-fact notification fired when a flee attempt fails for an
	// environmental reason (no exits, missing room — spec §5.2
	// step 2). Distinct from prevented (which is a policy refusal)
	// so subscribers can render different messaging.
	EventFleeFailed = "combat.flee_failed"
	// XPGained fires after progression.Manager.GrantExperience adds
	// XP to a track (spec progression.md §5.4). Carries the source
	// string the granter passed in so quest / achievement listeners
	// can filter on origin without parsing names.
	EventXPGained = "progression.xp.gained"
	// XPLost fires after progression.Manager.DeductExperience
	// removes XP — only when the actual loss is > 0 (spec §5.5).
	EventXPLost = "progression.xp.lost"
	// LevelUp fires once per level-up step inside the cascade
	// triggered by GrantExperience (spec §5.4). A single grant that
	// crosses N thresholds emits N events.
	EventLevelUp = "progression.level.up"
	// TrackReset fires after ResetTrack (spec §5.7). No level-up
	// follows: the reset is downward.
	EventTrackReset = "progression.track.reset"
	// CharacterCreated fires once when a brand-new character enters
	// the world (spec progression.md §4.5 second bullet — the
	// path processor treats it as level 1). M8.4 publishes from the
	// session login path the first time it observes a fresh save
	// with no progression state; the M12 character-creation wizard
	// will own the canonical publish.
	EventCharacterCreated = "character.created"
	// AlignmentShiftCheck is the cancellable pre-event fired by
	// AlignmentManager.Shift before applying the change (spec
	// progression.md §6.4 step 3). Listeners may set the cancel
	// flag to abort, or rewrite SuggestedDelta to alter the
	// applied magnitude (e.g. an item that halves negative
	// shifts). Spec is explicit that admin-tagged entities never
	// reach this event — Shift's bypass returns before publish.
	EventAlignmentShiftCheck = "alignment.shift.check"
	// AlignmentShifted is the post-fact notification fired after
	// AlignmentManager.Shift successfully applies a delta (spec
	// §6.5 step 5). Fires only when the actual delta is non-zero;
	// a clamped-to-bounds shift does not emit.
	EventAlignmentShifted = "alignment.shifted"
	// AlignmentBucketChanged fires IN ADDITION to alignment.shifted
	// when the bucket boundary is crossed (spec §6.5 step 6). Two
	// events fire on a bucket-crossing shift, in this order.
	EventAlignmentBucketChanged = "alignment.bucket.changed"
	// EffectApplied fires after EffectManager.Apply successfully
	// installs a new active effect on a target (spec
	// abilities-and-effects §5.2 step 4). Single-instance
	// refusals do NOT emit.
	EventEffectApplied = "effect.applied"
	// EffectRemoved fires after RemoveByID / RemoveByFlag /
	// external dispel reverses an active effect (spec §5.3 step 4).
	// Expiration uses EventEffectExpired instead, so subscribers
	// can render different messaging for "spell wore off" vs "spell
	// was dispelled".
	EventEffectRemoved = "effect.removed"
	// EffectExpired fires after Tick decrements an effect's
	// remaining counter to zero and the batch expiration runs
	// (spec §5.4). Distinct from EffectRemoved so renderers can
	// distinguish duration-end from external removal.
	EventEffectExpired = "effect.expired"
	// AbilityUsed fires when the ability-resolution phase resolves a
	// queued invocation as a hit (spec abilities-and-effects §4.5
	// step 8).
	EventAbilityUsed = "ability.used"
	// AbilityMissed fires when a queued invocation resolves as a miss
	// (spec §4.5 step 6).
	EventAbilityMissed = "ability.missed"
	// AbilityFizzled fires when the per-pulse driver drops a queued
	// invocation that failed validation (spec §4.2 step 2, §4.8).
	EventAbilityFizzled = "ability.fizzled"
	// AbilityVitalDepleted fires when the resolver's post-hit death
	// check observes the target's HP at or below zero (spec §4.5
	// step 9). Distinct topic from combat.death_check; a subscriber
	// that bridges to the cancellable death flow forwards it.
	EventAbilityVitalDepleted = "ability.vital_depleted"
	// CurrencyCredited fires after CurrencyService.AddGold applies a
	// non-negative delta, and after every SetGold regardless of
	// direction (spec economy-survival §2.2). Amount is the absolute
	// magnitude requested; NewTotal is the post-mutation balance.
	EventCurrencyCredited = "currency.credited"
	// CurrencyDebited fires after CurrencyService.AddGold applies a
	// negative delta (§2.2). Amount is the absolute magnitude
	// requested (NOT the clamped change), so a debit of 100 against a
	// balance of 30 reports amount=100, NewTotal=0.
	EventCurrencyDebited = "currency.debited"
	// ShopBuy is the cancellable pre-event fired before a shop buy
	// charges the player (spec economy-survival §3.5 step 5 / §3.10).
	// A listener may Cancel() to veto the sale.
	EventShopBuy = "shop.buy"
	// ShopSell is the cancellable pre-event fired before a shop sell
	// transfers the item (spec §3.6 step 5 / §3.10).
	EventShopSell = "shop.sell"

	// RestStateChanged is the cancellable rest-state change event (spec
	// economy-survival §5.3 step 3). A listener may Cancel() to veto a
	// player-initiated transition. The combat-wake path (§5.4) publishes
	// the same event with Reason="combat" but ignores the veto.
	EventRestStateChanged = "entity.rest_state.changed"

	// ItemConsuming is the cancellable pre-event a consume fires before
	// spending a charge or destroying the item (spec economy-survival
	// §6.2 step 5). A listener may Cancel() to veto.
	EventItemConsuming = "item.consuming"
	// ItemConsumed fires after a successful consume but BEFORE the item
	// is destroyed (spec §6.2 step 9), so subscribers (the effects
	// feature) can still read the item's state and the carried effect
	// parameters.
	EventItemConsumed = "item.consumed"

	// EventDoorOpened / EventDoorClosed / EventDoorLocked /
	// EventDoorUnlocked fire after a successful door state mutation
	// from the corresponding verb (spec world-rooms-movement §5.2
	// step 5). The reverse-side door is mutated under the same world
	// lock as the near side but the event fires once — the payload
	// names the near room + direction so subscribers can derive the
	// reverse side themselves if they care.
	EventDoorOpened   = "door.opened"
	EventDoorClosed   = "door.closed"
	EventDoorLocked   = "door.locked"
	EventDoorUnlocked = "door.unlocked"
	// EventDoorBlocked fires when a move attempt is rejected
	// because the exit's door is closed (spec §3.3 step 4). The
	// payload tells listeners (renderer, AI, future scripting) why
	// the move failed so they can decorate the "you can't go that
	// way" path with door-specific text.
	EventDoorBlocked = "door.blocked"

	// EventPortalOpened fires after the portal service creates a
	// runtime keyword exit (spec world-rooms-movement §5.6).
	// CreatePaired emits once for the primary side; the partner
	// registration is silent.
	EventPortalOpened = "portal.opened"
	// EventPortalClosed fires after a portal is torn down — by
	// explicit Remove, by auto-expiry on area tick, or as the
	// paired-partner removal triggered by either of those. Paired
	// removals emit once for the primary side only.
	EventPortalClosed = "portal.closed"

	// EventRecallSet fires after `set recall` commits a new
	// recall address on a character (spec recall.md §5).
	EventRecallSet = "recall.set"
	// EventRecallBefore is the cancellable pre-event fired when
	// `recall` resolves a destination and is about to teleport
	// (spec recall.md §3.1 step 5 / §5). Subscribers (cooldowns,
	// holy ground, combat blocks supplied by content) flip the
	// cancel flag to veto. The actor-facing message on cancel is
	// intentionally generic so subscribers can write their own
	// specific reason.
	EventRecallBefore = "recall.before"
	// EventRecallAfter fires once after an uncancelled recall
	// teleport commits (spec recall.md §5). The room change
	// itself still publishes player.moved through the SetRoom
	// path; recall.after is the higher-level "the verb resolved"
	// signal that content packs subscribe to for cooldown/cost
	// post-hooks.
	EventRecallAfter = "recall.after"
	// EventRecallNoPoint fires when `recall` is invoked with no
	// saved recall point (spec recall.md §5). Observability-only
	// — useful for content packs that want to nudge new players
	// toward `set recall`.
	EventRecallNoPoint = "recall.no_point"
	// EventRecallUnresolved fires when the saved recall room no
	// longer resolves in the world registry — content drift
	// (spec recall.md §4 / §5). The operator log records the
	// missing room id; the actor-facing line stays generic.
	EventRecallUnresolved = "recall.unresolved"

	// EventWeatherChanged fires when an area's weather state
	// transitions to a different state (spec world-rooms-movement
	// §6.2 step 6). Identical-state rolls are no-ops and do NOT
	// publish. The payload carries both the previous and new
	// state so subscribers (renderers, scripting, analytics) can
	// react to the transition asymmetrically without keeping
	// their own previous-state shadow.
	EventWeatherChanged = "weather.changed"
)

// ItemPickedUp fires after GetHandler successfully moves an item
// from a room into a holder's contents (spec
// inventory-equipment-items §4.2 → "entity item picked up").
//
// Payload reflects the post-state: the item is now in HolderID's
// contents, no longer in RoomID's Placement entries.
type ItemPickedUp struct {
	HolderID entities.EntityID
	RoomID   world.RoomID
	ItemID   entities.EntityID
}

// Name implements Event.
func (ItemPickedUp) Name() string { return EventItemPickedUp }

// ItemDropped fires after DropHandler successfully moves an item
// from a holder's contents into a room (spec §4.3 → "entity item
// dropped"). Payload reflects the post-state: the item is in
// RoomID's Placement entries, no longer in HolderID's contents.
type ItemDropped struct {
	HolderID entities.EntityID
	RoomID   world.RoomID
	ItemID   entities.EntityID
}

// Name implements Event.
func (ItemDropped) Name() string { return EventItemDropped }

// EntityEquipped fires after EquipHandler successfully places an
// item in a slot (spec §3.3 step 7 → "entity equipped"). SlotName
// is the BASE slot name (no `:index` suffix) per §3.3 — the index
// is an internal disambiguator, not user-facing.
type EntityEquipped struct {
	HolderID entities.EntityID
	RoomID   world.RoomID
	ItemID   entities.EntityID
	SlotName string
}

// Name implements Event.
func (EntityEquipped) Name() string { return EventEntityEquipped }

// EntityUnequipped fires after UnequipHandler successfully removes
// an item from a slot (spec §3.4 step 4 → "entity unequipped").
// SlotName carries the base name only, matching §3.4's requirement
// that listeners see the base slot, never the index.
//
// The §3.4 `silent` mode (used by cleanup paths like dying entity
// drops everything) suppresses this event at the publisher's
// discretion. The bus has no notion of silent — that's a
// publisher-side choice.
type EntityUnequipped struct {
	HolderID entities.EntityID
	RoomID   world.RoomID
	ItemID   entities.EntityID
	SlotName string
}

// Name implements Event.
func (EntityUnequipped) Name() string { return EventEntityUnequipped }

// ItemGiven fires after GiveHandler successfully moves an item from
// one holder's contents into another's (spec inventory-equipment-
// items §4.4 step 4 → "entity item given"). Payload reflects the
// post-state: ItemID is in RecipientID's contents, no longer in
// GiverID's. RoomID is where the transfer happened (both holders
// must be in the same room). TemplateID carries the originating
// template so a future loot / quest listener can match without
// having to round-trip back to the entity store.
type ItemGiven struct {
	GiverID     entities.EntityID
	RecipientID entities.EntityID
	RoomID      world.RoomID
	ItemID      entities.EntityID
	ItemName    string
	TemplateID  string
}

// Name implements Event.
func (ItemGiven) Name() string { return EventItemGiven }

// ContainerItemAdding is the cancellable pre-event fired by
// PutHandler before the actor → container transfer commits (spec
// inventory-equipment-items §4.5 step 5). Listeners that flip the
// embedded CancelFlag abort the operation; the handler then returns
// the "cancelled" failure reason and emits no post-event.
//
// Payload reflects the *intended* post-state: ItemID is about to be
// placed inside ContainerID by ActorID. RoomID is the room where the
// put is happening (the actor's current room — useful for
// room-scoped quest listeners).
//
// The CancelFlag is a pointer so siblings later in the dispatch loop
// can observe an earlier listener's veto (per §dispatch semantics in
// internal/eventbus/event.go).
type ContainerItemAdding struct {
	*CancelFlag
	ActorID     entities.EntityID
	ContainerID entities.EntityID
	ItemID      entities.EntityID
	RoomID      world.RoomID
}

// Name implements Event.
func (ContainerItemAdding) Name() string { return EventContainerItemAdding }

// NewContainerItemAdding wires up the cancel flag so the publisher
// (PutHandler) does not have to remember to allocate it. Idiomatic
// constructor mirroring how cancellable events should be built —
// passing a zero-value struct would yield a nil CancelFlag and panic
// the moment a listener calls Cancel().
func NewContainerItemAdding(actor, container, item entities.EntityID, room world.RoomID) *ContainerItemAdding {
	return &ContainerItemAdding{
		CancelFlag:  &CancelFlag{},
		ActorID:     actor,
		ContainerID: container,
		ItemID:      item,
		RoomID:      room,
	}
}

// ContainerItemAdded fires after a successful put-in-container
// commits (spec §4.5 step 7). Post-state: ItemID is in ContainerID's
// contents, no longer in ActorID's inventory.
type ContainerItemAdded struct {
	ActorID     entities.EntityID
	ContainerID entities.EntityID
	ItemID      entities.EntityID
	RoomID      world.RoomID
}

// Name implements Event.
func (ContainerItemAdded) Name() string { return EventContainerItemAdded }

// ItemFilled fires after a successful fill commits (spec
// inventory-equipment-items §4.6 step 7). Post-state: TargetID's
// `charges` property is at `max_charges` and its `fill_type` is set
// to FillType; if SourceID declared a `fill_supply`, that value has
// been decremented by one.
//
// FillType is the liquid label (e.g. "water", "wine"). It comes from
// the source's `fill_source` property when set; otherwise the spec
// fallback "water" applies when the source carried the `fill_source`
// tag alone.
type ItemFilled struct {
	ActorID  entities.EntityID
	SourceID entities.EntityID
	TargetID entities.EntityID
	RoomID   world.RoomID
	FillType string
}

// Name implements Event.
func (ItemFilled) Name() string { return EventItemFilled }

// MobSpawned fires after a mob has been instantiated, placed in a
// room, and tracked in the entity store (spec mobs-ai-spawning §3.1
// step 10). TemplateID is carried verbatim so loot listeners, area
// reset accounting, and quest hooks can identify which template
// produced this instance without a round-trip to the entity store.
type MobSpawned struct {
	EntityID   entities.EntityID
	RoomID     world.RoomID
	TemplateID string
}

// Name implements Event.
func (MobSpawned) Name() string { return EventMobSpawned }

// PlayerMoved fires after a player's room has changed (spec
// mobs-ai-spawning §5.2). Sources today: movement command, login
// spawn (From is zero), and link-dead reconnect (From may equal To
// if the room hasn't changed but presence has).
//
// Used by the disposition evaluator to clear per-room reaction
// state for the moving player. Other subscribers (e.g. future quest
// triggers, scent trails) may attach later.
type PlayerMoved struct {
	PlayerID string
	From     world.RoomID
	To       world.RoomID
}

// Name implements Event.
func (PlayerMoved) Name() string { return EventPlayerMoved }

// MobAggro fires when the disposition evaluator dispatches a fresh
// hostile reaction (spec §5.5). Combat's engagement listener
// subscribes. RoomID is the location of the mob at dispatch time
// (always equal to the player's room since the evaluator is hooked
// at room-entry).
type MobAggro struct {
	MobID    entities.EntityID
	MobName  string
	PlayerID string
	RoomID   world.RoomID
}

// Name implements Event.
func (MobAggro) Name() string { return EventMobAggro }

// MobWary fires on a fresh wary reaction (spec §5.5).
type MobWary struct {
	MobID    entities.EntityID
	MobName  string
	PlayerID string
	RoomID   world.RoomID
}

// Name implements Event.
func (MobWary) Name() string { return EventMobWary }

// MobFriendly fires on a fresh friendly reaction (spec §5.5).
type MobFriendly struct {
	MobID    entities.EntityID
	MobName  string
	PlayerID string
	RoomID   world.RoomID
}

// Name implements Event.
func (MobFriendly) Name() string { return EventMobFriendly }

// AreaTick fires at each area's configured cadence (spec
// mobs-ai-spawning §3.7). TickCount is monotonic per area (not
// global). PlayerCount is the snapshot value the scheduler used to
// derive this cadence's "occupied modifier" — passed through so
// subscribers can branch on activity without re-querying the
// session manager.
type AreaTick struct {
	AreaID      world.AreaID
	TickCount   uint64
	PlayerCount int
}

// Name implements Event.
func (AreaTick) Name() string { return EventAreaTick }

// DeathCheck is the cancellable pre-event fired before combat-side
// death disengagement (spec combat §6.1). VictimID is the combatant
// identity (prefixed CombatantID-style string: "mob:<entityID>" or
// "player:<playerID>") of the entity at zero HP. KillerID carries the
// already-resolved attribution per §6.2 (explicit attacker on the
// vital-depleted event > victim's primary target > empty). Listeners
// MAY call Cancel() to veto disengagement; cancellers are responsible
// for restoring victim HP to a non-dead value before the next round.
//
// VictimTemplateID is populated only for mob victims and is carried
// here so a resurrection listener can decide whether THIS template
// resurrects without re-walking the entity store.
type DeathCheck struct {
	*CancelFlag
	VictimID         string
	VictimName       string
	KillerID         string
	KillerName       string
	RoomID           world.RoomID
	VictimIsMob      bool
	VictimTemplateID string
}

// Name implements Event.
func (DeathCheck) Name() string { return EventDeathCheck }

// NewDeathCheck constructs a cancellable death-check with the flag
// wired so the publisher doesn't have to remember to allocate it.
// Mirrors NewContainerItemAdding.
func NewDeathCheck(victimID, victimName, killerID, killerName string, room world.RoomID, isMob bool, templateID string) *DeathCheck {
	return &DeathCheck{
		CancelFlag:       &CancelFlag{},
		VictimID:         victimID,
		VictimName:       victimName,
		KillerID:         killerID,
		KillerName:       killerName,
		RoomID:           room,
		VictimIsMob:      isMob,
		VictimTemplateID: templateID,
	}
}

// Kill fires after an uncancelled DeathCheck (spec combat §6.3
// step 1). Killer fields may be empty when attribution is absent
// (environmental damage, ability one-shots without attribution).
type Kill struct {
	VictimID   string
	VictimName string
	KillerID   string
	KillerName string
	RoomID     world.RoomID
}

// Name implements Event.
func (Kill) Name() string { return EventKill }

// Flee fires when a combatant successfully fled through an exit
// (spec combat §5.2 step 3). The direction string carries the
// canonical world.Direction value (e.g. "north"); From and To are
// the source and destination room ids.
type Flee struct {
	EntityID   string
	EntityName string
	From       world.RoomID
	To         world.RoomID
	Direction  string
}

// Name implements Event.
func (Flee) Name() string { return EventFlee }

// FleePrevented fires when a flee attempt is refused because the
// entity carries the no-flee tag (spec §5.2 step 1). RoomID is the
// room the entity was in when the attempt was made.
type FleePrevented struct {
	EntityID   string
	EntityName string
	RoomID     world.RoomID
}

// Name implements Event.
func (FleePrevented) Name() string { return EventFleePrevented }

// FleeFailed fires when a flee attempt fails environmentally —
// either the entity is in an unknown room or the room has no exits
// (spec §5.2 step 2). RoomID may be empty when the entity has no
// tracked room. Reason is a short tag ("no-exits" / "unknown-room")
// so renderers can pick a message without a string match on prose.
type FleeFailed struct {
	EntityID   string
	EntityName string
	RoomID     world.RoomID
	Reason     string
}

// Name implements Event.
func (FleeFailed) Name() string { return EventFleeFailed }

// Reason values carried on FleeFailed.
const (
	FleeFailedNoExits     = "no-exits"
	FleeFailedUnknownRoom = "unknown-room"
)

// PlayerRespawned fires after the player-death subscriber has
// healed a dead player to a playable HP and moved them to the
// respawn room (spec combat §6.4). RespawnHP carries the post-
// respawn HP so listeners that want to render "you wake at 1 HP"
// don't have to round-trip back to vitals.
type PlayerRespawned struct {
	PlayerID   string
	PlayerName string
	From       world.RoomID
	To         world.RoomID
	RespawnHP  int
	// MaxHP is the player's max HP at respawn time so listeners can
	// compute RespawnHP as a percentage (renderers wanting "you
	// wake at 1/50 HP", future XP-loss policies that scale on the
	// fraction). Added at the same time as RespawnHP — listeners
	// that only care about the absolute value ignore it.
	MaxHP int
}

// Name implements Event.
func (PlayerRespawned) Name() string { return EventPlayerRespawned }

// MobKilled fires alongside Kill when the victim was a mob
// (spec combat §6.3 step 2). TemplateID is the mob template, which
// loot / XP / quest listeners key on.
type MobKilled struct {
	MobID      entities.EntityID
	MobName    string
	TemplateID string
	KillerID   string
	KillerName string
	RoomID     world.RoomID
}

// Name implements Event.
func (MobKilled) Name() string { return EventMobKilled }

// XPGained fires after a progression XP grant lands (spec
// progression.md §5.4). Source is the free-form attribution the
// granter passed in ("kill:mob:wolf", "quest:rescue", "admin").
// NewTotal is post-grant XP on this track.
type XPGained struct {
	EntityID string
	Track    string
	Amount   int64
	NewTotal int64
	Source   string
}

// Name implements Event.
func (XPGained) Name() string { return EventXPGained }

// XPLost fires after a progression XP deduction (spec §5.5).
// Amount is the actual loss (may be < what the caller requested
// if floored at the current level threshold).
type XPLost struct {
	EntityID string
	Track    string
	Amount   int64
	NewTotal int64
}

// Name implements Event.
func (XPLost) Name() string { return EventXPLost }

// LevelUp fires once per level-up step in a cascade (spec §5.4).
// A single grant crossing multiple thresholds publishes one
// LevelUp event per step. OldLevel + 1 == NewLevel always; both
// are present so subscribers can render the transition without
// arithmetic.
type LevelUp struct {
	EntityID string
	Track    string
	OldLevel int
	NewLevel int
}

// Name implements Event.
func (LevelUp) Name() string { return EventLevelUp }

// TrackReset fires after ResetTrack (spec §5.7). The post-reset
// state is always (level=1, xp=0); no fields needed beyond entity
// + track identity.
type TrackReset struct {
	EntityID string
	Track    string
}

// Name implements Event.
func (TrackReset) Name() string { return EventTrackReset }

// CharacterCreated fires once when a brand-new character enters
// the world (spec progression.md §4.5). M8.4's class-path
// processor subscribes and treats it as a level-1 grant; the
// M12 character-creation wizard will own the canonical publish.
// EntityID carries the playerID (combatant prefix omitted —
// progression keys are bare ids by convention, matching XPGained).
// ClassID is the resolved class id at creation, may be empty if
// the engine has no default class wired yet.
type CharacterCreated struct {
	EntityID string
	ClassID  string
}

// Name implements Event.
func (CharacterCreated) Name() string { return EventCharacterCreated }

// AlignmentShiftCheck is the cancellable pre-event fired by the
// alignment manager before applying a shift (spec
// progression.md §6.4 step 3). Listeners may flip the embedded
// CancelFlag to abort, or rewrite SuggestedDelta via the
// RewriteDelta method to alter the applied magnitude.
//
// EntityID is the bare id (no combat prefix). Reason carries the
// gameplay-side attribution string the caller passed in
// ("kill:mob:bandit-3", "quest:save-victor"). SuggestedDelta is
// the value the shift would apply absent any listener edit.
//
// The mutable SuggestedDelta sits behind a pointer so siblings
// later in the dispatch loop observe each earlier listener's
// rewrite — mirrors how the engine handles other cancellable
// events with mutable fields.
type AlignmentShiftCheck struct {
	*CancelFlag
	EntityID       string
	Reason         string
	suggestedDelta *int
}

// Name implements Event.
func (AlignmentShiftCheck) Name() string { return EventAlignmentShiftCheck }

// NewAlignmentShiftCheck constructs a fresh check event. The
// cancel flag is freshly allocated; the suggested-delta pointer
// is owned by the event so listeners can rewrite via
// RewriteDelta without a separate allocation.
func NewAlignmentShiftCheck(entityID, reason string, suggested int) *AlignmentShiftCheck {
	d := suggested
	return &AlignmentShiftCheck{
		CancelFlag:     &CancelFlag{},
		EntityID:       entityID,
		Reason:         reason,
		suggestedDelta: &d,
	}
}

// SuggestedDelta returns the current proposed delta. Reads the
// shared backing storage so siblings see prior rewrites.
func (e *AlignmentShiftCheck) SuggestedDelta() int {
	if e.suggestedDelta == nil {
		return 0
	}
	return *e.suggestedDelta
}

// RewriteDelta lets a listener override the proposed delta. The
// rewrite is observed by subsequent listeners and by the
// AlignmentManager when the dispatch completes.
func (e *AlignmentShiftCheck) RewriteDelta(v int) {
	if e.suggestedDelta == nil {
		d := v
		e.suggestedDelta = &d
		return
	}
	*e.suggestedDelta = v
}

// AlignmentShifted fires after an alignment shift successfully
// applies a non-zero delta (spec §6.5 step 5). OldValue +
// ActualDelta == NewValue always; BucketChanged is true iff the
// shift crossed a bucket threshold (in which case
// AlignmentBucketChanged also fires).
type AlignmentShifted struct {
	EntityID      string
	Reason        string
	OldValue      int
	NewValue      int
	ActualDelta   int
	BucketChanged bool
}

// Name implements Event.
func (AlignmentShifted) Name() string { return EventAlignmentShifted }

// AlignmentBucketChanged fires in addition to AlignmentShifted
// when the bucket boundary is crossed (spec §6.5 step 6). Carries
// both the old and new bucket names so subscribers (renderers,
// world tag index) can mirror the transition without a follow-up
// lookup.
type AlignmentBucketChanged struct {
	EntityID  string
	OldBucket string
	NewBucket string
}

// Name implements Event.
func (AlignmentBucketChanged) Name() string { return EventAlignmentBucketChanged }

// EffectApplied fires after EffectManager.Apply installs a new
// active effect on a target (spec abilities-and-effects §5.2
// step 4). EntityID is the target the effect was attached to;
// SourceAbilityID is the ability that produced the effect (empty
// when applied without an ability source — admin grant, world
// hook). Duration is the initial remaining-pulse counter (<0
// for permanent).
//
// Single-instance refusals (spec §5.2 step 2) do NOT publish.
type EffectApplied struct {
	EntityID        string
	EffectID        string
	SourceAbilityID string
	Duration        int
}

// Name implements Event.
func (EffectApplied) Name() string { return EventEffectApplied }

// EffectRemoved fires after RemoveByID / RemoveByFlag / external
// dispel reverses an active effect (spec §5.3 step 4). A flag-
// driven removal publishes one event per removed effect.
// Expiration emits EffectExpired instead so renderers can
// distinguish "wore off" from "dispelled".
type EffectRemoved struct {
	EntityID        string
	EffectID        string
	SourceAbilityID string
}

// Name implements Event.
func (EffectRemoved) Name() string { return EventEffectRemoved }

// EffectExpired fires after Tick's batch expiration removes an
// active effect whose remaining counter reached zero (spec §5.4).
// Distinct from EffectRemoved by event name only; payload shape
// matches so subscribers that care about identity-only can share
// a handler via a wrapper.
type EffectExpired struct {
	EntityID        string
	EffectID        string
	SourceAbilityID string
}

// Name implements Event.
func (EffectExpired) Name() string { return EventEffectExpired }

// AbilityUsed fires when the ability-resolution phase resolves a
// queued invocation as a hit (spec abilities-and-effects §4.5
// step 8). SourceID is the invoking entity; TargetID is the bare
// entity id the ability resolved against ("" for a pure self-cast).
// Category lets renderers distinguish "you cast …" (spell) from
// "you …" (skill).
type AbilityUsed struct {
	SourceID     string
	AbilityID    string
	AbilityName  string
	Category     string
	HandlerToken string
	TargetID     string
	TargetName   string
}

// Name implements Event.
func (AbilityUsed) Name() string { return EventAbilityUsed }

// AbilityMissed fires when a queued invocation resolves as a miss
// (spec §4.5 step 6).
type AbilityMissed struct {
	SourceID    string
	AbilityID   string
	AbilityName string
	TargetID    string
	TargetName  string
}

// Name implements Event.
func (AbilityMissed) Name() string { return EventAbilityMissed }

// AbilityFizzled fires when the per-pulse driver drops a queued
// invocation that failed validation (spec §4.2 step 2, §4.8).
// Reason is the lower-case keyword; clients SHOULD treat unknown
// reasons as opaque strings.
type AbilityFizzled struct {
	SourceID    string
	AbilityID   string
	AbilityName string
	Reason      string
}

// Name implements Event.
func (AbilityFizzled) Name() string { return EventAbilityFizzled }

// AbilityVitalDepleted fires when the resolver's post-hit death
// check observes the target at HP ≤ 0 (spec §4.5 step 9). KillerID
// is the invoking entity. A subscriber bridges this to the
// cancellable combat death flow; the separate topic keeps the
// progression layer from importing combat.
type AbilityVitalDepleted struct {
	VictimID string
	KillerID string
	Vital    string
}

// Name implements Event.
func (AbilityVitalDepleted) Name() string { return EventAbilityVitalDepleted }

// CurrencyCredited fires after gold is added to or set on an entity
// (spec economy-survival §2.2). EntityID is the bare holder id;
// Amount is the absolute magnitude of the change; Reason is the
// caller-supplied tag (e.g. "quest_reward", "pickup:<templateId>");
// NewTotal is the post-mutation balance.
type CurrencyCredited struct {
	EntityID string
	Amount   int
	Reason   string
	NewTotal int
}

// Name implements Event.
func (CurrencyCredited) Name() string { return EventCurrencyCredited }

// CurrencyDebited fires after gold is subtracted from an entity
// (spec §2.2). Field semantics mirror CurrencyCredited; Amount is
// the absolute magnitude requested, which may exceed the actual
// change when the balance floored at zero.
type CurrencyDebited struct {
	EntityID string
	Amount   int
	Reason   string
	NewTotal int
}

// Name implements Event.
func (CurrencyDebited) Name() string { return EventCurrencyDebited }

// ShopBuy is the cancellable pre-event a shop fires before charging a
// buyer (spec §3.5 step 5). ActorID is the buyer, NpcID the shop,
// TemplateID the item being bought, Price the computed buy price. A
// listener calls Cancel() to veto.
type ShopBuy struct {
	*CancelFlag
	ActorID    string
	NpcID      string
	TemplateID string
	Price      int64
}

// Name implements Event.
func (ShopBuy) Name() string { return EventShopBuy }

// NewShopBuy constructs a fresh cancellable buy pre-event.
func NewShopBuy(actorID, npcID, templateID string, price int64) *ShopBuy {
	return &ShopBuy{CancelFlag: &CancelFlag{}, ActorID: actorID, NpcID: npcID, TemplateID: templateID, Price: price}
}

// ShopSell is the cancellable pre-event a shop fires before taking an
// item from a seller (spec §3.6 step 5). Fields mirror ShopBuy;
// Price is the computed sell price.
type ShopSell struct {
	*CancelFlag
	ActorID    string
	NpcID      string
	TemplateID string
	Price      int64
}

// Name implements Event.
func (ShopSell) Name() string { return EventShopSell }

// NewShopSell constructs a fresh cancellable sell pre-event.
func NewShopSell(actorID, npcID, templateID string, price int64) *ShopSell {
	return &ShopSell{CancelFlag: &CancelFlag{}, ActorID: actorID, NpcID: npcID, TemplateID: templateID, Price: price}
}

// RestStateChanged is the cancellable event a RestService fires when an
// entity's rest state changes (spec economy-survival §5.3 step 3). On
// the player-initiated path a listener may Cancel() to veto. On the
// combat-wake path (§5.4) it is published with Reason="combat" as an
// informational notification — the publisher ignores the veto. OldState
// / NewState are the wire names ("awake"/"resting"/"sleeping").
type RestStateChanged struct {
	*CancelFlag
	EntityID string
	OldState string
	NewState string
	Reason   string
}

// Name implements Event.
func (RestStateChanged) Name() string { return EventRestStateChanged }

// NewRestStateChanged constructs a fresh cancellable rest-change event.
func NewRestStateChanged(entityID, oldState, newState, reason string) *RestStateChanged {
	return &RestStateChanged{CancelFlag: &CancelFlag{}, EntityID: entityID, OldState: oldState, NewState: newState, Reason: reason}
}

// ItemConsuming is the cancellable pre-event a consume fires before
// spending a charge / destroying the item (spec economy-survival §6.2
// step 5). ActorID is the consumer, ItemID the item, Method its
// consume_method. A listener calls Cancel() to veto.
type ItemConsuming struct {
	*CancelFlag
	ActorID entities.EntityID
	ItemID  entities.EntityID
	Method  string
}

// Name implements Event.
func (ItemConsuming) Name() string { return EventItemConsuming }

// NewItemConsuming constructs a fresh cancellable consume pre-event.
func NewItemConsuming(actorID, itemID entities.EntityID, method string) *ItemConsuming {
	return &ItemConsuming{CancelFlag: &CancelFlag{}, ActorID: actorID, ItemID: itemID, Method: method}
}

// ItemConsumed fires after a successful consume, before the item is
// destroyed (spec §6.2 step 9). It carries the effect parameters the
// effects feature subscribes to (§6.3) — the consume path does NOT
// apply the effect itself. EffectData is a transient int-keyed map; nil
// when the item declares none.
type ItemConsumed struct {
	ActorID         entities.EntityID
	ItemID          entities.EntityID
	ItemName        string
	Method          string
	EffectID        string
	EffectDuration  int
	EffectData      map[string]int
	SustenanceValue int
}

// Name implements Event.
func (ItemConsumed) Name() string { return EventItemConsumed }

// DoorEvent is the shared payload for door.opened / door.closed /
// door.locked / door.unlocked / door.blocked. Spec §5.2 step 5
// requires room id, direction (short form), actor id, door name on
// every door event; the lock/unlock events also carry the door's
// key id for subscribers that want to know which key was used.
//
// One struct serves all five events because the field set is the
// same; the event name distinguishes lifecycle stage. KeyID is
// empty on opened/closed/blocked (and on lock/unlock for a keyless
// door).
type DoorEvent struct {
	RoomID    world.RoomID
	Direction string // short-form ("n", "s", "e", "w", "u", "d")
	ActorID   entities.EntityID
	DoorName  string
	KeyID     string
}

// doorEventName picks the event name based on which builder was
// used. We keep five distinct constructors rather than a tagged-
// union because subscribers register by event name; the builder
// pattern keeps Name() trivial per type.

// DoorOpened is the door.opened payload.
type DoorOpened struct{ DoorEvent }

// Name implements Event.
func (DoorOpened) Name() string { return EventDoorOpened }

// DoorClosed is the door.closed payload.
type DoorClosed struct{ DoorEvent }

// Name implements Event.
func (DoorClosed) Name() string { return EventDoorClosed }

// DoorLocked is the door.locked payload.
type DoorLocked struct{ DoorEvent }

// Name implements Event.
func (DoorLocked) Name() string { return EventDoorLocked }

// DoorUnlocked is the door.unlocked payload.
type DoorUnlocked struct{ DoorEvent }

// Name implements Event.
func (DoorUnlocked) Name() string { return EventDoorUnlocked }

// DoorBlocked is the door.blocked payload. Fires from the
// movement-command path when world.Move returns ErrDoorClosed.
type DoorBlocked struct{ DoorEvent }

// Name implements Event.
func (DoorBlocked) Name() string { return EventDoorBlocked }

// PortalEvent is the shared payload for portal.opened / portal.closed.
// Carries enough state for renderers to announce the portal in the
// affected rooms and for subscribers (admin tooling, scripting) to
// react without a follow-up service lookup.
//
// Spec: world-rooms-movement §5.6.
type PortalEvent struct {
	PortalID    string
	SourceRoom  world.RoomID
	TargetRoom  world.RoomID
	Keyword     string // lowercased
	DisplayName string
	ExpiryTick  uint64 // 0 = no expiry
	PairedID    string // empty for one-way portals
}

// PortalOpened is the portal.opened payload.
type PortalOpened struct{ PortalEvent }

// Name implements Event.
func (PortalOpened) Name() string { return EventPortalOpened }

// PortalClosed is the portal.closed payload.
type PortalClosed struct{ PortalEvent }

// Name implements Event.
func (PortalClosed) Name() string { return EventPortalClosed }

// RecallSet is the recall.set payload — `set recall` committed a
// new bind point on the character. Spec recall.md §5.
type RecallSet struct {
	PlayerID string
	RoomID   world.RoomID
}

// Name implements Event.
func (RecallSet) Name() string { return EventRecallSet }

// RecallBefore is the cancellable pre-event fired by the recall
// verb after a destination is resolved and before the teleport
// commits (spec recall.md §3.1 step 5 / §5). Listeners flip the
// embedded CancelFlag to veto; the verb emits a generic
// "cancelled" message on veto so subscribers own the specific
// reason.
type RecallBefore struct {
	*CancelFlag
	PlayerID string
	From     world.RoomID
	To       world.RoomID
}

// Name implements Event.
func (RecallBefore) Name() string { return EventRecallBefore }

// NewRecallBefore constructs a cancellable recall.before with the
// flag wired so the publisher doesn't have to remember to allocate
// it. Mirrors NewDeathCheck / NewContainerItemAdding.
func NewRecallBefore(playerID string, from, to world.RoomID) *RecallBefore {
	return &RecallBefore{
		CancelFlag: &CancelFlag{},
		PlayerID:   playerID,
		From:       from,
		To:         to,
	}
}

// RecallAfter is the post-fact event fired after an uncancelled
// recall teleport commits (spec recall.md §5). The room-change
// itself still publishes player.moved through the SetRoom path;
// recall.after is the higher-level verb-resolved signal.
type RecallAfter struct {
	PlayerID string
	From     world.RoomID
	To       world.RoomID
}

// Name implements Event.
func (RecallAfter) Name() string { return EventRecallAfter }

// RecallNoPoint fires when `recall` runs against an empty save
// field (spec recall.md §5).
type RecallNoPoint struct {
	PlayerID string
}

// Name implements Event.
func (RecallNoPoint) Name() string { return EventRecallNoPoint }

// RecallUnresolved fires when the saved recall room id no longer
// resolves in the world registry (spec recall.md §4 / §5). The
// missing id is carried so the operator log can name it.
type RecallUnresolved struct {
	PlayerID    string
	MissingRoom world.RoomID
}

// Name implements Event.
func (RecallUnresolved) Name() string { return EventRecallUnresolved }

// WeatherChanged is the weather.changed payload — an area's weather
// state transitioned to a different value (spec world-rooms-movement
// §6.2). PreviousState may be empty on the very first roll of a
// freshly-loaded area that had no recorded state; the spec defaults
// the absence to `clear`, but listeners should treat empty as "no
// prior state" for first-roll asymmetry (e.g. a renderer that wants
// to skip the "rain ends" half on cold start).
type WeatherChanged struct {
	AreaID        world.AreaID
	PreviousState string
	NewState      string
}

// Name implements Event.
func (WeatherChanged) Name() string { return EventWeatherChanged }
