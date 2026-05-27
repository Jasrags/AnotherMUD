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
	EntityID        string
	Reason          string
	suggestedDelta  *int
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
