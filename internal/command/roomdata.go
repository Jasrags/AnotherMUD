package command

import (
	"context"
	"fmt"
	"sort"
	"strings"

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
func AppendRoomData(base string, r *world.Room, viewer Actor, adminRole string) string {
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
	return base + "\n" + renderRoomData(r)
}

// renderRoomWithData renders r for this actor (RenderRoom) and appends
// the admin/builder room-data block via AppendRoomData. The shared entry
// every Context-based room render uses so the builder view is consistent
// across look and the arrival renders. lvl is supplied by the caller
// because the movement path computes the destination light specially.
func (c *Context) renderRoomWithData(r *world.Room, lvl light.Level) string {
	out := RenderRoom(r, c.Placement, c.Items, c.questMarker(), c.Ambience,
		c.hostileMarker(), lvl, c.canSeeExit, QuestSpawnVisible(c.Actor.PlayerID()), c.otherPlayerNames(r.ID)...)
	out = AppendMinimap(out, r, c.Actor, c.World)
	return AppendRoomData(out, r, c.Actor, c.AdminRole)
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
			rows = append(rows, row{key: d.Long(), line: fmt.Sprintf("%s -> %s%s", d.Short(), e.Target, doorSuffix(e))})
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
