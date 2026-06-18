package command

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/economy"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

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
	// Stabled = owned multiset minus the live (materialized) multiset.
	stabled := multiset(owned)
	for _, t := range owner.LiveMountTemplates() {
		if stabled[t] > 0 {
			stabled[t]--
		}
	}
	var b strings.Builder
	b.WriteString("You own:")
	for _, t := range sortedKeys(multiset(owned)) {
		name, ok := c.Mounts.MountName(t)
		if !ok {
			name = t // content drift: name the id rather than hide the asset
		}
		total := multiset(owned)[t]
		stab := stabled[t]
		for i := 0; i < total; i++ {
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
		return c.Actor.Write(ctx, fmt.Sprintf("%s costs %d gold; you only have %d.", capitalize(name), offer.price, balance))
	}
	if c.Currency == nil {
		return c.Actor.Write(ctx, "You can't pay for that right now.")
	}
	left, okDebit := c.Currency.Debit(ctx, holder, offer.price, "buymount:"+offer.templateID)
	if !okDebit {
		return c.Actor.Write(ctx, fmt.Sprintf("%s costs %d gold; you only have %d.", capitalize(name), offer.price, balance))
	}
	owner.AddMount(offer.templateID)
	return c.Actor.Write(ctx, fmt.Sprintf("You buy %s for %d gold; it is stabled here. (You have %d gold left.)", name, offer.price, left))
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
	stabledAny := false
	for _, m := range mine {
		if query != "" && !mountMatches(c, m, query) {
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
		return c.Actor.Write(ctx, "You have no such mount here to stable.")
	}
	return nil
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
		price := intFromAny(v)
		if price < 0 {
			price = 0
		}
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
	if i := strings.IndexByte(qualified, ':'); i >= 0 {
		return qualified[:i]
	}
	return ""
}

// leafOf returns the unqualified leaf of an id ("ns:leaf" → "leaf").
func leafOf(qualified string) string {
	if i := strings.IndexByte(qualified, ':'); i >= 0 {
		return qualified[i+1:]
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

// intFromAny coerces a YAML-decoded numeric (int / int64 / float64) to int.
func intFromAny(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return 0
	}
}
