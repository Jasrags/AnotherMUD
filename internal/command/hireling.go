package command

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/economy"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// This file holds the hireling lifecycle verbs (hireable-mobs.md §3): `hire` a
// companion, `dismiss` it, and `hirelings` to list them. It mirrors the mount
// surface (mount.go) — a hireling fights where a mount carries. Acquisition is AT
// a recruiter (§3.1): `hire` only works in a room with a recruiter NPC, and
// resolves the request against the hirelings that recruiter offers.

// HireableOffer is one hireling a recruiter will hire out (hireable-mobs.md §3.1):
// the hireling template id, its display name, and the up-front hire cost.
type HireableOffer struct {
	TemplateID string
	Name       string
	HireCost   int
}

// HirelingService is the runtime hireling lifecycle the command layer depends on
// (hireable-mobs.md §2, §3) — implemented at the composition root over the mob
// spawn pipeline. The durable ownership records live on the player save
// (hirelingOwner).
type HirelingService interface {
	// RecruiterOffers returns the hirelings offered by the given recruiter mob
	// template ids (those present in the room), deduped and stably ordered. Empty
	// when none of the ids is a recruiter. This is the catalog `hire` resolves a
	// request against (hireable-mobs.md §3.1).
	RecruiterOffers(recruiterTemplateIDs []string) []HireableOffer
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

// LiveHirelingRef names one materialized hireling: its live entity id and the
// template it was hired from. The roster of these (LiveHirelings) is the stable,
// indexable handle the targeting verbs use when an owner holds several hirelings
// (hireable-mobs.md §3.3) — distinct same-template duplicates that share a name.
type LiveHirelingRef struct {
	ID         entities.EntityID
	TemplateID string
}

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
	// LiveHirelings returns every materialized hireling in a STABLE order (by
	// entity id), so a 1-based index over it is a durable targeting handle across
	// commands within a session — the way to address same-template duplicates.
	LiveHirelings() []LiveHirelingRef
	// SetHirelingStance sets a live hireling's order stance (hireable-mobs.md §8).
	// No-op if the id isn't a currently-live hireling.
	SetHirelingStance(id entities.EntityID, stance string)
}

// HireHandler implements `hire [<name>]` (hireable-mobs.md §3.1): hire a companion
// from a **recruiter** present in the room. With no argument it browses the
// recruiter's catalog; with a name it hires that companion — charging the hire
// cost up front, capping the count, and materializing the hireling into the room.
func HireHandler(ctx context.Context, c *Context) error {
	owner, ok := c.Actor.(hirelingOwner)
	if !ok || c.Hirelings == nil {
		return c.Actor.Write(ctx, "You can't hire anyone right now.")
	}
	room := c.Actor.Room()
	if room == nil {
		return c.Actor.Write(ctx, "You're nowhere to hire help.")
	}
	// Hiring happens AT a recruiter (§3.1): only what a recruiter in this room
	// offers can be hired. No recruiter present → no one to hire from.
	offers := c.recruiterOffersHere(room.ID)
	if len(offers) == 0 {
		return c.Actor.Write(ctx, "There's no one here to hire from.")
	}
	// Bare `hire` browses the catalog.
	if len(c.Args) == 0 {
		return c.Actor.Write(ctx, renderHireOffers(offers))
	}
	// A non-positive cap means "no limit" (the int zero-value shouldn't silently
	// block every hire — mirrors the timeout<=0 = never convention).
	if c.HirelingCap > 0 && owner.HirelingCount() >= c.HirelingCap {
		return c.Actor.Write(ctx, "You already have all the help you can manage.")
	}
	query := strings.Join(c.Args, " ")
	offer, found := matchHireableOffer(offers, query)
	if !found {
		return c.Actor.Write(ctx, fmt.Sprintf("There's no %q for hire here.", query))
	}
	// Charge the hire cost up front (hireable-mobs.md §3.1) — no creature until paid.
	holder, ok := c.Actor.(economy.Entity)
	if !ok || c.Currency == nil {
		return c.Actor.Write(ctx, "You can't pay for that right now.")
	}
	balance := c.Currency.Read(holder)
	if balance < offer.HireCost {
		return c.Actor.Write(ctx, fmt.Sprintf("%s costs %d gold to hire; you only have %d.", capitalize(offer.Name), offer.HireCost, balance))
	}
	left, okDebit := c.Currency.Debit(ctx, holder, offer.HireCost, "hire:"+offer.TemplateID)
	if !okDebit {
		return c.Actor.Write(ctx, fmt.Sprintf("%s costs %d gold to hire; you only have %d.", capitalize(offer.Name), offer.HireCost, balance))
	}
	id, err := c.Hirelings.Materialize(ctx, c.Actor.PlayerID(), offer.TemplateID, room.ID)
	if err != nil {
		// Refund: the contract never formed, so the gold should not be lost.
		c.Currency.AddGold(ctx, holder, offer.HireCost, "hire-refund:"+offer.TemplateID)
		return c.Actor.Write(ctx, "You couldn't hire them just now.")
	}
	owner.AddHireling(offer.TemplateID)
	owner.TrackLiveHireling(id, offer.TemplateID)
	return c.Actor.Write(ctx, fmt.Sprintf("You hire %s for %d gold. (You have %d gold left.)", offer.Name, offer.HireCost, left))
}

// recruiterOffersHere returns the hirelings any recruiter NPC in the room hires
// out (hireable-mobs.md §3.1). It enumerates the room's mobs, gathers their
// template ids, and asks the service which are recruiters and what they offer.
// Empty when no recruiter is present or the room can't be enumerated (tests).
func (c *Context) recruiterOffersHere(roomID world.RoomID) []HireableOffer {
	if c.Placement == nil || c.Items == nil {
		return nil
	}
	ids := c.Placement.InRoom(roomID)
	tmpls := make([]string, 0, len(ids))
	for _, id := range ids {
		e, ok := c.Items.GetByID(id)
		if !ok {
			continue
		}
		if mi, ok := e.(*entities.MobInstance); ok {
			tmpls = append(tmpls, string(mi.TemplateID()))
		}
	}
	return c.Hirelings.RecruiterOffers(tmpls)
}

// matchOffer resolves a hire request to one of the recruiter's offers by name or
// id (case-insensitive), the same keyword match the other targeting verbs use.
func matchHireableOffer(offers []HireableOffer, query string) (HireableOffer, bool) {
	q := strings.ToLower(strings.TrimSpace(query))
	for _, o := range offers {
		if templateMatches(o.Name, o.TemplateID, q) {
			return o, true
		}
	}
	return HireableOffer{}, false
}

// renderOffers lists a recruiter's catalog for a bare `hire` (hireable-mobs.md §3.1).
func renderHireOffers(offers []HireableOffer) string {
	var b strings.Builder
	b.WriteString("Available for hire here:")
	for _, o := range offers {
		fmt.Fprintf(&b, "\n  %s — %d gold", o.Name, o.HireCost)
	}
	return b.String()
}

// DismissHandler implements `dismiss <name|number>` (hireable-mobs.md §3.2): the
// owner ends a hire contract, removing the hireling from the world. No refund.
// With several hirelings, a bare name that matches more than one is ambiguous —
// dismiss it by its roster number (see `hirelings`).
func DismissHandler(ctx context.Context, c *Context) error {
	owner, ok := c.Actor.(hirelingOwner)
	if !ok || c.Hirelings == nil {
		return c.Actor.Write(ctx, "You have no one to dismiss.")
	}
	if len(c.Args) == 0 {
		return c.Actor.Write(ctx, "Dismiss whom?  (try: dismiss <name|number>)")
	}
	query := strings.ToLower(strings.TrimSpace(strings.Join(c.Args, " ")))
	matches := c.resolveHireling(owner, query)
	switch len(matches) {
	case 1:
		m := matches[0]
		// Untrack first: if the hireling died in the window between resolving and
		// here (combat goroutine ran OnHirelingDeath), it's already gone — don't
		// print a dismiss confirmation for a contract that no longer exists.
		tmpl, ok := owner.UntrackLiveHireling(m.id)
		if !ok {
			return c.Actor.Write(ctx, fmt.Sprintf("%s is already gone.", capitalize(m.name)))
		}
		c.Hirelings.Dematerialize(ctx, m.id)
		owner.RemoveHireling(tmpl)
		return c.Actor.Write(ctx, fmt.Sprintf("You dismiss %s.", m.name))
	case 0:
		// No LIVE match. A stranded contract (owned but never materialized — content
		// drift) still has a record to drop, so the owner is never stuck with a ghost.
		if tmpl, name, ok := c.matchOwnedHireling(owner, query); ok {
			owner.RemoveHireling(tmpl)
			return c.Actor.Write(ctx, fmt.Sprintf("You dismiss %s.", name))
		}
		return c.Actor.Write(ctx, fmt.Sprintf("You have no %q to dismiss.", query))
	default:
		return c.Actor.Write(ctx, fmt.Sprintf("You have more than one %q — dismiss it by number (see `hirelings`).", query))
	}
}

// HirelingsHandler implements `hirelings` (hireable-mobs.md §4): list the
// hirelings this character has, NUMBERED — the number is the stable targeting
// handle (`dismiss 2`, `order 2 guard`) that disambiguates same-template
// duplicates. Each line shows whether the hireling is here with you or holding a
// position elsewhere (a stay/guard order, §8).
func HirelingsHandler(ctx context.Context, c *Context) error {
	owner, ok := c.Actor.(hirelingOwner)
	if !ok || c.Hirelings == nil {
		return c.Actor.Write(ctx, "You have no hirelings.")
	}
	live := owner.LiveHirelings()
	if len(live) == 0 {
		return c.Actor.Write(ctx, "You have no hirelings.")
	}
	myRoom := c.Actor.Room()
	var b strings.Builder
	b.WriteString("Your hirelings:")
	for i, ref := range live {
		where := "with you"
		if c.Placement != nil && myRoom != nil {
			if r, ok := c.Placement.RoomOf(ref.ID); ok && r != myRoom.ID {
				where = "elsewhere"
			}
		}
		fmt.Fprintf(&b, "\n  %d) %s — %s", i+1, c.hirelingName(ref.TemplateID), where)
	}
	// Surface any contract with no creature in the world (content drift — a template
	// that left content can't re-materialize) so the owner can `dismiss` it by name.
	if absent := owner.HirelingCount() - len(live); absent > 0 {
		fmt.Fprintf(&b, "\n  (%d more under contract but not present)", absent)
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
// caller's own live hirelings, so a non-owner can never name someone else's.
// follow/stay/guard set a persistent stance the relocate (§5) and assist (§6.1)
// seams read; `attack <target>` engages a foe now without changing the stance.
// The hireling may be named, given as a roster number, or "all" (every hireling);
// with exactly one hireling the target may be omitted (`order guard`).
func OrderHandler(ctx context.Context, c *Context) error {
	owner, ok := c.Actor.(hirelingOwner)
	if !ok || c.Hirelings == nil {
		return c.Actor.Write(ctx, "You have no one to order.")
	}
	hirelingQuery, order, orderArgs, parsed := parseOrder(c.Args)
	if !parsed {
		return c.Actor.Write(ctx, "Order them to do what?  (follow, stay, guard, or attack <target>)")
	}
	// Lowercase the name token so name-matching is case-insensitive (templateMatches
	// expects a lowercased query); the number / "all" / "" forms are unaffected.
	hirelingQuery = strings.ToLower(strings.TrimSpace(hirelingQuery))
	targets, errMsg := c.orderTargets(owner, hirelingQuery)
	if errMsg != "" {
		return c.Actor.Write(ctx, errMsg)
	}
	switch order {
	case HirelingStanceFollow, HirelingStanceStay, HirelingStanceGuard:
		for _, t := range targets {
			owner.SetHirelingStance(t.id, order)
		}
		if len(targets) == 1 {
			return c.Actor.Write(ctx, orderStanceConfirm(targets[0].name, order))
		}
		return c.Actor.Write(ctx, orderStanceConfirmAll(len(targets), order))
	case "attack":
		if c.Combat == nil {
			return c.Actor.Write(ctx, "There's nothing to fight right now.")
		}
		// Each hireling engages and reports its own line (so `order all attack rat`
		// sics the whole band on the foe).
		for _, t := range targets {
			_ = c.orderAttack(ctx, t.id, t.name, orderArgs)
		}
		return nil
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

// hirelingMatch names a resolved live hireling: its entity id + display name.
type hirelingMatch struct {
	id   entities.EntityID
	name string
}

// hirelingName resolves a hireling template's display name, falling back to the
// template id on content drift (a contract whose template left content).
func (c *Context) hirelingName(templateID string) string {
	if n, ok := c.Hirelings.HirelingName(templateID); ok {
		return n
	}
	return templateID
}

// resolveHireling resolves a target token to live hireling(s) (hireable-mobs.md
// §3.3). A bare NUMBER selects the N-th in the stable roster (1-based, the
// `hirelings` index) — the unambiguous handle for same-template duplicates.
// Otherwise the token is a name/keyword and EVERY live hireling whose template
// matches is returned; callers read len 0 = none, 1 = the target, >1 = ambiguous.
func (c *Context) resolveHireling(owner hirelingOwner, query string) []hirelingMatch {
	live := owner.LiveHirelings()
	if n, err := strconv.Atoi(query); err == nil {
		if n >= 1 && n <= len(live) {
			ref := live[n-1]
			return []hirelingMatch{{id: ref.ID, name: c.hirelingName(ref.TemplateID)}}
		}
		return nil
	}
	var out []hirelingMatch
	for _, ref := range live {
		name := c.hirelingName(ref.TemplateID)
		if templateMatches(name, ref.TemplateID, query) {
			out = append(out, hirelingMatch{id: ref.ID, name: name})
		}
	}
	return out
}

// orderTargets resolves which hireling(s) an order applies to (hireable-mobs.md
// §8), returning the targets and a player-facing error (empty on success):
//   - "all" → every live hireling (band order);
//   - "" (omitted) → the sole live hireling, else "which one?";
//   - a number/name → resolveHireling (a name matching several is ambiguous).
func (c *Context) orderTargets(owner hirelingOwner, query string) ([]hirelingMatch, string) {
	live := owner.LiveHirelings()
	if len(live) == 0 {
		return nil, "You have no hireling here to order."
	}
	if strings.EqualFold(query, "all") {
		out := make([]hirelingMatch, 0, len(live))
		for _, ref := range live {
			out = append(out, hirelingMatch{id: ref.ID, name: c.hirelingName(ref.TemplateID)})
		}
		return out, ""
	}
	if query == "" {
		if len(live) == 1 {
			return []hirelingMatch{{id: live[0].ID, name: c.hirelingName(live[0].TemplateID)}}, ""
		}
		return nil, "Order which hireling?  (by name or number — see `hirelings`)"
	}
	switch matches := c.resolveHireling(owner, query); len(matches) {
	case 0:
		return nil, fmt.Sprintf("You have no %q to order.", query)
	case 1:
		return matches, ""
	default:
		return nil, fmt.Sprintf("You have more than one %q — order it by number (see `hirelings`).", query)
	}
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

// orderStanceConfirm is the owner-facing line for a single-hireling stance order.
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

// orderStanceConfirmAll is the owner-facing line for a band stance order (`order
// all ...`), summarizing the n hirelings affected.
func orderStanceConfirmAll(n int, stance string) string {
	switch stance {
	case HirelingStanceStay:
		return fmt.Sprintf("Your %d hirelings hold this position.", n)
	case HirelingStanceGuard:
		return fmt.Sprintf("Your %d hirelings take up guard here.", n)
	default: // follow
		return fmt.Sprintf("Your %d hirelings will follow you.", n)
	}
}
