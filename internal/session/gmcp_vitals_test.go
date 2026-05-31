package session

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/gmcp"
	"github.com/Jasrags/AnotherMUD/internal/player"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// gmcpFakeConn extends fakeConn with the GMCP sender interface so
// the flusher's type-assertion succeeds. Records every SendGmcp
// call as a (pkg, payload) tuple for assertions.
type gmcpFakeConn struct {
	fakeConn
	mu     sync.Mutex
	active bool
	frames []gmcpFrame
}

type gmcpFrame struct {
	pkg     string
	payload []byte
}

func (g *gmcpFakeConn) GmcpActive() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.active
}

func (g *gmcpFakeConn) SendGmcp(_ context.Context, pkg string, payload []byte) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	// Copy payload; flushGmcpVitals may reuse the json buffer.
	dup := append([]byte(nil), payload...)
	g.frames = append(g.frames, gmcpFrame{pkg: pkg, payload: dup})
	return nil
}

func (g *gmcpFakeConn) setActive(v bool) {
	g.mu.Lock()
	g.active = v
	g.mu.Unlock()
}

func (g *gmcpFakeConn) framesSnapshot() []gmcpFrame {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := make([]gmcpFrame, len(g.frames))
	copy(out, g.frames)
	return out
}

// newGmcpActor builds a connActor backed by a gmcpFakeConn with a
// realistic Vitals + sustenance starting state.
func newGmcpActor(playerID string, hp, maxHP int) (*connActor, *gmcpFakeConn) {
	fc := &gmcpFakeConn{fakeConn: fakeConn{id: "test-" + playerID}}
	room := &world.Room{ID: "test-room", Name: "Test"}
	a := &connActor{
		id:       fc.id,
		conn:     fc,
		playerID: playerID,
		room:     room,
		vitals:   combat.NewVitalsAt(hp, maxHP),
		save: &player.Save{
			ID:         playerID,
			Name:       playerID,
			Sustenance: 100,
		},
	}
	a.sustenance = 100
	return a, fc
}

func TestFlushGmcpVitals_NoSendBeforeActivation(t *testing.T) {
	a, fc := newGmcpActor("p-1", 50, 100)
	// active=false (default).
	a.flushGmcpVitals(context.Background())
	if frames := fc.framesSnapshot(); len(frames) != 0 {
		t.Errorf("pre-activation flush emitted %d frames, want 0", len(frames))
	}
}

func TestFlushGmcpVitals_SendsOnFirstActiveFlush(t *testing.T) {
	a, fc := newGmcpActor("p-1", 50, 100)
	fc.setActive(true)

	a.flushGmcpVitals(context.Background())
	frames := fc.framesSnapshot()
	if len(frames) != 1 {
		t.Fatalf("first flush emitted %d frames, want 1", len(frames))
	}
	if frames[0].pkg != gmcp.PackageCharVitals {
		t.Errorf("pkg = %q, want %q", frames[0].pkg, gmcp.PackageCharVitals)
	}
	var got gmcp.CharVitals
	if err := json.Unmarshal(frames[0].payload, &got); err != nil {
		t.Fatalf("payload unmarshal: %v", err)
	}
	if got.HP != 50 || got.MaxHP != 100 || got.Sustenance != 100 {
		t.Errorf("payload = %+v, want hp/maxhp/sust 50/100/100", got)
	}
}

func TestFlushGmcpVitals_NoRedundantSendsWhenUnchanged(t *testing.T) {
	// PD-3 contract: zero frames when nothing changed.
	a, fc := newGmcpActor("p-1", 50, 100)
	fc.setActive(true)

	a.flushGmcpVitals(context.Background()) // primes the shadow
	a.flushGmcpVitals(context.Background())
	a.flushGmcpVitals(context.Background())

	if frames := fc.framesSnapshot(); len(frames) != 1 {
		t.Errorf("three flushes with no change emitted %d frames, want 1", len(frames))
	}
}

func TestFlushGmcpVitals_SendsOnHPChange(t *testing.T) {
	a, fc := newGmcpActor("p-1", 50, 100)
	fc.setActive(true)
	a.flushGmcpVitals(context.Background()) // baseline

	a.vitals.ApplyDamage(10)
	a.flushGmcpVitals(context.Background())

	frames := fc.framesSnapshot()
	if len(frames) != 2 {
		t.Fatalf("post-damage flush count = %d, want 2", len(frames))
	}
	// Second frame should reflect the new HP.
	if !strings.Contains(string(frames[1].payload), `"hp":40`) {
		t.Errorf("post-damage payload = %s, want hp:40", frames[1].payload)
	}
}

func TestFlushGmcpVitals_SendsOnSustenanceChange(t *testing.T) {
	a, fc := newGmcpActor("p-1", 50, 100)
	fc.setActive(true)
	a.flushGmcpVitals(context.Background())

	a.SetSustenance(75)
	a.flushGmcpVitals(context.Background())

	frames := fc.framesSnapshot()
	if len(frames) != 2 {
		t.Fatalf("post-sustenance flush count = %d, want 2", len(frames))
	}
	if !strings.Contains(string(frames[1].payload), `"sustenance":75`) {
		t.Errorf("post-sustenance payload = %s, want sustenance:75", frames[1].payload)
	}
}

func TestFlushGmcpVitals_NonGmcpConnIsSilentNoOp(t *testing.T) {
	// A fakeConn (non-GMCP) actor: the flush type-assert fails
	// and returns silently.
	room := &world.Room{ID: "r", Name: "R"}
	a := &connActor{
		id:       "x",
		conn:     &fakeConn{id: "x"},
		playerID: "p-x",
		room:     room,
		vitals:   combat.NewVitalsAt(50, 100),
		save:     &player.Save{ID: "p-x", Sustenance: 100},
	}
	a.sustenance = 100
	// Should not panic, should not write.
	a.flushGmcpVitals(context.Background())
}

func TestManagerFlushGmcpVitals_FansOutToLiveActors(t *testing.T) {
	// End-to-end: two GMCP-active actors registered in a Manager.
	// One flush emits one frame to each.
	mgr := NewManager()
	a1, fc1 := newGmcpActor("p-1", 50, 100)
	a2, fc2 := newGmcpActor("p-2", 80, 80)
	fc1.setActive(true)
	fc2.setActive(true)
	mgr.Add(a1)
	mgr.Add(a2)

	mgr.FlushGmcpVitals(context.Background())

	if len(fc1.framesSnapshot()) != 1 {
		t.Errorf("a1 received %d frames, want 1", len(fc1.framesSnapshot()))
	}
	if len(fc2.framesSnapshot()) != 1 {
		t.Errorf("a2 received %d frames, want 1", len(fc2.framesSnapshot()))
	}
}
