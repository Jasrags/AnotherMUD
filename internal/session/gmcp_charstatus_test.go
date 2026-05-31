package session

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/gmcp"
	"github.com/Jasrags/AnotherMUD/internal/player"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// newCharStatusActor builds an actor with the identity surface
// (raceID/classID/alignment/alignmentTag, account id) populated
// for the flusher under test.
func newCharStatusActor(t *testing.T, playerID, accountID string) (*connActor, *gmcpFakeConn) {
	t.Helper()
	fc := &gmcpFakeConn{fakeConn: fakeConn{id: "test-" + playerID}}
	room := &world.Room{ID: "test-room", Name: "Test"}
	a := &connActor{
		id:        fc.id,
		conn:      fc,
		playerID:  playerID,
		accountID: accountID,
		room:      room,
		vitals:    combat.NewVitalsAt(50, 100),
		save:      &player.Save{ID: playerID, Name: playerID, Sustenance: 100},
	}
	a.sustenance = 100
	return a, fc
}

func framesByPackage(fc *gmcpFakeConn, pkg string) [][]byte {
	raw := fc.framesSnapshot()
	out := make([][]byte, 0, len(raw))
	for _, f := range raw {
		if f.pkg == pkg {
			out = append(out, f.payload)
		}
	}
	return out
}

func TestFlushGmcpCharStatus_NoSendBeforeActivation(t *testing.T) {
	a, fc := newCharStatusActor(t, "p-1", "acc-1")
	a.flushGmcpCharStatus(context.Background())
	if got := len(fc.framesSnapshot()); got != 0 {
		t.Errorf("pre-activation emitted %d frames, want 0", got)
	}
}

func TestFlushGmcpCharStatus_FirstFlushSendsAllThree(t *testing.T) {
	// Char.Login + Char.StatusVars + Char.Status all ship on the
	// first GMCP-active flush.
	a, fc := newCharStatusActor(t, "p-1", "acc-1")
	a.save.Name = "Alice"
	fc.setActive(true)

	a.flushGmcpCharStatus(context.Background())

	if got := len(framesByPackage(fc, gmcp.PackageCharLogin)); got != 1 {
		t.Errorf("Char.Login frames = %d, want 1", got)
	}
	if got := len(framesByPackage(fc, gmcp.PackageCharStatusVars)); got != 1 {
		t.Errorf("Char.StatusVars frames = %d, want 1", got)
	}
	if got := len(framesByPackage(fc, gmcp.PackageCharStatus)); got != 1 {
		t.Errorf("Char.Status frames = %d, want 1", got)
	}

	// Login payload spot-check.
	var login gmcp.CharLogin
	_ = json.Unmarshal(framesByPackage(fc, gmcp.PackageCharLogin)[0], &login)
	if login.Name != "Alice" || login.Account != "acc-1" {
		t.Errorf("Char.Login payload = %+v", login)
	}

	// StatusVars catalogue spot-check.
	var vars gmcp.CharStatusVars
	_ = json.Unmarshal(framesByPackage(fc, gmcp.PackageCharStatusVars)[0], &vars)
	for _, key := range []string{"race", "class", "alignment", "alignment_tag"} {
		if _, ok := vars.Vars[key]; !ok {
			t.Errorf("catalogue missing %q: %+v", key, vars.Vars)
		}
	}
}

func TestFlushGmcpCharStatus_NoRedundantSendsWhenUnchanged(t *testing.T) {
	// Char.Login + Char.StatusVars never re-emit (sent flag);
	// Char.Status doesn't re-emit when nothing changed. So three
	// flushes still produce one frame of each.
	a, fc := newCharStatusActor(t, "p-1", "acc-1")
	fc.setActive(true)

	a.flushGmcpCharStatus(context.Background())
	a.flushGmcpCharStatus(context.Background())
	a.flushGmcpCharStatus(context.Background())

	for _, pkg := range []string{gmcp.PackageCharLogin, gmcp.PackageCharStatusVars, gmcp.PackageCharStatus} {
		if got := len(framesByPackage(fc, pkg)); got != 1 {
			t.Errorf("%s emitted %d frames after idempotent flushes, want 1", pkg, got)
		}
	}
}

func TestFlushGmcpCharStatus_StatusReemitsOnAlignmentShift(t *testing.T) {
	a, fc := newCharStatusActor(t, "p-1", "acc-1")
	fc.setActive(true)

	a.flushGmcpCharStatus(context.Background())

	a.SetAlignment(-100)
	a.SetAlignmentTag("evil")
	a.flushGmcpCharStatus(context.Background())

	statusFrames := framesByPackage(fc, gmcp.PackageCharStatus)
	if len(statusFrames) != 2 {
		t.Fatalf("Char.Status frames after shift = %d, want 2", len(statusFrames))
	}
	var got gmcp.CharStatus
	_ = json.Unmarshal(statusFrames[1], &got)
	if got.Alignment != -100 || got.AlignmentTag != "evil" {
		t.Errorf("post-shift Status = %+v", got)
	}
}

func TestFlushGmcpCharStatus_LoginAndVarsAreEmitOnce(t *testing.T) {
	// An alignment shift re-emits Char.Status but NOT Char.Login
	// or Char.StatusVars — those are identity packages that don't
	// change during a session.
	a, fc := newCharStatusActor(t, "p-1", "acc-1")
	fc.setActive(true)

	a.flushGmcpCharStatus(context.Background())
	a.SetAlignment(50)
	a.flushGmcpCharStatus(context.Background())
	a.SetAlignment(-50)
	a.flushGmcpCharStatus(context.Background())

	if got := len(framesByPackage(fc, gmcp.PackageCharLogin)); got != 1 {
		t.Errorf("Char.Login frames = %d, want 1 (emit-once)", got)
	}
	if got := len(framesByPackage(fc, gmcp.PackageCharStatusVars)); got != 1 {
		t.Errorf("Char.StatusVars frames = %d, want 1 (emit-once)", got)
	}
	if got := len(framesByPackage(fc, gmcp.PackageCharStatus)); got != 3 {
		t.Errorf("Char.Status frames = %d, want 3 (re-emit on diff)", got)
	}
}

func TestFlushGmcpCharStatus_NonGmcpConnIsSilentNoOp(t *testing.T) {
	room := &world.Room{ID: "test-room", Name: "Test"}
	a := &connActor{
		id:        "test-p-1",
		conn:      &fakeConn{id: "test-p-1"},
		playerID:  "p-1",
		accountID: "acc-1",
		room:      room,
		vitals:    combat.NewVitalsAt(50, 100),
		save:      &player.Save{ID: "p-1", Name: "p-1"},
	}
	a.flushGmcpCharStatus(context.Background()) // no panic, no send
}

func TestManagerFlushGmcpCharStatus_FansOutToLiveActors(t *testing.T) {
	a, fc := newCharStatusActor(t, "p-1", "acc-1")
	fc.setActive(true)

	mgr := NewManager()
	mgr.Add(a)

	mgr.FlushGmcpCharStatus(context.Background())

	// Each of the three packages emits once on the first
	// GMCP-active flush.
	for _, pkg := range []string{gmcp.PackageCharLogin, gmcp.PackageCharStatusVars, gmcp.PackageCharStatus} {
		if got := len(framesByPackage(fc, pkg)); got != 1 {
			t.Errorf("fan-out %s frames = %d, want 1", pkg, got)
		}
	}
}

func TestFlushGmcpCharStatus_ShadowResetReemitsAllThree(t *testing.T) {
	a, fc := newCharStatusActor(t, "p-1", "acc-1")
	fc.setActive(true)

	a.flushGmcpCharStatus(context.Background())
	a.flushGmcpCharStatus(context.Background()) // idempotent

	a.resetGmcpCharStatusShadow()
	a.flushGmcpCharStatus(context.Background())

	// After reset, all three packages emit a fresh baseline frame.
	for _, pkg := range []string{gmcp.PackageCharLogin, gmcp.PackageCharStatusVars, gmcp.PackageCharStatus} {
		if got := len(framesByPackage(fc, pkg)); got != 2 {
			t.Errorf("%s frames after reset = %d, want 2", pkg, got)
		}
	}
}

func TestCharStatusVarCatalogue_KeysMatchCharStatusFields(t *testing.T) {
	// The catalogue must declare every field the Char.Status
	// payload can carry, otherwise a client building its panel
	// from StatusVars would miss columns.
	wantKeys := []string{"race", "class", "alignment", "alignment_tag"}
	for _, key := range wantKeys {
		if _, ok := charStatusVarCatalogue[key]; !ok {
			t.Errorf("catalogue missing key %q", key)
		}
	}
}
