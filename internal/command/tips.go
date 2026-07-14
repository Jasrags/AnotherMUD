package command

import (
	"context"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/light"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// Contextual tips (ui-rendering-help §12): one-time newbie hints shown as a
// player first encounters a situation. Each tip fires once ever per character,
// is opt-out via the `tips` verb, and is drip-fed one-per-room-view so a new
// player is never buried. The tip catalogue (ids + copy) lives here; the actor
// owns the shown-once + opt-out persistence (session.connActor).

// TipShower is the optional actor capability the room-view seam uses to show a
// one-time tip. ShowTipOnce returns whether the tip was actually shown (false
// when tips are disabled or the id was already shown), so the caller can fall
// through to the next candidate. Test/headless actors that don't implement it
// simply show no tips (mirrors AreaTracker / RoomDataViewer).
type TipShower interface {
	ShowTipOnce(ctx context.Context, id, text string) bool
}

// TipController is the optional actor capability the `tips` verb uses to read
// and change the opt-out and re-arm the shown-once set.
type TipController interface {
	TipsEnabled() bool
	SetTipsEnabled(on bool)
	ResetTips()
}

// Contextual-tip ids (the persisted shown-once keys) and their copy.
const (
	tipHelpID  = "help"
	tipShopID  = "shop"
	tipItemsID = "items"
	tipDarkID  = "dark"

	tipHelpText  = "New here? Type 'help' to see what you can do, or 'help getting-started' for a quick tour. ('tips off' to silence these.)"
	tipShopText  = "A merchant trades here — 'list' shows their wares, then 'buy <item>'."
	tipItemsText = "Something lies here — 'get <item>' to pick it up, or 'get all'."
	tipDarkText  = "It's too dark to see — 'light <torch>' if you carry one, or feel your way by the exits."
)

// maybeShowRoomTips fires at most one not-yet-seen contextual tip after a room
// view (ui-rendering-help §12). Candidates are ordered general → situational;
// ShowTipOnce returns false for an already-seen (or disabled) tip, so evaluation
// falls through to the next candidate and the player is drip-fed one new hint per
// room as they meet each situation. Called from writeRoomView, the single
// arrival-render seam (look, movement, flee, recall, teleport).
func (c *Context) maybeShowRoomTips(ctx context.Context, r *world.Room, lvl light.Level) {
	ts, ok := c.Actor.(TipShower)
	if !ok || r == nil {
		return
	}
	// Cheap opt-out gate: a player who disabled tips pays no per-view scan cost.
	if tc, ok := c.Actor.(TipController); ok && !tc.TipsEnabled() {
		return
	}

	if ts.ShowTipOnce(ctx, tipHelpID, tipHelpText) {
		return
	}
	if findShopInRoom(c, r.ID) != nil && ts.ShowTipOnce(ctx, tipShopID, tipShopText) {
		return
	}
	if roomHasGroundItem(c, r.ID) && ts.ShowTipOnce(ctx, tipItemsID, tipItemsText) {
		return
	}
	if lvl == light.Black && ts.ShowTipOnce(ctx, tipDarkID, tipDarkText) {
		return
	}
}

// roomHasGroundItem reports whether the room holds at least one visible loose
// item (not a mob). Mirrors findShopInRoom's placement walk and honors the
// per-observer quest-spawn gate so a foreign quest item doesn't trigger the tip.
func roomHasGroundItem(c *Context, roomID world.RoomID) bool {
	if c.Items == nil || c.Placement == nil {
		return false
	}
	for _, id := range c.Placement.InRoom(roomID) {
		e, ok := c.Items.GetByID(id)
		if !ok {
			continue
		}
		if c.questSpawnBlockedFrom(e) {
			continue
		}
		if _, ok := e.(*entities.ItemInstance); ok {
			return true
		}
	}
	return false
}

// TipsHandler implements the `tips` verb: bare shows status, `tips on|off`
// toggles the opt-out, `tips reset` re-arms every tip so the introductory hints
// show again. Bare `tips` reports rather than flipping (checking a preference
// shouldn't change it).
func TipsHandler(ctx context.Context, c *Context) error {
	ctrl, ok := c.Actor.(TipController)
	if !ok {
		return c.Actor.Write(ctx, "You can't change that right now.")
	}
	if len(c.Args) == 0 {
		state := "on"
		if !ctrl.TipsEnabled() {
			state = "off"
		}
		return c.Actor.Write(ctx, "Contextual tips are "+state+". Use 'tips on', 'tips off', or 'tips reset' to see them again.")
	}
	switch strings.ToLower(c.Args[0]) {
	case "on":
		ctrl.SetTipsEnabled(true)
		return c.Actor.Write(ctx, "Tips enabled — you'll get one-time hints as you explore.")
	case "off":
		ctrl.SetTipsEnabled(false)
		return c.Actor.Write(ctx, "Tips disabled.")
	case "reset":
		ctrl.ResetTips()
		return c.Actor.Write(ctx, "Tips reset — the introductory hints will show again.")
	default:
		return c.Actor.Write(ctx, "Usage: tips [on|off|reset]")
	}
}
