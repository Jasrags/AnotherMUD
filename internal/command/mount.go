package command

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/economy"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/eventbus"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// mountRider is the per-character ride-relationship surface the connActor
// satisfies (mounts.md §4.3). Transient — the live ride is never persisted.
// The mount/dismount verbs type-assert c.Actor to this.
type mountRider interface {
	MountID() entities.EntityID
	SetMountID(entities.EntityID)
}

// This file implements the mount acquisition + stabling verbs (mounts.md §3,
// §9): listing owned mounts, buying one from a stablemaster, and stabling /
// retrieving a mount at a stable access point. The ride verbs (mount/dismount)
// and mounted travel live in their own slices. A stablemaster is a mob that is
// BOTH the mount vendor and the stable access point — it carries the `stable`
// tag (the access-point marker, mirroring economy.TagShop) and a `stable`
// property block listing the mounts it sells.

const (
	// stableTag marks a mob as a stable access point + mount vendor
	// (mounts.md §3.1, §3.2). Found in the room the same way the shop tag is.
	stableTag = "stable"
	// stableProp is the mob property key carrying the stable's sell block —
	// a `sells:` map of mount-template-id → price. Mirrors shopProp.
	stableProp = "stable"
)

// MountService is the runtime mount lifecycle the command layer depends on
// (mounts.md §2, §3) — implemented at the composition root over the mob spawn
// pipeline. It materializes an owned mount into the world and removes it again;
// the durable ownership records live on the player save (mountOwner).
type MountService interface {
	// MountName returns the display name of a mount template and whether it
	// resolves to a real mount (a mob template carrying a mount block). A
	// non-mount or unknown id returns ("", false).
	MountName(templateID string) (string, bool)
	// Materialize spawns the owned mount into roomID and stamps ownerID as its
	// owner (mounts.md §3.1, §5.5) — the inverse of stabling. Returns the live
	// mount's entity id.
	Materialize(ctx context.Context, ownerID, templateID string, roomID world.RoomID) (entities.EntityID, error)
	// Dematerialize removes a live mount from the world (mounts.md §3.2 stabling
	// / §9) — the inverse of Materialize. Ownership (the save record) is NOT
	// touched; only the live creature leaves the world. Reports whether a live
	// mount was found and removed.
	Dematerialize(ctx context.Context, mountID entities.EntityID) bool
}

// mountOwner is the per-character mount-ownership surface the connActor
// satisfies (mounts.md §2.2, §10). Durable ownership (OwnedMountTemplates /
// Add / Remove) is backed by the player save; the live-materialized set
// (Track / Untrack / LiveMountTemplates) is transient session state tracking
// which owned mounts currently have a creature in the world. Handlers
// type-assert c.Actor to this rather than widening the Actor interface.
type mountOwner interface {
	OwnedMountTemplates() []string
	AddMount(templateID string)
	RemoveMount(templateID string) bool
	TrackLiveMount(id entities.EntityID, templateID string)
	UntrackLiveMount(id entities.EntityID) (templateID string, ok bool)
	LiveMountTemplates() []string
}

// MountsHandler implements `mounts` (mounts.md §9): list the mounts this
// character owns and whether each is currently stabled or out in the world.
func MountsHandler(ctx context.Context, c *Context) error {
	owner, ok := c.Actor.(mountOwner)
	if !ok || c.Mounts == nil {
		return c.Actor.Write(ctx, "You own no mounts.")
	}
	owned := owner.OwnedMountTemplates()
	if len(owned) == 0 {
		return c.Actor.Write(ctx, "You own no mounts.")
	}
	// Owned total per template, and stabled = owned minus the live
	// (materialized) multiset. Compute the owned multiset ONCE and reuse it.
	ownedSet := multiset(owned)
	stabled := multiset(owned)
	for _, t := range owner.LiveMountTemplates() {
		if stabled[t] > 0 {
			stabled[t]--
		}
	}
	var b strings.Builder
	b.WriteString("You own:")
	for _, t := range sortedKeys(ownedSet) {
		name, ok := c.Mounts.MountName(t)
		if !ok {
			name = t // content drift: name the id rather than hide the asset
		}
		total := ownedSet[t]
		stab := stabled[t]
		for i := range total {
			state := "out in the world"
			if i < stab {
				state = "stabled"
			}
			b.WriteString(fmt.Sprintf("\n  %s — %s", name, state))
		}
	}
	return c.Actor.Write(ctx, b.String())
}

// BuyMountHandler implements `buymount <name>` (mounts.md §3.1): purchase a
// mount from the stablemaster in the room. A successful buy debits gold and
// adds a stabled ownership record — the mount is retrieved with `unstable`.
func BuyMountHandler(ctx context.Context, c *Context) error {
	owner, st, ok := stableContext(ctx, c)
	if !ok {
		return nil
	}
	if len(c.Args) == 0 {
		return c.Actor.Write(ctx, "Buy which mount?")
	}
	query := strings.Join(c.Args, " ")
	offers := stableOffers(c, st)
	if len(offers) == 0 {
		return c.Actor.Write(ctx, "There is nothing for sale here.")
	}
	offer, matched := matchOffer(c, offers, query)
	if !matched {
		return c.Actor.Write(ctx, "That mount isn't for sale here.")
	}
	name, _ := c.Mounts.MountName(offer.templateID)
	holder, ok := c.Actor.(economy.Entity)
	if !ok {
		return c.Actor.Write(ctx, "You can't pay for that.")
	}
	balance := holder.Gold()
	if c.Currency != nil {
		balance = c.Currency.Read(holder)
	}
	if balance < offer.price {
		return c.Actor.Write(ctx, fmt.Sprintf("%s costs %s; you only have %s.", capitalize(name), c.Money.Format(offer.price), c.Money.Format(balance)))
	}
	if c.Currency == nil {
		return c.Actor.Write(ctx, "You can't pay for that right now.")
	}
	left, okDebit := c.Currency.Debit(ctx, holder, offer.price, "buymount:"+offer.templateID)
	if !okDebit {
		return c.Actor.Write(ctx, fmt.Sprintf("%s costs %s; you only have %s.", capitalize(name), c.Money.Format(offer.price), c.Money.Format(balance)))
	}
	owner.AddMount(offer.templateID)
	return c.Actor.Write(ctx, fmt.Sprintf("You buy %s for %s; it is stabled here. (You have %s left.)", name, c.Money.Format(offer.price), c.Money.Format(left)))
}

// UnstableHandler implements `unstable <name>` (mounts.md §3.2): retrieve a
// stabled owned mount into the room, materializing the live creature. Requires
// a stable access point in the room.
func UnstableHandler(ctx context.Context, c *Context) error {
	owner, _, ok := stableContext(ctx, c)
	if !ok {
		return nil
	}
	if len(c.Args) == 0 {
		return c.Actor.Write(ctx, "Retrieve which mount?")
	}
	room := c.Actor.Room()
	if room == nil {
		return c.Actor.Write(ctx, "There is no stable here.")
	}
	query := strings.Join(c.Args, " ")
	// Stabled = owned minus live. Pick a stabled template matching the query.
	stabled := multiset(owner.OwnedMountTemplates())
	for _, t := range owner.LiveMountTemplates() {
		if stabled[t] > 0 {
			stabled[t]--
		}
	}
	templateID, ok := matchOwnedTemplate(c, stabled, query)
	if !ok {
		return c.Actor.Write(ctx, "You have no such mount stabled here.")
	}
	id, err := c.Mounts.Materialize(ctx, ownerID(c), templateID, room.ID)
	if err != nil {
		return c.Actor.Write(ctx, "The stablemaster can't find that mount right now.")
	}
	owner.TrackLiveMount(id, templateID)
	name, _ := c.Mounts.MountName(templateID)
	if c.Broadcaster != nil {
		c.Broadcaster.SendToRoom(ctx, room.ID, fmt.Sprintf("The stablemaster brings out %s for %s.", name, c.Actor.Name()), c.Actor.PlayerID())
	}
	return c.Actor.Write(ctx, fmt.Sprintf("The stablemaster brings out %s.", name))
}

// StableHandler implements `stable [<name>]` (mounts.md §3.2, §9): put an owned
// mount that is here back into the stable, dematerializing the live creature.
// With no argument, stables every owned mount of yours in the room.
func StableHandler(ctx context.Context, c *Context) error {
	owner, _, ok := stableContext(ctx, c)
	if !ok {
		return nil
	}
	room := c.Actor.Room()
	if room == nil || c.Placement == nil || c.Items == nil {
		return c.Actor.Write(ctx, "There is no stable here.")
	}
	query := strings.TrimSpace(strings.Join(c.Args, " "))
	mine := myLiveMountsInRoom(c, owner, room.ID)
	if len(mine) == 0 {
		return c.Actor.Write(ctx, "You have no mount here to stable.")
	}
	rider, _ := c.Actor.(mountRider)
	stabledAny := false
	ridingHere := false
	for _, m := range mine {
		if query != "" && !mountMatches(c, m, query) {
			continue
		}
		// A mount you're riding can't be led into the stable — dismount first
		// (mounts.md §4.2). Skip it so `stable` (all) stables the rest.
		if rider != nil && rider.MountID() == m.ID() {
			ridingHere = true
			continue
		}
		if c.Mounts.Dematerialize(ctx, m.ID()) {
			owner.UntrackLiveMount(m.ID())
			stabledAny = true
			if c.Broadcaster != nil {
				c.Broadcaster.SendToRoom(ctx, room.ID, fmt.Sprintf("%s leads %s into the stable.", c.Actor.Name(), m.Name()), c.Actor.PlayerID())
			}
			_ = c.Actor.Write(ctx, fmt.Sprintf("You stable %s.", m.Name()))
		}
	}
	if !stabledAny {
		if ridingHere {
			return c.Actor.Write(ctx, "You can't stable a mount you're riding — dismount first.")
		}
		return c.Actor.Write(ctx, "You have no such mount here to stable.")
	}
	return nil
}

// MountHandler implements `mount <name>` (mounts.md §4.1): bind the ride
// relationship to an owned mount sharing the room. Refused for a non-mount, an
// unowned mount, an absent target, or an already-mounted rider — each with a
// clear message — and gated by a cancellable mount.before event.
func MountHandler(ctx context.Context, c *Context) error {
	rider, ok := c.Actor.(mountRider)
	if !ok {
		return c.Actor.Write(ctx, "You can't ride anything.")
	}
	if rider.MountID() != "" {
		return c.Actor.Write(ctx, "You are already mounted. Dismount first.")
	}
	if len(c.Args) == 0 {
		return c.Actor.Write(ctx, "Mount what?")
	}
	room := c.Actor.Room()
	if room == nil || c.Placement == nil || c.Items == nil {
		return c.Actor.Write(ctx, "You don't see that here.")
	}
	target, why := findMountTargetInRoom(c, room.ID, strings.Join(c.Args, " "))
	if target == nil {
		return c.Actor.Write(ctx, why)
	}
	// Cancellable pre-event (mounts.md §4.1): content may gate or charge
	// mounting. A veto aborts with no state change.
	if c.Bus != nil {
		pre := eventbus.NewMountBefore(ownerID(c), string(target.ID()), room.ID)
		if c.Bus.PublishCancellable(ctx, pre) {
			return c.Actor.Write(ctx, "You can't mount right now.")
		}
	}
	rider.SetMountID(target.ID())
	if c.Broadcaster != nil {
		c.Broadcaster.SendToRoom(ctx, room.ID, fmt.Sprintf("%s climbs onto %s.", c.Actor.Name(), target.Name()), c.Actor.PlayerID())
	}
	return c.Actor.Write(ctx, fmt.Sprintf("You climb onto %s.", target.Name()))
}

// DismountHandler implements `dismount` (mounts.md §4.2): end the ride
// relationship, leaving rider and mount in the current room. Always available
// to a conscious rider (the never-strand guarantee, §6).
func DismountHandler(ctx context.Context, c *Context) error {
	rider, ok := c.Actor.(mountRider)
	if !ok || rider.MountID() == "" {
		return c.Actor.Write(ctx, "You aren't mounted.")
	}
	name := "your mount"
	if m, ok := riddenMount(c, rider); ok {
		name = m.Name()
	}
	rider.SetMountID("")
	room := c.Actor.Room()
	if c.Broadcaster != nil && room != nil {
		c.Broadcaster.SendToRoom(ctx, room.ID, fmt.Sprintf("%s climbs down from %s.", c.Actor.Name(), name), c.Actor.PlayerID())
	}
	return c.Actor.Write(ctx, fmt.Sprintf("You climb down from %s.", name))
}

// PropRoomMountImpassable is the room property flag marking a destination no
// mount can enter (mounts.md §5.3) — a cramped interior, a sheer stair. A
// mounted step into such a room is refused; the rider dismounts and walks. This
// is the broad room-level gate; a mount type's own Impassable list is the
// narrower per-mount gate (CannotEnterTerrain).
const PropRoomMountImpassable = "mount_impassable"

// mountImpassableText / mountBlownText are the player-facing refusals for the
// two mounted-travel blocks (§5.3, §5.4). Both leave dismount-and-walk open
// (the never-strand rule, §6).
const (
	mountImpassableText = "Your mount can't go that way. (Dismount to continue on foot.)"
	mountBlownText      = "Your mount is blown and won't go on. (Dismount to walk, or wait for it to recover.)"
)

// mountedSteed returns the live mount the actor is currently riding, or nil
// when on foot. Clears a stale ride pointer as a side effect (lazy
// never-strand). The movement gate calls this to decide who pays for the step.
func mountedSteed(c *Context) *entities.MobInstance {
	rider, ok := c.Actor.(mountRider)
	if !ok {
		return nil
	}
	m, ok := riddenMount(c, rider)
	if !ok {
		return nil
	}
	return m
}

// mountBlockedBy reports whether a mount is barred from entering dst (§5.3):
// either the room flags itself mount-impassable for all mounts, or this mount
// type's own impassable-terrain list names dst's terrain.
func mountBlockedBy(c *Context, dst *world.Room, steed *entities.MobInstance) bool {
	if dst == nil {
		return false
	}
	if blocked, _ := dst.PropertyBool(PropRoomMountImpassable); blocked {
		return true
	}
	return steed.CannotEnterTerrain(dst.Terrain)
}

// spendMountTravel charges a mounted step against the mount's travel pool
// (mounts.md §5.1, §5.4): a non-positive cost moves free; a cost the mount can
// afford is charged; otherwise the step is refused (the mount is blown — or the
// terrain is permanently beyond its ceiling, in which case the rider dismounts
// and walks it). Unlike the on-foot spendMovement gate, there is NO
// "cost > max ⇒ free" branch: a mount always has a pool (the loader enforces
// travel_max > 0), so a step it can never afford is a refusal, not a free pass —
// that surfaces a content misconfiguration instead of silently galloping
// through impassably-costly terrain. Returns (allowed, charged).
func spendMountTravel(steed *entities.MobInstance, cost int) (allowed, charged bool) {
	if cost <= 0 {
		return true, false
	}
	if steed.TrySpendTravel(cost) {
		return true, true
	}
	return false, false
}

// riddenMount resolves the live mount a rider is on, or clears a stale ride
// pointer and reports false (lazy never-strand, mounts.md §6): if the mount has
// left the world (died, was removed), the rider is simply on foot again. Safe
// to call from any mount-aware handler before relying on the ride.
func riddenMount(c *Context, rider mountRider) (*entities.MobInstance, bool) {
	id := rider.MountID()
	if id == "" || c.Items == nil {
		return nil, false
	}
	e, ok := c.Items.GetByID(id)
	if !ok {
		rider.SetMountID("") // the mount is gone — set the rider back on foot
		return nil, false
	}
	m, ok := e.(*entities.MobInstance)
	if !ok || !m.IsMount() {
		rider.SetMountID("")
		return nil, false
	}
	return m, true
}

// findMountTargetInRoom resolves a `mount <query>` target, returning the
// rideable+owned mount or a nil mount plus a precise refusal message. It
// prefers the most specific failure: an unowned matching mount over a
// non-mount match over no match at all.
func findMountTargetInRoom(c *Context, roomID world.RoomID, query string) (*entities.MobInstance, string) {
	if c.Placement == nil || c.Items == nil {
		return nil, "You don't see that mount here."
	}
	q := strings.ToLower(strings.TrimSpace(query))
	var unowned, notMount bool
	for _, id := range c.Placement.InRoom(roomID) {
		e, ok := c.Items.GetByID(id)
		if !ok {
			continue
		}
		m, ok := e.(*entities.MobInstance)
		if !ok || !templateMatches(m.Name(), string(m.TemplateID()), q) {
			continue
		}
		if !m.IsMount() {
			notMount = true
			continue
		}
		if !m.IsOwnedBy(ownerID(c)) {
			unowned = true
			continue
		}
		return m, ""
	}
	switch {
	case unowned:
		return nil, "That isn't your mount."
	case notMount:
		return nil, "You can't ride that."
	default:
		return nil, "You don't see that mount here."
	}
}

// --- helpers ---

// stableContext resolves the common prelude for the stabling verbs: the actor
// must be a mount owner and a stable access point must share their room. Writes
// the player-facing failure and returns ok=false on any miss.
func stableContext(ctx context.Context, c *Context) (mountOwner, *entities.MobInstance, bool) {
	owner, ok := c.Actor.(mountOwner)
	if !ok || c.Mounts == nil {
		_ = c.Actor.Write(ctx, "There is no stable here.")
		return nil, nil, false
	}
	room := c.Actor.Room()
	if room == nil {
		_ = c.Actor.Write(ctx, "There is no stable here.")
		return nil, nil, false
	}
	st := findStableInRoom(c, room.ID)
	if st == nil {
		_ = c.Actor.Write(ctx, "There is no stable here.")
		return nil, nil, false
	}
	return owner, st, true
}

// findStableInRoom returns the first stable-access-point mob in the room (the
// stablemaster), or nil. Mirrors findShopInRoom.
func findStableInRoom(c *Context, roomID world.RoomID) *entities.MobInstance {
	if c.Items == nil || c.Placement == nil {
		return nil
	}
	for _, id := range c.Placement.InRoom(roomID) {
		e, ok := c.Items.GetByID(id)
		if !ok {
			continue
		}
		m, ok := e.(*entities.MobInstance)
		if !ok {
			continue
		}
		if mobHasTag(m, stableTag) {
			return m
		}
	}
	return nil
}

// mountOffer is one purchasable mount at a stable: a namespace-qualified mount
// template id and its price.
type mountOffer struct {
	templateID string
	price      int
}

// stableOffers reads the stablemaster's `stable.sells` block — a map of mount
// id → price — qualifying bare ids against the stablemaster's own namespace so
// they match the (qualified) mob registry. Malformed entries are skipped.
func stableOffers(c *Context, st *entities.MobInstance) []mountOffer {
	raw, ok := st.Property(stableProp)
	if !ok {
		return nil
	}
	block, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	sells, ok := block["sells"].(map[string]any)
	if !ok {
		return nil
	}
	ns := namespaceOf(string(st.TemplateID()))
	var offers []mountOffer
	for id, v := range sells {
		price := max(intFromAny(v), 0)
		offers = append(offers, mountOffer{templateID: qualifyAgainst(ns, id), price: price})
	}
	sort.Slice(offers, func(i, j int) bool { return offers[i].templateID < offers[j].templateID })
	return offers
}

// matchOffer resolves a buy query against the stable's offers by the mount's
// display name or template-id leaf (case-insensitive substring). Returns the
// first match in id order for determinism.
func matchOffer(c *Context, offers []mountOffer, query string) (mountOffer, bool) {
	q := strings.ToLower(strings.TrimSpace(query))
	for _, o := range offers {
		name, _ := c.Mounts.MountName(o.templateID)
		if templateMatches(name, o.templateID, q) {
			return o, true
		}
	}
	return mountOffer{}, false
}

// matchOwnedTemplate resolves a query against the stabled multiset by the
// mount's display name or template-id leaf. Returns the matching template id.
func matchOwnedTemplate(c *Context, stabled map[string]int, query string) (string, bool) {
	q := strings.ToLower(strings.TrimSpace(query))
	for _, t := range sortedKeys(stabled) {
		if stabled[t] <= 0 {
			continue
		}
		name, _ := c.Mounts.MountName(t)
		if templateMatches(name, t, q) {
			return t, true
		}
	}
	return "", false
}

// myLiveMountsInRoom returns the live mount creatures in the room that this
// owner has materialized (tracked live) and still owns.
func myLiveMountsInRoom(c *Context, owner mountOwner, roomID world.RoomID) []*entities.MobInstance {
	// Fast path: nothing materialized ⇒ nothing of ours to stable. The actual
	// per-mount filter is ownership (IsOwnedBy) against the live room scan —
	// the placement index already holds only live entities.
	if len(owner.LiveMountTemplates()) == 0 {
		return nil
	}
	var out []*entities.MobInstance
	for _, id := range c.Placement.InRoom(roomID) {
		e, ok := c.Items.GetByID(id)
		if !ok {
			continue
		}
		m, ok := e.(*entities.MobInstance)
		if !ok || !m.IsMount() {
			continue
		}
		if m.IsOwnedBy(ownerID(c)) {
			out = append(out, m)
		}
	}
	return out
}

// mountMatches reports whether a live mount answers to the query by its name or
// template-id leaf.
func mountMatches(c *Context, m *entities.MobInstance, query string) bool {
	return templateMatches(m.Name(), string(m.TemplateID()), strings.ToLower(strings.TrimSpace(query)))
}

// templateMatches is the shared name/id-leaf substring match (q already
// lowercased). Matches the empty query against anything.
func templateMatches(name, templateID, q string) bool {
	if q == "" {
		return true
	}
	if strings.Contains(strings.ToLower(name), q) {
		return true
	}
	return strings.Contains(strings.ToLower(leafOf(templateID)), q)
}

// ownerID is the character id a mount is owned under — the player id, falling
// back to the bare actor id for test actors.
func ownerID(c *Context) string {
	if id := c.Actor.PlayerID(); id != "" {
		return id
	}
	return c.Actor.ID()
}

// multiset counts occurrences of each string.
func multiset(in []string) map[string]int {
	m := make(map[string]int, len(in))
	for _, s := range in {
		m[s]++
	}
	return m
}

// sortedKeys returns the keys of a multiset in deterministic order.
func sortedKeys(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// namespaceOf returns the pack namespace prefix of a qualified id ("ns:leaf" →
// "ns"), or "" when the id is bare.
func namespaceOf(qualified string) string {
	if before, _, ok := strings.Cut(qualified, ":"); ok {
		return before
	}
	return ""
}

// leafOf returns the unqualified leaf of an id ("ns:leaf" → "leaf").
func leafOf(qualified string) string {
	if _, after, ok := strings.Cut(qualified, ":"); ok {
		return after
	}
	return qualified
}

// qualifyAgainst qualifies a bare id against ns ("riding-horse" → "ns:riding-horse");
// an already-qualified id passes through unchanged.
func qualifyAgainst(ns, id string) string {
	id = strings.TrimSpace(id)
	if ns == "" || strings.IndexByte(id, ':') >= 0 {
		return id
	}
	return ns + ":" + id
}

// intFromAny coerces a YAML-decoded numeric (int / int64 / float64) to int. An
// out-of-range float64 reads as 0 rather than wrapping to a huge negative via a
// bare int() cast — a wrapped negative price would otherwise be clamped to 0
// (a free mount) by the caller. Pathological content only (the sells block is
// content-authored, never player-writable), but fail loud-and-safe.
func intFromAny(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		if n < float64(math.MinInt) || n > float64(math.MaxInt) {
			return 0
		}
		return int(n)
	default:
		return 0
	}
}
