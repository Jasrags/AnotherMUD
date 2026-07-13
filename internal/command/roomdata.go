package command

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/light"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// RoomDataViewer is the optional actor capability the `roomdata` toggle
// and the `look` admin block read: a persisted on/off preference for the
// room-metadata block. connActor satisfies it; an actor that does not
// implement it is treated as having the toggle off and unsettable.
type RoomDataViewer interface {
	ShowRoomData() bool
	SetShowRoomData(bool)
}

// RoomDataHandler toggles the calling admin's room-metadata `look`
// preference. Admin-only (gated at dispatch): `roomdata` flips it,
// `roomdata on|off` sets it explicitly. The block itself renders in
// LookHandler when this preference is on AND the viewer holds the admin
// role — the toggle is a display preference, the role is the gate.
func RoomDataHandler(ctx context.Context, c *Context) error {
	v, ok := c.Actor.(RoomDataViewer)
	if !ok {
		return c.Actor.Write(ctx, "Room data display is not available.")
	}
	return applyBinaryToggle(ctx, c, "roomdata", v.ShowRoomData(), v.SetShowRoomData,
		"Room data display ON.", "Room data display OFF.")
}

// AppendRoomData returns base with the admin/builder room-data block
// appended when viewer holds the admin role (adminRole, defaulting to
// the engine default when empty) AND has the `roomdata` toggle on;
// otherwise base is returned unchanged. This is the single gate shared
// by every "you see the room" render — `look`, movement, recall,
// teleport, flee, the login spawn, and link-dead reattach — so the
// builder view appears on room entry, not only on an explicit `look`.
// viewer may be any Actor; one that does not implement RoleHolder +
// RoomDataViewer simply gets base back.
func AppendRoomData(base string, r *world.Room, viewer Actor, adminRole string, extras ...string) string {
	if r == nil || viewer == nil {
		return base
	}
	role := adminRole
	if role == "" {
		role = defaultAdminRole
	}
	if rh, ok := viewer.(RoleHolder); !ok || !rh.HasRole(role) {
		return base
	}
	if rdv, ok := viewer.(RoomDataViewer); !ok || !rdv.ShowRoomData() {
		return base
	}
	block := renderRoomData(r)
	// extras carries the Context-derived lines (resolved biome + hazard, the
	// effective light level, current weather) that renderRoomData can't build
	// from the room alone. Optional so the free-function/test callers still work.
	for _, e := range extras {
		if strings.TrimSpace(e) != "" {
			block += "\n" + e
		}
	}
	return base + "\n" + block
}

// renderRoomWithData renders r for this actor (RenderRoom) and appends
// the admin/builder room-data block via AppendRoomData. The shared entry
// every Context-based room render uses so the builder view is consistent
// across look and the arrival renders. lvl is supplied by the caller
// because the movement path computes the destination light specially.
func (c *Context) renderRoomWithData(r *world.Room, lvl light.Level) string {
	out := RenderRoom(r, c.Placement, c.Items, c.questMarker(), c.Ambience,
		c.hostileMarker(), lvl, c.canSeeExit, QuestSpawnVisible(c.Actor, c.AdminRole), c.otherPlayerNames(r.ID)...)
	out = AppendMinimap(out, r, c.Actor, c.World)
	return AppendRoomData(out, r, c.Actor, c.AdminRole, c.roomDataExtras(r, lvl))
}

// roomDataExtras builds the Context-derived room-data lines that
// renderRoomData can't produce from the room alone: the resolved biome
// (display name, move cost, ambience count, and — the recent work — its
// intrinsic ambient hazard), the effective light level for this render, and
// the current weather when the room is weather-exposed. Returns "" when none
// of the deps are wired (tests) so the block degrades cleanly to the static
// fields. Labels render as <subtle> chrome, matching renderRoomData.
func (c *Context) roomDataExtras(r *world.Room, lvl light.Level) string {
	if r == nil {
		return ""
	}
	var b strings.Builder

	// Resolved biome + its intrinsic hazard (biomes.md / area-effects.md §4.6).
	if c.Biomes != nil {
		if bm, ok := c.Biomes.Resolve(r.Terrain); ok {
			name := bm.DisplayName
			if name == "" {
				name = bm.ID
			}
			fmt.Fprintf(&b, "<subtle>biome</subtle> %s (%s)   <subtle>move</subtle> %d   <subtle>ambience</subtle> %d line(s)\n",
				name, bm.ID, bm.MoveCost, len(bm.Ambience))
			if bm.Hazard.Active() {
				prot := bm.Hazard.ProtectionKey
				if prot == "" {
					prot = "none"
				}
				dtype := bm.Hazard.DamageType
				if dtype == "" {
					dtype = "untyped"
				}
				fmt.Fprintf(&b, "<subtle>hazard</subtle> %d %s / tick   <subtle>protection</subtle> %s (worn)\n",
					bm.Hazard.Damage, dtype, prot)
			}
		}
	}

	// Live occupants (mobs + loose items on the room floor), by template id +
	// name. The builder block lists ALL of them — hidden, invisible, and
	// other players' quest spawns included — since a GM wants ground truth,
	// not the per-observer view. Sorted for a deterministic dump.
	if c.Placement != nil && c.Items != nil {
		var mobs, items []string
		for _, id := range c.Placement.InRoom(r.ID) {
			e, ok := c.Items.GetByID(id)
			if !ok {
				continue
			}
			switch v := e.(type) {
			case *entities.MobInstance:
				mobs = append(mobs, fmt.Sprintf("%s <%s>", v.Name(), v.TemplateID()))
			case *entities.ItemInstance:
				items = append(items, fmt.Sprintf("%s <%s>", v.Name(), v.TemplateID()))
			}
		}
		sort.Strings(mobs)
		sort.Strings(items)
		if len(mobs) > 0 {
			fmt.Fprintf(&b, "<subtle>mobs </subtle> %s\n", strings.Join(mobs, ", "))
		}
		if len(items) > 0 {
			fmt.Fprintf(&b, "<subtle>items</subtle> %s\n", strings.Join(items, ", "))
		}
	}

	// Effective light for this render + current weather when exposed.
	fmt.Fprintf(&b, "<subtle>light</subtle> %s (effective)", lvl.String())
	if r.WeatherExposed && c.WeatherState != nil {
		if w := c.WeatherState(r.AreaID); w != "" {
			fmt.Fprintf(&b, "   <subtle>weather</subtle> %s", w)
		}
	}
	b.WriteString("\n")

	return strings.TrimRight(b.String(), "\n")
}

// renderRoomData builds the admin/builder metadata block appended to
// `look`: the room/area ids, the area-local coordinate and its source
// (room-coordinates §3), terrain + exposure flags, tags, the property
// bag, and every exit's target with door state. Labels render as
// <subtle> so the block reads as out-of-narrative chrome. Tags,
// properties, and exits are sorted so the dump is deterministic.
//
// The block is intentionally NOT light-gated (a builder inspecting a
// dark room still wants the data) and shows content the admin already
// controls, so authored markup in property values rides through the
// renderer the same way a room description does.
func renderRoomData(r *world.Room) string {
	var b strings.Builder
	b.WriteString("<subtle>── room data ──────────────────────────────</subtle>\n")
	fmt.Fprintf(&b, "<subtle>room </subtle> %s\n", r.ID)
	fmt.Fprintf(&b, "<subtle>area </subtle> %s\n", r.AreaID)

	coord := "unplaced"
	if r.Coord != nil {
		src := "derived"
		if r.Pin != nil {
			src = "pinned"
		}
		coord = fmt.Sprintf("(%d,%d,%d) %s", r.Coord.X, r.Coord.Y, r.Coord.Z, src)
	}
	terrain := r.Terrain
	if terrain == "" {
		terrain = "outdoors (default)"
	}
	fmt.Fprintf(&b, "<subtle>coord</subtle> %s   <subtle>terrain</subtle> %s   <subtle>heal</subtle> %+d\n",
		coord, terrain, r.HealingRate)

	var flags []string
	if r.WeatherExposed {
		flags = append(flags, "weather-exposed")
	}
	if r.TimeExposed {
		flags = append(flags, "time-exposed")
	}
	if len(flags) > 0 {
		fmt.Fprintf(&b, "<subtle>flags</subtle> %s\n", strings.Join(flags, ", "))
	}

	if len(r.Tags) > 0 {
		tags := append([]string(nil), r.Tags...)
		sort.Strings(tags)
		fmt.Fprintf(&b, "<subtle>tags </subtle> %s\n", strings.Join(tags, ", "))
	}

	if len(r.Properties) > 0 {
		keys := make([]string, 0, len(r.Properties))
		for k := range r.Properties {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, len(keys))
		for i, k := range keys {
			parts[i] = fmt.Sprintf("%s=%v", k, r.Properties[k])
		}
		fmt.Fprintf(&b, "<subtle>props</subtle> %s\n", strings.Join(parts, ", "))
	}

	// Directional exits, sorted by long name, each with door state.
	if len(r.Exits) > 0 {
		type row struct{ key, line string }
		rows := make([]row, 0, len(r.Exits))
		for d, e := range r.Exits {
			rows = append(rows, row{key: d.Long(), line: fmt.Sprintf("%s -> %s%s%s", d.Short(), e.Target, doorSuffix(e), hiddenSuffix(e))})
		}
		sort.Slice(rows, func(i, j int) bool { return rows[i].key < rows[j].key })
		for i, rw := range rows {
			label := "exits"
			if i > 0 {
				label = "     "
			}
			fmt.Fprintf(&b, "<subtle>%s</subtle> %s\n", label, rw.line)
		}
	}

	// Keyword exits (portals) listed after directional exits.
	if len(r.KeywordExits) > 0 {
		kws := make([]string, 0, len(r.KeywordExits))
		for k := range r.KeywordExits {
			kws = append(kws, k)
		}
		sort.Strings(kws)
		for i, k := range kws {
			label := "portal"
			if i > 0 {
				label = "      "
			}
			fmt.Fprintf(&b, "<subtle>%s</subtle> %s -> %s\n", label, k, r.KeywordExits[k].Target)
		}
	}

	return strings.TrimRight(b.String(), "\n")
}

// hiddenSuffix renders a secret exit's concealment for the builder exits
// line (hidden-exits §2), or "" when the exit is not hidden. The builder
// block shows hidden exits unconditionally (unlike the player view, which
// filters them per-observer), with the search DC when authored.
func hiddenSuffix(e world.Exit) string {
	if !e.Hidden {
		return ""
	}
	if e.SearchDifficulty > 0 {
		return fmt.Sprintf(" (hidden, dc %d)", e.SearchDifficulty)
	}
	return " (hidden)"
}

// doorSuffix renders an exit's door state for the builder exits line, or
// "" when the exit has no door.
func doorSuffix(e world.Exit) string {
	if e.Door == nil {
		return ""
	}
	switch {
	case e.Door.Locked:
		return " (door: locked)"
	case e.Door.Closed:
		return " (door: closed)"
	default:
		return " (door: open)"
	}
}
