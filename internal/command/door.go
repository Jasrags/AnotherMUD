package command

import (
	"context"
	"fmt"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/eventbus"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// OpenHandler implements `open <target>` (M15.1). The target is a
// direction or a door keyword (with optional ordinal); the world's
// ResolveDoorTarget produces a Direction. A locked door cannot be
// opened — the verb routes through UnlockDoor implicitly only when
// the player explicitly types `unlock`.
//
// Spec: docs/specs/world-rooms-movement.md §5.2 (Open operation),
// §5.5 (target resolution).
func OpenHandler(ctx context.Context, c *Context) error {
	return doorOpHandler(ctx, c, "open")
}

// CloseHandler implements `close <target>`.
func CloseHandler(ctx context.Context, c *Context) error {
	return doorOpHandler(ctx, c, "close")
}

// LockHandler implements `lock <target>`. Requires the actor to
// hold the door's key item (when the door declares a KeyID).
func LockHandler(ctx context.Context, c *Context) error {
	return doorOpHandler(ctx, c, "lock")
}

// UnlockHandler implements `unlock <target>`. Same key-check as
// Lock.
func UnlockHandler(ctx context.Context, c *Context) error {
	return doorOpHandler(ctx, c, "unlock")
}

// PickHandler implements `pick <target>` (alias `picklock`) — the lockpicking
// skill verb (skills §4), a keyless alternative to unlock gated on the Open
// Lock skill vs the door's pick difficulty.
func PickHandler(ctx context.Context, c *Context) error {
	return doorOpHandler(ctx, c, "pick")
}

// doorOpHandler is the shared verb implementation. The op string
// is the verb's name; chosen so the user-facing copy reads
// naturally without a switch in every error path.
func doorOpHandler(ctx context.Context, c *Context, op string) error {
	if c.World == nil {
		return c.Actor.Write(ctx, "There is nothing here to "+op+".")
	}
	room := c.Actor.Room()
	if room == nil {
		return c.Actor.Write(ctx, "You see nothing here.")
	}

	// M17.2c/d: the `door` arg resolved the target (a direction or door
	// keyword, with optional ordinal) before this runs — missing-arg,
	// ambiguous, and not-found are reported by the dispatcher with the
	// §5.4 / door sentinels. We parse the resolved short direction back
	// to a world.Direction and re-fetch the LIVE DoorState: the snapshot
	// the resolver took may be stale, and the per-op checks below want
	// current state (mirrors the old GetDoor-before-switch behavior).
	ref, ok := c.Resolved["door"].(DoorRef)
	if !ok {
		return c.Actor.Write(ctx, fmt.Sprintf("You don't see anything to %s there.", op))
	}
	dir, ok := world.ParseDirection(ref.Direction)
	if !ok {
		return c.Actor.Write(ctx, fmt.Sprintf("You don't see anything to %s there.", op))
	}
	door, ok := c.World.GetDoor(room.ID, dir)
	if !ok {
		return c.Actor.Write(ctx, fmt.Sprintf("You don't see anything to %s there.", op))
	}

	switch op {
	case "open":
		return handleOpen(ctx, c, room.ID, dir, door)
	case "close":
		return handleClose(ctx, c, room.ID, dir, door)
	case "lock":
		return handleLock(ctx, c, room.ID, dir, door)
	case "unlock":
		return handleUnlock(ctx, c, room.ID, dir, door)
	case "pick":
		return handlePick(ctx, c, room.ID, dir, door)
	default:
		return c.Actor.Write(ctx, "Huh?")
	}
}

// skillOpenLock is the ability id of the lockpicking skill (skills §4 — a
// `skill`-category, trained-only ability). The `pick` verb checks/gains it.
const skillOpenLock = "open-lock"

// statValuer is the actor surface the skill check reads a governing ability
// score from (connActor satisfies it; tests supply a fake).
type statValuer interface {
	StatValue(progression.StatType) int
}

// actorStatReader adapts a statValuer to progression.StatReader (the
// RollUseGain stat-factor seam); the actor IS the entity, so entityID is
// ignored.
type actorStatReader struct{ sv statValuer }

func (r actorStatReader) StatValue(_ string, s progression.StatType) int { return r.sv.StatValue(s) }

// handlePick implements `pick <door>` (skills §4): an Open-Lock skill check
// (skills §3) against the door's pick difficulty — a keyless alternative to
// `unlock`. Success drives the same unlock transition + side-sync; failure
// leaves the door locked and is noisy (the room hears the fumble). The skill
// gains with use either way (reduced on a miss). The key path is untouched.
func handlePick(ctx context.Context, c *Context, src world.RoomID, dir world.Direction, door world.DoorState) error {
	if c.Proficiency == nil || c.SkillRoller == nil {
		return c.Actor.Write(ctx, "You can't do that right now.")
	}
	if door.KeyID == "" {
		return c.Actor.Write(ctx, fmt.Sprintf("There's no lock on %s to pick.", door.Name))
	}
	// A non-pickable door, or a pickable one with no positive difficulty
	// (a content slip — pick_difficulty 0 would auto-succeed), can't be
	// picked. Guarding here keeps a mis-authored door from being a free lock.
	if !door.Pickable || door.PickDifficulty <= 0 {
		return c.Actor.Write(ctx, fmt.Sprintf("%s's lock can't be picked.", capitalize(door.Name)))
	}
	if !door.Locked {
		return c.Actor.Write(ctx, fmt.Sprintf("%s isn't locked.", capitalize(door.Name)))
	}
	// Trained-only (skills §2): no proficiency ⇒ you don't know how.
	prof, trained := c.Proficiency.Proficiency(c.Actor.PlayerID(), skillOpenLock)
	if !trained {
		return c.Actor.Write(ctx, "You don't know how to pick locks.")
	}

	// Governing stat = the skill ability's gain stat (Open Lock keys off
	// Dexterity); default Dex when the ability isn't loaded.
	gov := progression.StatDEX
	if c.Abilities != nil {
		if ab, ok := c.Abilities.Get(skillOpenLock); ok && ab.GainStat != "" {
			gov = ab.GainStat
		}
	}
	sv, _ := c.Actor.(statValuer)
	statScore := 10
	if sv != nil {
		statScore = sv.StatValue(gov)
	}
	bonus := progression.SkillBonus(prof, statScore, progression.DefaultSkillConfig())
	// EPIC S4 Phase 3c: a Skill Emphasis feat on this skill adds a flat bonus.
	if fb, ok := c.Actor.(featSkillBonuser); ok {
		bonus += fb.FeatSkillBonus(skillOpenLock)
	}
	// Tool bonus (skills.md tool seam + masterwork §3): a carried lockpick
	// aids the Open-Lock check; a graded one aids it more. Best-applies across
	// carried tools (non-stacking).
	bonus += c.skillToolBonus(skillOpenLock)
	// Armor check penalty (armor-depth §6): a worn armor/shield's check penalty
	// reduces Str- and Dex-based skill checks (Open Lock keys off Dex). The
	// total worn penalty is carried on the armor_check stat (applied at equip,
	// grade-reduced); subtract it before the roll. Gated to Str/Dex skills so a
	// future non-physical skill is unaffected (§6).
	if sv != nil && (gov == progression.StatSTR || gov == progression.StatDEX) {
		bonus -= sv.StatValue(progression.StatType(statKeyArmorCheck))
	}
	outcome := progression.ResolveSkillCheck(c.SkillRoller, bonus, door.PickDifficulty)

	// The skill improves on every attempt (the existing use-gain loop, halved
	// on a miss by the ability's gain-failure multiplier).
	var stats progression.StatReader
	if sv != nil {
		stats = actorStatReader{sv}
	}
	c.Proficiency.RollUseGain(c.Actor.PlayerID(), skillOpenLock, outcome.Success, c.SkillRoller, stats)

	if !outcome.Success {
		// Friction (skills §4): a failed pick is noisy — the room hears it,
		// so picking in company has a cost even if you can free-retry.
		if c.Broadcaster != nil {
			c.Broadcaster.SendToRoom(ctx, src,
				fmt.Sprintf("%s fumbles at %s's lock.", c.Actor.Name(), door.Name), c.Actor.PlayerID())
		}
		return c.Actor.Write(ctx, fmt.Sprintf("You fail to pick %s's lock.", door.Name))
	}
	if !c.World.UnlockDoor(src, dir) {
		return c.Actor.Write(ctx, fmt.Sprintf("%s won't unlock.", capitalize(door.Name)))
	}
	// Empty KeyID: a pick uses no key, so a subscriber can't mistake it for a
	// keyed unlock (mirrors how open/close pass "").
	c.Publish(ctx, eventbus.DoorUnlocked{DoorEvent: doorEvent(c, src, dir, door, "")})
	if c.Broadcaster != nil {
		c.Broadcaster.SendToRoom(ctx, src,
			fmt.Sprintf("%s picks %s's lock.", c.Actor.Name(), door.Name), c.Actor.PlayerID())
	}
	return c.Actor.Write(ctx, fmt.Sprintf("You deftly pick %s's lock.", door.Name))
}

func handleOpen(ctx context.Context, c *Context, src world.RoomID, dir world.Direction, door world.DoorState) error {
	if door.Locked {
		return c.Actor.Write(ctx, fmt.Sprintf("%s is locked.", capitalize(door.Name)))
	}
	if !door.Closed {
		return c.Actor.Write(ctx, fmt.Sprintf("%s is already open.", capitalize(door.Name)))
	}
	if !c.World.OpenDoor(src, dir) {
		return c.Actor.Write(ctx, fmt.Sprintf("%s won't budge.", capitalize(door.Name)))
	}
	c.Publish(ctx, eventbus.DoorOpened{DoorEvent: doorEvent(c, src, dir, door, "")})
	return c.Actor.Write(ctx, fmt.Sprintf("You open %s.", door.Name))
}

func handleClose(ctx context.Context, c *Context, src world.RoomID, dir world.Direction, door world.DoorState) error {
	if door.Closed {
		return c.Actor.Write(ctx, fmt.Sprintf("%s is already closed.", capitalize(door.Name)))
	}
	if !c.World.CloseDoor(src, dir) {
		return c.Actor.Write(ctx, fmt.Sprintf("%s won't budge.", capitalize(door.Name)))
	}
	c.Publish(ctx, eventbus.DoorClosed{DoorEvent: doorEvent(c, src, dir, door, "")})
	return c.Actor.Write(ctx, fmt.Sprintf("You close %s.", door.Name))
}

func handleLock(ctx context.Context, c *Context, src world.RoomID, dir world.Direction, door world.DoorState) error {
	// Policy (world-rooms-movement §5.3): a door is lockable only if it
	// has a lock — i.e. it declares a key. A keyless door is a plain door,
	// not a free latch, so refuse before the close/lock-state checks.
	if door.KeyID == "" {
		return c.Actor.Write(ctx, fmt.Sprintf("There's no lock on %s.", door.Name))
	}
	if !door.Closed {
		return c.Actor.Write(ctx, fmt.Sprintf("You'll need to close %s first.", door.Name))
	}
	if door.Locked {
		return c.Actor.Write(ctx, fmt.Sprintf("%s is already locked.", capitalize(door.Name)))
	}
	if !actorHasKey(c, door.KeyID) {
		return c.Actor.Write(ctx, fmt.Sprintf("You don't have a key for %s.", door.Name))
	}
	if !c.World.LockDoor(src, dir) {
		return c.Actor.Write(ctx, fmt.Sprintf("%s won't lock.", capitalize(door.Name)))
	}
	c.Publish(ctx, eventbus.DoorLocked{DoorEvent: doorEvent(c, src, dir, door, door.KeyID)})
	return c.Actor.Write(ctx, fmt.Sprintf("You lock %s.", door.Name))
}

func handleUnlock(ctx context.Context, c *Context, src world.RoomID, dir world.Direction, door world.DoorState) error {
	// A keyless door has no lock to work (mirror of handleLock policy).
	if door.KeyID == "" {
		return c.Actor.Write(ctx, fmt.Sprintf("There's no lock on %s.", door.Name))
	}
	if !door.Locked {
		return c.Actor.Write(ctx, fmt.Sprintf("%s isn't locked.", capitalize(door.Name)))
	}
	if !actorHasKey(c, door.KeyID) {
		return c.Actor.Write(ctx, fmt.Sprintf("You don't have a key for %s.", door.Name))
	}
	if !c.World.UnlockDoor(src, dir) {
		return c.Actor.Write(ctx, fmt.Sprintf("%s won't unlock.", capitalize(door.Name)))
	}
	c.Publish(ctx, eventbus.DoorUnlocked{DoorEvent: doorEvent(c, src, dir, door, door.KeyID)})
	return c.Actor.Write(ctx, fmt.Sprintf("You unlock %s.", door.Name))
}

// doorEvent builds the shared DoorEvent payload for the five door
// lifecycle events. KeyID is only meaningful on lock / unlock; the
// open / close / blocked builders pass an empty string.
func doorEvent(c *Context, src world.RoomID, dir world.Direction, door world.DoorState, keyID string) eventbus.DoorEvent {
	return eventbus.DoorEvent{
		RoomID:    src,
		Direction: dir.Short(),
		ActorID:   entities.EntityID(c.Actor.PlayerID()),
		DoorName:  door.Name,
		KeyID:     keyID,
	}
}

// actorHasKey reports whether the actor's inventory carries any
// item whose template id equals keyID (case-insensitive) OR whose
// `key_for` property names a door that resolves to keyID. The
// first form is the spec's literal §5.3 check; the second is the
// PD-4 hook so content can declare a key with a property rather
// than expecting the door to name an item by template id.
//
// Returns false when Items / Templates are unwired (test envs) —
// the verb's caller renders a clear "no key" message.
func actorHasKey(c *Context, keyID string) bool {
	if c.Items == nil || keyID == "" {
		return false
	}
	wantTpl := item.TemplateID(strings.ToLower(strings.TrimSpace(keyID)))
	for _, id := range c.Actor.Inventory() {
		ent, ok := c.Items.GetByID(id)
		if !ok {
			continue
		}
		it, ok := ent.(*entities.ItemInstance)
		if !ok {
			continue
		}
		// Direct template id match (spec §5.3).
		if it.TemplateID() == wantTpl {
			return true
		}
		// PD-4 property hook: an item declares `key_for: <door-id>`
		// to act as a key for any door whose KeyID matches.
		if pv, ok := it.Property("key_for"); ok {
			if s, _ := pv.(string); strings.EqualFold(s, keyID) {
				return true
			}
		}
	}
	return false
}
