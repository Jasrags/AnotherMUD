package session

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/gmcp"
	"github.com/Jasrags/AnotherMUD/internal/notifications"
	"github.com/Jasrags/AnotherMUD/internal/player"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// newCommChannelActor builds a connActor whose conn supports GMCP.
// The actor's actorSink is the wire under test — Deliver routes
// "channel" notifications to sendCommChannelText.
func newCommChannelActor(t *testing.T, playerID string) (*connActor, *gmcpFakeConn) {
	t.Helper()
	fc := &gmcpFakeConn{fakeConn: fakeConn{id: "test-" + playerID}}
	room := &world.Room{ID: "test-room", Name: "Test"}
	a := &connActor{
		id:       fc.id,
		conn:     fc,
		playerID: playerID,
		room:     room,
		vitals:   combat.NewVitalsAt(50, 100),
		save:     &player.Save{ID: playerID, Name: playerID, Sustenance: 100},
	}
	a.sustenance = 100
	return a, fc
}

func commChannelFrames(t *testing.T, fc *gmcpFakeConn) []gmcp.CommChannelText {
	t.Helper()
	raw := fc.framesSnapshot()
	out := make([]gmcp.CommChannelText, 0, len(raw))
	for _, f := range raw {
		if f.pkg != gmcp.PackageCommChannelText {
			continue
		}
		var c gmcp.CommChannelText
		if err := json.Unmarshal(f.payload, &c); err != nil {
			t.Fatalf("payload unmarshal: %v (raw %s)", err, f.payload)
		}
		out = append(out, c)
	}
	return out
}

func TestActorSink_Deliver_ChannelEmitsGmcpAlongsideText(t *testing.T) {
	a, fc := newCommChannelActor(t, "p-1")
	fc.setActive(true)

	sink := actorSink{a: a}
	err := sink.Deliver(context.Background(), notifications.Notification{
		Kind:    "channel",
		Text:    "[ooc] Alice: hello",
		Sender:  "Alice",
		Channel: "ooc",
	})
	if err != nil {
		t.Fatalf("Deliver: %v", err)
	}

	// Main-window text shipped via fakeConn writes; channel frame
	// shipped via the GMCP path.
	frames := commChannelFrames(t, fc)
	if len(frames) != 1 {
		t.Fatalf("emitted %d Comm.Channel frames, want 1", len(frames))
	}
	got := frames[0]
	if got.Channel != "ooc" || got.Talker != "Alice" || got.Text != "[ooc] Alice: hello" {
		t.Errorf("frame = %+v, want ooc/Alice/[ooc] Alice: hello", got)
	}
}

func TestActorSink_Deliver_ChannelWithoutChannelIDSkipsGmcp(t *testing.T) {
	// Defensive: a "channel"-kind notification with an empty
	// Channel field (caller bug, or a non-chat-verb path that
	// repurposes the kind) must NOT emit a GMCP frame with an
	// empty channel id. The main-window text still ships.
	a, fc := newCommChannelActor(t, "p-1")
	fc.setActive(true)

	sink := actorSink{a: a}
	_ = sink.Deliver(context.Background(), notifications.Notification{
		Kind:   "channel",
		Text:   "[??] Alice: malformed",
		Sender: "Alice",
		// Channel intentionally empty
	})

	if got := len(commChannelFrames(t, fc)); got != 0 {
		t.Errorf("empty channel id emitted %d frames, want 0", got)
	}
}

func TestActorSink_Deliver_ChannelGmcpInactiveDoesNotEmit(t *testing.T) {
	// GMCP not negotiated → no frame, but the main-window text
	// still ships normally.
	a, fc := newCommChannelActor(t, "p-1")
	// fc.active stays false (default).

	sink := actorSink{a: a}
	_ = sink.Deliver(context.Background(), notifications.Notification{
		Kind: "channel", Text: "[ooc] Alice: hi", Sender: "Alice", Channel: "ooc",
	})

	if got := len(commChannelFrames(t, fc)); got != 0 {
		t.Errorf("pre-activation emitted %d frames, want 0", got)
	}
}

func TestActorSink_Deliver_TellDoesNotEmitCommChannel(t *testing.T) {
	// A tell-kind notification must not produce a Comm.Channel
	// frame even if Channel happens to be populated (defensive
	// against future cross-population).
	a, fc := newCommChannelActor(t, "p-1")
	fc.setActive(true)

	sink := actorSink{a: a}
	_ = sink.Deliver(context.Background(), notifications.Notification{
		Kind: "tell", Text: "Alice tells you, 'hi'", Sender: "Alice", Channel: "ooc",
	})

	if got := len(commChannelFrames(t, fc)); got != 0 {
		t.Errorf("tell emitted %d Comm.Channel frames, want 0", got)
	}
}

func TestActorSink_Deliver_ChannelSystemMessageOmitsTalker(t *testing.T) {
	// A channel notification with empty Sender (system broadcast)
	// emits a frame whose `talker` field is omitted via omitempty
	// in the marshalled JSON. The struct field is still empty.
	a, fc := newCommChannelActor(t, "p-1")
	fc.setActive(true)

	sink := actorSink{a: a}
	_ = sink.Deliver(context.Background(), notifications.Notification{
		Kind:    "channel",
		Text:    "[admin] Server restart in 5 minutes.",
		Channel: "admin",
		// Sender intentionally empty
	})

	frames := commChannelFrames(t, fc)
	if len(frames) != 1 {
		t.Fatalf("system message emitted %d frames, want 1", len(frames))
	}
	if frames[0].Talker != "" {
		t.Errorf("Talker = %q, want empty for system message", frames[0].Talker)
	}
}

func TestActorSink_Deliver_NonGmcpConnIsSilentNoOp(t *testing.T) {
	// Conn doesn't implement gmcpSender — the GMCP send path must
	// early-return rather than panic.
	room := &world.Room{ID: "test-room", Name: "Test"}
	a := &connActor{
		id:       "test-p-1",
		conn:     &fakeConn{id: "test-p-1"}, // plain fakeConn, no GMCP
		playerID: "p-1",
		room:     room,
		vitals:   combat.NewVitalsAt(50, 100),
		save:     &player.Save{ID: "p-1", Name: "p-1"},
	}
	sink := actorSink{a: a}
	if err := sink.Deliver(context.Background(), notifications.Notification{
		Kind: "channel", Text: "x", Sender: "y", Channel: "ooc",
	}); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
}
