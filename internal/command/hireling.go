package command

import (
	"context"
	"fmt"
	"strings"

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

// hirelingOwner is the per-character hireling-ownership surface the connActor
// satisfies (hireable-mobs.md §2, §9). Durable ownership is backed by the player
// save; the live-materialized overlay (Track / Untrack / LiveHireling) is
// transient session state. Handlers type-assert c.Actor to this.
type hirelingOwner interface {
	OwnedHirelingTemplates() []string
	HirelingCount() int
	AddHireling(templateID string)
	RemoveHireling(templateID string) bool
	TrackLiveHireling(id entities.EntityID, templateID string)
	UntrackLiveHireling(id entities.EntityID) (templateID string, ok bool)
	LiveHireling(templateID string) (entities.EntityID, bool)
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
