package command

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/light"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// RenderRoom is the M1 room renderer, extended in M6.3 to include
// Placement-tracked entities (items + mobs). Replaced by the
// ui-rendering-help pipeline in a later milestone; lives here for now
// so the session layer has something to call.
//
// placement and items may be nil — older callers and tests that only
// care about geography pass nil for both. When supplied, the renderer
// appends a "You see here:" line listing each placed entity by name
// in insertion order. Entities nested inside containers are not
// shown: those live in Contents, not Placement (the put pipeline
// removes from Placement when nesting).
// otherPlayerNames returns the display names of players in roomID other
// than the acting player, for the room render's "You see here:" line.
// Empty when no Locator is wired (tests / headless paths).
func (c *Context) otherPlayerNames(roomID world.RoomID) []string {
	if c.Locator == nil {
		return nil
	}
	self := c.Actor.PlayerID()
	var out []string
	for _, p := range c.Locator.PlayersInRoom(roomID) {
		if p == nil || (self != "" && p.PlayerID() == self) {
			continue
		}
		if n := p.Name(); n != "" {
			out = append(out, n)
		}
	}
	return out
}

// hostileMarker returns a predicate reporting whether a placed mob is
// hostile to the viewing actor, for RenderRoom's red coloring. Returns
// nil when disposition is unwired or the actor has no player id (tests,
// pre-login) so the renderer falls back to the neutral <present.mob>
// color. Players carry no tags here — the same nil-tags simplification
// the room-entry hook already uses (move handler above) — so the v1
// reddens statically-hostile mobs and tag-free hostile rules.
func (c *Context) hostileMarker() func(*entities.MobInstance) bool {
	if c.Disposition == nil || c.Actor == nil {
		return nil
	}
	pid := c.Actor.PlayerID()
	if pid == "" {
		return nil
	}
	name := c.Actor.Name()
	return func(m *entities.MobInstance) bool {
		return c.Disposition.Hostile(m, pid, name, nil)
	}
}

// RenderRoom renders a room's name, description, entities, and exits.
//
// marker, when non-nil, reports whether an entity's template id
// carries a quest marker for the viewer (M10.10b); such entities get
// a marker glyph before their name. Pass nil to skip marker
// decoration.
//
// ambience, when non-nil and non-empty for r, is appended after the
// room description on its own line. The current consumer is the
// M15.4b₂b weather hook (weather.Service.Ambience). Pass nil for
// renderers (tests, link-dead recovery before weather is wired)
// that don't have an ambience source; an empty return from a
// non-nil ambience is also treated as "nothing to render".
// players are the display names of OTHER players present (the viewer
// excludes themselves before calling). Variadic so existing callers
// without a player list stay source-compatible; players are listed in
// the "You see here:" line alongside mobs and items.
// hostile, when non-nil, reports whether a placed mob is hostile to the
// viewer; such mobs render in <present.hostile> (red) instead of the
// neutral <present.mob>. Pass nil (tests, renderers without a
// disposition source) to color every mob neutrally.
//
// lvl is the viewer's effective light level (light-and-darkness §5.1).
// It branches the render: `lit` is the full render; `dim` is the full
// render with the description muted; `gloom` obscures (terse prose,
// coarse occupant presence with identities hidden, bare-direction
// exits); `black` suppresses everything to a single dark line. Callers
// that do not gate on light (tests, unwired paths) pass light.Lit for
// the unchanged full render.
func RenderRoom(r *world.Room, placement *entities.Placement, items *entities.Store, marker func(templateID string) bool, ambience func(*world.Room) string, hostile func(*entities.MobInstance) bool, lvl light.Level, players ...string) string {
	switch {
	case lvl <= light.Black:
		// Suppressed: name, description, occupants all withheld (§5.1).
		return "<subtle>" + blackRoomText + "</subtle>"
	case lvl == light.Gloom:
		return renderGloomRoom(r, placement, items, players)
	default:
		// Lit or Dim: full render; dim mutes the description prose.
		return renderFullRoom(r, placement, items, marker, ambience, hostile, lvl == light.Dim, players)
	}
}

// Reduced-light render strings (§5.1). Hardcoded for v1; externalizing
// them to the configuration surface (§11) is deferred.
const (
	blackRoomText = "It is pitch black. You can see nothing."
	gloomRoomText = "It is too dark to make out any detail; you can sense only shapes and directions."
)

// renderFullRoom is the lit/dim render: the room name, description,
// ambience, occupants, and exits. When dim is true the description is
// wrapped in the {dim} attribute so the prose reads muted while the
// rest of the body keeps its semantic colors (a single SGR attribute
// over plain prose, so no nested-tag reset problem). Both forms degrade
// to clean text on no-color clients.
func renderFullRoom(r *world.Room, placement *entities.Placement, items *entities.Store, marker func(templateID string) bool, ambience func(*world.Room) string, hostile func(*entities.MobInstance) bool, dim bool, players []string) string {
	var b strings.Builder
	b.WriteString("<title>")
	b.WriteString(r.Name)
	b.WriteString("</title>")
	b.WriteString("\n")
	desc := reflowDescription(r.Description)
	if dim && desc != "" {
		b.WriteString("{dim}")
		b.WriteString(desc)
		b.WriteString("{/}")
	} else {
		b.WriteString(desc)
	}
	b.WriteString("\n")
	if ambience != nil {
		if line := ambience(r); line != "" {
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	if line := renderRoomEntities(r, placement, items, marker, hostile, players); line != "" {
		b.WriteString(line)
		b.WriteString("\n")
	}
	b.WriteString(renderExits(r))
	return b.String()
}

// reflowDescription treats a room description's single newlines as SOFT
// wraps — authored line breaks added for source readability, not real
// breaks — joining each paragraph's lines into one flowing line so the
// prose re-wraps cleanly to whatever width the room column is. Blank
// lines (a double newline) are kept as hard paragraph breaks. Without
// this, the authored ~76-column breaks fight the side-by-side minimap's
// narrower column and orphan the trailing word of every authored line.
// Whitespace within a paragraph is collapsed to single spaces.
func reflowDescription(s string) string {
	paras := strings.Split(s, "\n\n")
	out := make([]string, 0, len(paras))
	for _, p := range paras {
		if joined := strings.Join(strings.Fields(p), " "); joined != "" {
			out = append(out, joined)
		}
	}
	return strings.Join(out, "\n\n")
}

// renderGloomRoom is the obscured render (§5.1 gloom): the room name
// still anchors (you know where you stand), but the prose is replaced
// by a terse dark line, occupants are coarsened to presence-without-
// identity (names hidden), and exits render as bare directions with no
// door/weather detail.
func renderGloomRoom(r *world.Room, placement *entities.Placement, items *entities.Store, players []string) string {
	var b strings.Builder
	b.WriteString("<title>")
	b.WriteString(r.Name)
	b.WriteString("</title>")
	b.WriteString("\n")
	b.WriteString("{dim}")
	b.WriteString(gloomRoomText)
	b.WriteString("{/}")
	b.WriteString("\n")
	if line := renderCoarseOccupants(r, placement, items, players); line != "" {
		b.WriteString(line)
		b.WriteString("\n")
	}
	b.WriteString(renderBareExits(r))
	return b.String()
}

// renderCoarseOccupants lists occupant PRESENCE at gloom without
// identity: each other player and each placed mob becomes an
// anonymous shape; items are not made out at all (objects need detail).
// Names are hidden — the §5.1 occupant-coarsening rule. The
// granularity here (one anonymous token per occupant) is the v1
// default; configurable presence/count/kind granularity (§11) is
// deferred.
func renderCoarseOccupants(r *world.Room, placement *entities.Placement, items *entities.Store, players []string) string {
	shapes := make([]string, 0, len(players))
	for range players {
		shapes = append(shapes, "someone")
	}
	if placement != nil && items != nil {
		for _, id := range placement.InRoom(r.ID) {
			e, ok := items.GetByID(id)
			if !ok {
				continue
			}
			if _, ok := e.(*entities.MobInstance); ok {
				shapes = append(shapes, "a shape")
			}
		}
	}
	if len(shapes) == 0 {
		return ""
	}
	return "<subtle>You can make out:</subtle> " + strings.Join(shapes, ", ") + "."
}

// renderBareExits lists exit directions only — no door state, no
// decoration — for the gloom render (§5.1: "exits shown as bare
// directions").
func renderBareExits(r *world.Room) string {
	if len(r.Exits) == 0 {
		return "<subtle>Exits:</subtle> none"
	}
	longs := make([]string, 0, len(r.Exits))
	for d := range r.Exits {
		longs = append(longs, "<exit>"+d.Long()+"</exit>")
	}
	sort.Strings(longs)
	return "<subtle>Exits:</subtle> " + strings.Join(longs, ", ")
}

// renderRoomEntities builds the "You see here: …" line. Other players
// (passed in, viewer already excluded) list first, then placed
// mobs/items. Returns the empty string when there's nothing to show —
// no other players AND no resolvable placed entities (Placement/Store
// nil, no placed ids, or every placed id fails resolution). Each
// entity branch is a silent skip rather than a panic because the
// renderer is on the player-visible path; missing data should look
// like nothing-here, not a runtime error.
func renderRoomEntities(r *world.Room, placement *entities.Placement, items *entities.Store, marker func(templateID string) bool, hostile func(*entities.MobInstance) bool, players []string) string {
	// Other players first (highlighted), then placed mobs/items colored
	// by kind. The "You see here:" label dims to <subtle> so the names
	// it introduces carry the visual weight.
	names := make([]string, 0, len(players))
	for _, p := range players {
		names = append(names, "<present.player>"+p+"</present.player>")
	}
	ids := []entities.EntityID(nil)
	if placement != nil && items != nil {
		ids = placement.InRoom(r.ID)
	}
	for _, id := range ids {
		e, ok := items.GetByID(id)
		if !ok {
			continue
		}
		n, ok := e.(interface{ Name() string })
		if !ok {
			continue
		}
		name := n.Name()
		if name == "" {
			continue
		}
		// Color the bare name by entity kind first, then prepend the
		// quest marker OUTSIDE the color tag so the two never nest
		// (spec §2.4: a nested tag's close resets the outer color).
		name = colorizeEntityName(e, name, hostile)
		if marker != nil {
			if tid := templateIDOf(e); tid != "" && marker(tid) {
				name = "<good>(!)</good> " + name
			}
		}
		names = append(names, name)
	}
	if len(names) == 0 {
		return ""
	}
	return "<subtle>You see here:</subtle> " + strings.Join(names, ", ") + "."
}

// colorizeEntityName wraps a placed entity's display name in the
// semantic tag for its kind: items take an item.* rarity tag (from the
// reserved "rarity" instance property — the same source the
// item-decorations system reads, decorate.go) and mobs take
// <present.mob>. Other players are tagged at the call site (they arrive
// as bare names, not entities). An unrecognized entity kind renders
// plain. hostile, when non-nil, reddens a mob the viewer is hostile
// toward (<present.hostile>) instead of the neutral mob color.
func colorizeEntityName(e entities.Entity, name string, hostile func(*entities.MobInstance) bool) string {
	switch inst := e.(type) {
	case *entities.ItemInstance:
		tag := itemRarityTag(inst)
		return "<" + tag + ">" + name + "</" + tag + ">"
	case *entities.MobInstance:
		if hostile != nil && hostile(inst) {
			return "<present.hostile>" + name + "</present.hostile>"
		}
		return "<present.mob>" + name + "</present.mob>"
	default:
		return name
	}
}

// itemRarityTag returns the item.<key> theme tag for an item's rarity,
// read from the canonical "rarity" instance property (propRarity in
// decorate.go). Only the tiers the default theme ships colors for are
// honored; an absent, empty, or unrecognized key falls back to
// item.common so the room line never emits an unregistered tag (which
// the renderer would pass through as literal "<item.foo>" text). A pack
// that adds a custom tier registers its color via decoration but won't
// be name-colored in the room line until this whitelist or a registry
// hand-off grows — a deliberate safe default, not a silent drop.
func itemRarityTag(it *entities.ItemInstance) string {
	key, ok := stringProp(it, propRarity)
	if !ok {
		return "item.common"
	}
	switch key {
	case "uncommon", "rare", "legendary":
		return "item." + key
	default:
		return "item.common"
	}
}

// templateIDOf returns the content template id of an entity (item or
// mob), or "" when it has none.
func templateIDOf(e entities.Entity) string {
	switch inst := e.(type) {
	case *entities.ItemInstance:
		return string(inst.TemplateID())
	case *entities.MobInstance:
		return string(inst.TemplateID())
	default:
		return ""
	}
}

func renderExits(r *world.Room) string {
	if len(r.Exits) == 0 {
		return "<subtle>Exits:</subtle> none"
	}
	// Build a slice of (long-name, decorated-name) pairs so we can
	// sort by long-name (stable, alphabetical) while emitting the
	// decorated form (M15.1c: doors render their state).
	type labelled struct{ key, label string }
	out := make([]labelled, 0, len(r.Exits))
	for d, e := range r.Exits {
		out = append(out, labelled{key: d.Long(), label: decorateExit(d, e)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].key < out[j].key })
	labels := make([]string, len(out))
	for i, lb := range out {
		labels[i] = lb.label
	}
	return fmt.Sprintf("<subtle>Exits:</subtle> %s", strings.Join(labels, ", "))
}

// decorateExit returns the exit's long-name with door state appended
// when the exit carries a door. Format: "north (closed)",
// "north (locked)", "north (open)". An unlocked open door renders
// as a plain direction since "open" is the implicit default; an
// open BUT locked door cannot exist (locked implies closed).
//
// The direction word renders as an <exit> (cyan) so exits read as
// actionable; a closed door's "(closed)" suffix reuses <warning>
// (yellow) and a locked door's "(locked)" reuses <danger> (red), so
// the obstacle stands out without a legend. The severity tag sits
// OUTSIDE the <exit> tag (sequential, not nested) per spec §2.4.
func decorateExit(d world.Direction, e world.Exit) string {
	long := "<exit>" + d.Long() + "</exit>"
	if e.Door == nil {
		return long
	}
	switch {
	case e.Door.Locked:
		return long + " <danger>(locked)</danger>"
	case e.Door.Closed:
		return long + " <warning>(closed)</warning>"
	default:
		return long
	}
}
