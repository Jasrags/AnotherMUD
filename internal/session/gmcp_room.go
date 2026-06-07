package session

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/gmcp"
	"github.com/Jasrags/AnotherMUD/internal/logging"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

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
		Name:     room.Name,
		Area:     string(room.AreaID),
		Exits:    exits,
		Keywords: keywords,
		Terrain:  room.Terrain,
		Details:  room.Description,
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
