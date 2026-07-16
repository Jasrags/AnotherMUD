package session

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/gmcp"
	"github.com/Jasrags/AnotherMUD/internal/logging"
	"github.com/Jasrags/AnotherMUD/internal/render"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// gmcpPlain strips both markup systems (brace shorthand {Y}…{x} and angle
// tags <…>) from a display string so GMCP carries clean, structured text.
// GMCP is data for the client to style itself — the terminal's colour markup
// must not leak into it (a graphical mapper would render the raw codes as its
// room label). A no-op for strings without markup.
func gmcpPlain(s string) string {
	// StripTagsLenient (not StripTags): a room description is free text, so a
	// stray '<' (e.g. "the blade is <2ft long") is content, not markup — it must
	// survive to the wire rather than truncate the rest of the description.
	return render.StripBraces(render.StripTagsLenient(s))
}

// sendGmcpRoomInfo emits a Room.Info GMCP frame for the actor's
// current location to the peer. Event-driven (unlike Char.Vitals'
// poll-and-diff): every call sends, no shadow / diff. Callers:
//
//   - connActor.SetRoom — every transition, the canonical movement
//     seam.
//   - the login spawn render — first frame when the player enters
//     the world.
//   - connActor.reattach — link-dead recovery; the new peer needs
//     a baseline frame for its map panel even when the player
//     didn't move during the drop.
//
// Silent no-op when:
//
//   - the underlying conn doesn't speak GMCP;
//   - GMCP hasn't been negotiated;
//   - room is nil (defensive — every caller guards already).
//
// Safe for concurrent callers; the conn's own write mutex
// serializes the wire write.
func (a *connActor) sendGmcpRoomInfo(ctx context.Context, room *world.Room) {
	if room == nil {
		return
	}
	sender, ok := a.conn.(gmcpSender)
	if !ok || !sender.GmcpActive() {
		return
	}
	payload := buildRoomInfoPayload(room)
	// light-and-darkness §8: surface this viewer's effective light so a
	// capable client can theme the viewport / swap a day-night map.
	// Per-viewer (held light + darkvision), computed here where the
	// actor is in hand. Omitted when the resolver is unwired.
	if a.light != nil {
		payload.Light = command.EffectiveLight(a.light, room, a, a.items, a.placement).String()
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	if err := sender.SendGmcp(ctx, gmcp.PackageRoomInfo, data); err != nil {
		logging.From(ctx).Debug("gmcp room.info send failed",
			slog.String("player", a.PlayerName()),
			slog.String("room", string(room.ID)),
			slog.Any("err", err))
	}
	// web-client P2: the rich neighbourhood package rides the same transition.
	// A baseline client ignores it; a map client draws the walkable local graph.
	a.sendGmcpRoomMap(ctx, room)
}

// defaultRoomMapRadius is the Room.Map BFS bound when Config.RoomMapRadius is
// unset (0): the WHOLE AREA (-1, unbounded), so the web map shows the same rooms
// as the in-game `map` verb. A positive ANOTHERMUD_ROOM_MAP_RADIUS bounds it —
// useful for a very large area where a whole-area frame on every transition
// would be heavy.
const defaultRoomMapRadius = -1

// sendGmcpRoomMap emits a Room.Map GMCP frame — the local neighbourhood graph
// around room (web-client-plan P2) — to the peer. Same transition seam and
// same no-op guards as sendGmcpRoomInfo (nil room, non-GMCP conn, GMCP inactive),
// plus a nil-world guard (a worldless test boot emits no map). The neighbourhood
// is a.world.LocalWindow (the shared map seam) intersected with the viewer's
// fog-of-war visited set.
func (a *connActor) sendGmcpRoomMap(ctx context.Context, room *world.Room) {
	if room == nil || a.world == nil {
		return
	}
	sender, ok := a.conn.(gmcpSender)
	if !ok || !sender.GmcpActive() {
		return
	}
	radius := a.roomMapRadius
	if radius == 0 { // unset ⇒ whole area (matches the `map` verb); negative also means whole area
		radius = defaultRoomMapRadius
	}
	win, err := a.world.LocalWindow(room.ID, radius)
	if err != nil {
		return
	}
	payload := buildRoomMapPayload(win, room.ID, radius, a.HasVisited)
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	if err := sender.SendGmcp(ctx, gmcp.PackageRoomMap, data); err != nil {
		logging.From(ctx).Debug("gmcp room.map send failed",
			slog.String("player", a.PlayerName()),
			slog.String("room", string(room.ID)),
			slog.Any("err", err))
	}
}

// buildRoomMapPayload converts a world.Window into the Room.Map payload:
// each placed room becomes a node with its coordinate, short-form directional
// exits (target ids), and the viewer's visited flag (via hasVisited). Pure over
// its inputs — the fog check is injected so it is testable without a live actor.
func buildRoomMapPayload(win world.Window, center world.RoomID, radius int, hasVisited func(string) bool) gmcp.RoomMap {
	nodes := make([]gmcp.RoomMapNode, 0, len(win.Rooms))
	for _, wr := range win.Rooms {
		var exits map[string]string
		if len(wr.Room.Exits) > 0 {
			exits = make(map[string]string, len(wr.Room.Exits))
			for dir, exit := range wr.Room.Exits {
				short := dir.Short()
				if short == "" {
					continue
				}
				exits[short] = string(exit.Target)
			}
		}
		id := string(wr.Room.ID)
		// The center room is definitionally visited — you are standing in it —
		// even if the persisted fog set hasn't recorded it yet (a fresh character's
		// start room is marked on its first move, after this login-spawn frame).
		visited := id == string(center) || hasVisited(id)
		nodes = append(nodes, gmcp.RoomMapNode{
			Num:     id,
			Name:    gmcpPlain(wr.Room.Name),
			X:       wr.Coord.X,
			Y:       wr.Coord.Y,
			Z:       wr.Coord.Z,
			Exits:   exits,
			Visited: visited,
		})
	}
	return gmcp.RoomMap{Center: string(center), Radius: radius, Rooms: nodes}
}

// buildRoomInfoPayload converts a world.Room into the spec §7
// Room.Info payload shape. Direction exits flatten to short-form
// keys (n/s/e/w/u/d); keyword exits (M15.2 portals) land under
// their own keyword keys. Both maps omit when empty so the wire
// stays minimal for rooms without exits / portals.
func buildRoomInfoPayload(room *world.Room) gmcp.RoomInfo {
	exits := make(map[string]string, len(room.Exits))
	for dir, exit := range room.Exits {
		short := dir.Short()
		if short == "" {
			continue
		}
		exits[short] = string(exit.Target)
	}
	var keywords map[string]string
	if len(room.KeywordExits) > 0 {
		keywords = make(map[string]string, len(room.KeywordExits))
		for kw, exit := range room.KeywordExits {
			keywords[kw] = string(exit.Target)
		}
	}
	info := gmcp.RoomInfo{
		Num:      string(room.ID),
		Name:     gmcpPlain(room.Name),
		Area:     string(room.AreaID),
		Exits:    exits,
		Keywords: keywords,
		Terrain:  room.Terrain,
		Details:  gmcpPlain(room.Description),
	}
	// Area-local coordinate (room-coordinates §5), emitted only for a
	// placed room. An unplaced room (§4.3) leaves Coord nil and the
	// fields stay omitted — the mapper falls back to its own relative
	// placement (§5.1). Copy into fresh ints so the payload never
	// aliases the shared world.Room.
	if room.Coord != nil {
		x, y, z := room.Coord.X, room.Coord.Y, room.Coord.Z
		info.X, info.Y, info.Z = &x, &y, &z
	}
	return info
}
