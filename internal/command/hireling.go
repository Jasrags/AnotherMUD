package command

import (
	"context"
	"fmt"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/economy"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// This file holds the hireling lifecycle verbs (hireable-mobs.md §3): `hire` a
// companion, `dismiss` it, and `hirelings` to list them. It mirrors the mount
// surface (mount.go) — a hireling fights where a mount carries. Acquisition is
// model (b): `hire <name>` resolves against every hireable mob template (those
// carrying a `hireling:` block) by name, with no recruiter access point in v1.

// HirelingService is the runtime hireling lifecycle the command layer depends on
// (hireable-mobs.md §2, §3) — implemented at the composition root over the mob
// spawn pipeline. The durable ownership records live on the player save
// (hirelingOwner).
type HirelingService interface {
	// FindHireable resolves a hireling by name (or id) among all hireable mob
	// templates, returning its template id, display name, and hire cost. ok is
	// false when nothing hireable matches.
	FindHireable(query string) (templateID, name string, hireCost int, ok bool)
	// HirelingName returns a hireling template's display name and whether it
	// resolves to a hireable template (a mob carrying a `hireling:` block).
	HirelingName(templateID string) (string, bool)
	// Materialize spawns the owned hireling into roomID and stamps ownerID as its
	// owner (hireable-mobs.md §3.1, §9 login). Returns the live hireling's id.
	Materialize(ctx context.Context, ownerID, templateID string, roomID world.RoomID) (entities.EntityID, error)
	// Dematerialize removes a live hireling from the world (dismiss / §9 logout) —
	// the inverse of Materialize. Ownership (the save record) is NOT touched.
	// Reports whether a live hireling was found and removed.
	Dematerialize(ctx context.Context, id entities.EntityID) bool
}

// Hireling order stances (hireable-mobs.md §8) — the small stance set an owner
// sets with `order`. Stored on the live hireling (transient, reset to follow on
// re-materialize). The relocate/assist seams read them: follow trails + assists,
// stay holds + stands down, guard holds but assists combat in its own room.
const (
	HirelingStanceFollow = "follow"
	HirelingStanceStay   = "stay"
	HirelingStanceGuard  = "guard"
)

// hirelingOwner is the per-character hireling-ownership surface the connActor
// satisfies (hireable-mobs.md §2, §9). Durable ownership is backed by the player
// save; the live-materialized overlay (Track / Untrack / LiveHireling / stance) is
// transient session state. Handlers type-assert c.Actor to this.
type hirelingOwner interface {
	OwnedHirelingTemplates() []string
	HirelingCount() int
	AddHireling(templateID string)
	RemoveHireling(templateID string) bool
	TrackLiveHireling(id entities.EntityID, templateID string)
	UntrackLiveHireling(id entities.EntityID) (templateID string, ok bool)
	LiveHireling(templateID string) (entities.EntityID, bool)
	// SetHirelingStance sets a live hireling's order stance (hireable-mobs.md §8).
	// No-op if the id isn't a currently-live hireling.
	SetHirelingStance(id entities.EntityID, stance string)
}

// HireHandler implements `hire <name>` (hireable-mobs.md §3.1): hire a companion
// by name. Charges the hire cost up front, caps the number of simultaneous
// hirelings, and materializes the hireling into the owner's room.
func HireHandler(ctx context.Context, c *Context) error {
	owner, ok := c.Actor.(hirelingOwner)
	if !ok || c.Hirelings == nil {
		return c.Actor.Write(ctx, "You can't hire anyone right now.")
	}
	if len(c.Args) == 0 {
		return c.Actor.Write(ctx, "Hire whom?  (try: hire <name>)")
	}
	room := c.Actor.Room()
	if room == nil {
		return c.Actor.Write(ctx, "You're nowhere to hire help.")
	}
	// A non-positive cap means "no limit" (the int zero-value shouldn't silently
	// block every hire — mirrors the timeout<=0 = never convention).
	if c.HirelingCap > 0 && owner.HirelingCount() >= c.HirelingCap {
		return c.Actor.Write(ctx, "You already have all the help you can manage.")
	}
	query := strings.Join(c.Args, " ")
	templateID, name, cost, found := c.Hirelings.FindHireable(query)
	if !found {
		return c.Actor.Write(ctx, fmt.Sprintf("There is no %q to hire.", query))
	}
	// Charge the hire cost up front (hireable-mobs.md §3.1) — no creature until paid.
	holder, ok := c.Actor.(economy.Entity)
	if !ok || c.Currency == nil {
		return c.Actor.Write(ctx, "You can't pay for that right now.")
	}
	balance := c.Currency.Read(holder)
	if balance < cost {
		return c.Actor.Write(ctx, fmt.Sprintf("%s costs %d gold to hire; you only have %d.", capitalize(name), cost, balance))
	}
	left, okDebit := c.Currency.Debit(ctx, holder, cost, "hire:"+templateID)
	if !okDebit {
		return c.Actor.Write(ctx, fmt.Sprintf("%s costs %d gold to hire; you only have %d.", capitalize(name), cost, balance))
	}
	id, err := c.Hirelings.Materialize(ctx, c.Actor.PlayerID(), templateID, room.ID)
	if err != nil {
		// Refund: the contract never formed, so the gold should not be lost.
		c.Currency.AddGold(ctx, holder, cost, "hire-refund:"+templateID)
		return c.Actor.Write(ctx, "You couldn't hire them just now.")
	}
	owner.AddHireling(templateID)
	owner.TrackLiveHireling(id, templateID)
	return c.Actor.Write(ctx, fmt.Sprintf("You hire %s for %d gold. (You have %d gold left.)", name, cost, left))
}

// DismissHandler implements `dismiss <name>` (hireable-mobs.md §3.2): the owner
// ends a hire contract, removing the hireling from the world. No refund.
func DismissHandler(ctx context.Context, c *Context) error {
	owner, ok := c.Actor.(hirelingOwner)
	if !ok || c.Hirelings == nil {
		return c.Actor.Write(ctx, "You have no one to dismiss.")
	}
	if len(c.Args) == 0 {
		return c.Actor.Write(ctx, "Dismiss whom?  (try: dismiss <name>)")
	}
	query := strings.ToLower(strings.TrimSpace(strings.Join(c.Args, " ")))
	templateID, name, matched := c.matchOwnedHireling(owner, query)
	if !matched {
		return c.Actor.Write(ctx, fmt.Sprintf("You have no %q to dismiss.", query))
	}
	// Dematerialize the live creature if one is out, then drop the record.
	if id, live := owner.LiveHireling(templateID); live {
		c.Hirelings.Dematerialize(ctx, id)
		owner.UntrackLiveHireling(id)
	}
	owner.RemoveHireling(templateID)
	return c.Actor.Write(ctx, fmt.Sprintf("You dismiss %s.", name))
}

// HirelingsHandler implements `hirelings` (hireable-mobs.md §4): list the
// hirelings this character has under contract and whether each is present.
func HirelingsHandler(ctx context.Context, c *Context) error {
	owner, ok := c.Actor.(hirelingOwner)
	if !ok || c.Hirelings == nil {
		return c.Actor.Write(ctx, "You have no hirelings.")
	}
	owned := owner.OwnedHirelingTemplates()
	if len(owned) == 0 {
		return c.Actor.Write(ctx, "You have no hirelings.")
	}
	var b strings.Builder
	b.WriteString("Your hirelings:")
	for _, t := range owned {
		name, ok := c.Hirelings.HirelingName(t)
		if !ok {
			name = t // content drift: name the id rather than hide the contract
		}
		state := "away"
		if _, live := owner.LiveHireling(t); live {
			state = "with you"
		}
		b.WriteString(fmt.Sprintf("\n  %s — %s", name, state))
	}
	return c.Actor.Write(ctx, b.String())
}

// matchOwnedHireling resolves an owned hireling by (case-insensitive) name or id
// among the character's contracts, returning its template id and display name.
func (c *Context) matchOwnedHireling(owner hirelingOwner, query string) (templateID, name string, ok bool) {
	for _, t := range owner.OwnedHirelingTemplates() {
		n, _ := c.Hirelings.HirelingName(t)
		if templateMatches(n, t, query) {
			return t, n, true
		}
	}
	return "", "", false
}

// OrderHandler implements `order <hireling> <follow|stay|guard|attack [<target>]>`
// (hireable-mobs.md §8). Owner-only by construction — it resolves only among the
// caller's own contracts, so a non-owner can never name someone else's hireling.
// follow/stay/guard set a persistent stance the relocate (§5) and assist (§6.1)
// seams read; `attack <target>` engages a foe now without changing the stance.
// With exactly one live hireling the name may be omitted (`order guard`).
func OrderHandler(ctx context.Context, c *Context) error {
	owner, ok := c.Actor.(hirelingOwner)
	if !ok || c.Hirelings == nil {
		return c.Actor.Write(ctx, "You have no one to order.")
	}
	hirelingQuery, order, orderArgs, parsed := parseOrder(c.Args)
	if !parsed {
		return c.Actor.Write(ctx, "Order them to do what?  (follow, stay, guard, or attack <target>)")
	}
	id, name, found := c.resolveOrderTarget(owner, hirelingQuery)
	if !found {
		if hirelingQuery == "" {
			// Unnamed order: either nothing is present or it's ambiguous.
			if c.liveHirelingCount(owner) == 0 {
				return c.Actor.Write(ctx, "You have no hireling here to order.")
			}
			return c.Actor.Write(ctx, "Order which hireling?")
		}
		return c.Actor.Write(ctx, fmt.Sprintf("You have no %q to order.", hirelingQuery))
	}
	switch order {
	case HirelingStanceFollow, HirelingStanceStay, HirelingStanceGuard:
		owner.SetHirelingStance(id, order)
		return c.Actor.Write(ctx, orderStanceConfirm(name, order))
	case "attack":
		return c.orderAttack(ctx, id, name, orderArgs)
	}
	return c.Actor.Write(ctx, "Order them to do what?  (follow, stay, guard, or attack <target>)")
}

// parseOrder splits `order` args into the hireling name, the order keyword, and
// any trailing keyword args (the attack target). Two grammar rules disambiguate
// keywords that also appear in hireling or target names (e.g. a mob named "town
// guard", or `attack guard`):
//
//   - attack anchors its target: the FIRST "attack" token ends the hireling name
//     and everything after it is the target. So `order sellsword attack guard`
//     and `order town guard attack rat` both parse correctly.
//   - the no-arg stances (follow/stay/guard) are TERMINAL: they only count as the
//     command when they are the LAST token, so `order town guard follow` keeps
//     "town guard" as the name and "follow" as the command.
//
// An empty hireling name means "use my sole live hireling". Returns ok=false when
// no order keyword is present in a valid position.
func parseOrder(args []string) (hireling, order string, orderArgs []string, ok bool) {
	if len(args) == 0 {
		return "", "", nil, false
	}
	// attack: first occurrence anchors the target.
	for i, a := range args {
		if strings.EqualFold(a, "attack") {
			return strings.Join(args[:i], " "), "attack", args[i+1:], true
		}
	}
	// A terminal no-arg stance.
	last := strings.ToLower(args[len(args)-1])
	switch last {
	case HirelingStanceFollow, HirelingStanceStay, HirelingStanceGuard:
		return strings.Join(args[:len(args)-1], " "), last, nil, true
	}
	return "", "", nil, false
}

// resolveOrderTarget finds which live hireling an order applies to: by name when
// one is given, else the owner's sole live hireling (cap-1 convenience). Returns
// the live entity id + display name. Not ok when the name doesn't match a live
// hireling, or when an unnamed order is ambiguous (zero or several live).
func (c *Context) resolveOrderTarget(owner hirelingOwner, query string) (entities.EntityID, string, bool) {
	query = strings.ToLower(strings.TrimSpace(query))
	if query != "" {
		t, n, matched := c.matchOwnedHireling(owner, query)
		if !matched {
			return "", "", false
		}
		id, live := owner.LiveHireling(t)
		if !live {
			return "", "", false
		}
		return id, n, true
	}
	// Unnamed: apply to the sole live hireling, if exactly one.
	var (
		foundID   entities.EntityID
		foundName string
		count     int
	)
	for _, t := range owner.OwnedHirelingTemplates() {
		id, live := owner.LiveHireling(t)
		if !live {
			continue
		}
		count++
		foundID = id
		foundName, _ = c.Hirelings.HirelingName(t)
	}
	if count != 1 {
		return "", "", false
	}
	return foundID, foundName, true
}

// liveHirelingCount reports how many of the owner's contracts are currently
// materialized — used to tell "none present" from "ambiguous, name one".
func (c *Context) liveHirelingCount(owner hirelingOwner) int {
	n := 0
	for _, t := range owner.OwnedHirelingTemplates() {
		if _, live := owner.LiveHireling(t); live {
			n++
		}
	}
	return n
}

// orderAttack engages a hireling on a foe in the owner's room (hireable-mobs.md
// §8 attack). The hireling must be co-located with the owner to take the order
// (v1). It does not change the hireling's persistent stance — once engaged, the
// combat round loop and assist logic take over.
func (c *Context) orderAttack(ctx context.Context, hid entities.EntityID, hName string, args []string) error {
	if c.Combat == nil {
		return c.Actor.Write(ctx, "There's nothing to fight right now.")
	}
	target := strings.TrimSpace(strings.Join(args, " "))
	if target == "" {
		return c.Actor.Write(ctx, fmt.Sprintf("Order %s to attack what?", hName))
	}
	room := c.Actor.Room()
	if room == nil {
		return c.Actor.Write(ctx, "You see nothing to fight here.")
	}
	if c.Placement != nil {
		if r, ok := c.Placement.RoomOf(hid); !ok || r != room.ID {
			return c.Actor.Write(ctx, fmt.Sprintf("%s isn't here to take that order.", capitalize(hName)))
		}
	}
	foe, foeName, found := findCombatantInRoom(c, room.ID, target)
	if !found {
		return c.Actor.Write(ctx, "You don't see them here.")
	}
	hCID := combat.NewMobCombatantID(string(hid))
	if foe.CombatantID() == hCID {
		return c.Actor.Write(ctx, fmt.Sprintf("%s can't attack itself.", capitalize(hName)))
	}
	// Don't turn your own hireling on you.
	if self, ok := c.Actor.(combat.Combatant); ok && foe.CombatantID() == self.CombatantID() {
		return c.Actor.Write(ctx, fmt.Sprintf("%s won't attack you.", capitalize(hName)))
	}
	if _, ok := c.Combat.EngageWithReason(ctx, hCID, foe.CombatantID(), room.ID); !ok {
		return c.Actor.Write(ctx, fmt.Sprintf("%s can't attack %s right now.", capitalize(hName), foeName))
	}
	return c.Actor.Write(ctx, fmt.Sprintf("%s moves to attack %s!", capitalize(hName), foeName))
}

// orderStanceConfirm is the owner-facing line for a stance order.
func orderStanceConfirm(name, stance string) string {
	switch stance {
	case HirelingStanceStay:
		return fmt.Sprintf("%s will hold this position.", capitalize(name))
	case HirelingStanceGuard:
		return fmt.Sprintf("%s takes up guard here.", capitalize(name))
	default: // follow
		return fmt.Sprintf("%s will follow you.", capitalize(name))
	}
}
