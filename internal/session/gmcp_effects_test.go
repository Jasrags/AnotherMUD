package session

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/gmcp"
	"github.com/Jasrags/AnotherMUD/internal/player"
	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// newEffectsGmcpActor builds an actor wired to a real
// progression.EffectManager. The actor implements EffectTarget via
// its own AddModifiers/RemoveBySource methods (see session.go).
func newEffectsGmcpActor(t *testing.T, playerID string) (*connActor, *gmcpFakeConn, *progression.EffectManager) {
	t.Helper()
	fc := &gmcpFakeConn{fakeConn: fakeConn{id: "test-" + playerID}}
	room := &world.Room{ID: "test-room", Name: "Test"}
	a := &connActor{
		id:        fc.id,
		conn:      fc,
		playerID:  playerID,
		room:      room,
		vitals:    combat.NewVitalsAt(50, 100),
		save:      &player.Save{ID: playerID, Name: playerID, Sustenance: 100},
		statBlock: progression.NewWithBase(progression.DefaultPlayerBase()),
	}
	a.sustenance = 100
	mgr := progression.NewEffectManager(progression.TargetResolverFunc(func(id string) (progression.EffectTarget, bool) {
		if id == playerID {
			return a, true
		}
		return nil, false
	}), nil)
	a.effects = mgr
	return a, fc, mgr
}

func effectsFrames(t *testing.T, fc *gmcpFakeConn) []gmcp.CharEffectsList {
	t.Helper()
	raw := fc.framesSnapshot()
	out := make([]gmcp.CharEffectsList, 0, len(raw))
	for _, f := range raw {
		if f.pkg != gmcp.PackageCharEffects {
			continue
		}
		var e gmcp.CharEffectsList
		if err := json.Unmarshal(f.payload, &e); err != nil {
			t.Fatalf("payload unmarshal: %v (raw %s)", err, f.payload)
		}
		out = append(out, e)
	}
	return out
}

func TestFlushGmcpEffects_NoSendBeforeActivation(t *testing.T) {
	a, fc, _ := newEffectsGmcpActor(t, "p-1")
	a.flushGmcpEffects(context.Background())
	if got := len(fc.framesSnapshot()); got != 0 {
		t.Errorf("pre-activation emitted %d frames, want 0", got)
	}
}

func TestFlushGmcpEffects_FirstFlushSendsEmptyList(t *testing.T) {
	// Fresh actor with no active effects still emits the baseline
	// `[]` frame so the panel can clear stale rows from a previous
	// character.
	a, fc, _ := newEffectsGmcpActor(t, "p-1")
	fc.setActive(true)

	a.flushGmcpEffects(context.Background())
	frames := effectsFrames(t, fc)
	if len(frames) != 1 {
		t.Fatalf("first flush emitted %d frames, want 1", len(frames))
	}
	if frames[0].Effects == nil || len(frames[0].Effects) != 0 {
		t.Errorf("baseline list = %+v, want empty non-nil slice", frames[0].Effects)
	}
}

func TestFlushGmcpEffects_NoRedundantSendsWhenUnchanged(t *testing.T) {
	a, fc, _ := newEffectsGmcpActor(t, "p-1")
	fc.setActive(true)

	a.flushGmcpEffects(context.Background())
	a.flushGmcpEffects(context.Background())
	a.flushGmcpEffects(context.Background())

	if got := len(effectsFrames(t, fc)); got != 1 {
		t.Errorf("idempotent flushes emitted %d frames, want 1", got)
	}
}

func TestFlushGmcpEffects_SendsOnEffectApplied(t *testing.T) {
	a, fc, mgr := newEffectsGmcpActor(t, "p-1")
	fc.setActive(true)

	a.flushGmcpEffects(context.Background())

	if !mgr.Apply(context.Background(), "p-1", progression.EffectTemplate{
		ID:       "bless",
		Duration: 60,
		Flags:    []string{"buff"},
	}, "src-1", "ability:bless") {
		t.Fatalf("Apply returned false")
	}

	a.flushGmcpEffects(context.Background())
	frames := effectsFrames(t, fc)
	if len(frames) != 2 {
		t.Fatalf("post-apply flushes emitted %d frames, want 2", len(frames))
	}
	got := frames[1].Effects
	if len(got) != 1 || got[0].ID != "bless" || got[0].Remaining != 60 || got[0].Source != "ability:bless" {
		t.Errorf("apply frame = %+v, want bless/60/ability:bless", got)
	}
	if len(got[0].Flags) != 1 || got[0].Flags[0] != "buff" {
		t.Errorf("flags = %v, want [buff]", got[0].Flags)
	}
	if got[0].Permanent {
		t.Errorf("Permanent = true on a duration-60 effect")
	}
}

func TestFlushGmcpEffects_PermanentEffectSetsFlag(t *testing.T) {
	a, fc, mgr := newEffectsGmcpActor(t, "p-1")
	fc.setActive(true)

	if !mgr.Apply(context.Background(), "p-1", progression.EffectTemplate{
		ID:       "blessed-by-the-light",
		Duration: -1, // permanent
	}, "", "") {
		t.Fatalf("Apply returned false")
	}

	a.flushGmcpEffects(context.Background())
	frames := effectsFrames(t, fc)
	if len(frames) != 1 {
		t.Fatalf("flush emitted %d frames, want 1", len(frames))
	}
	got := frames[0].Effects
	if len(got) != 1 || !got[0].Permanent || got[0].Remaining != 0 {
		t.Errorf("permanent frame = %+v, want permanent=true remaining=0", got)
	}
}

func TestFlushGmcpEffects_SendsOnRemainingChange(t *testing.T) {
	a, fc, mgr := newEffectsGmcpActor(t, "p-1")
	fc.setActive(true)

	mgr.Apply(context.Background(), "p-1", progression.EffectTemplate{
		ID: "bless", Duration: 60,
	}, "", "")
	a.flushGmcpEffects(context.Background())

	// Tick the manager once — the bless effect should now have
	// Remaining=59, a real change against the last-sent shadow.
	mgr.Tick(context.Background())

	a.flushGmcpEffects(context.Background())
	frames := effectsFrames(t, fc)
	if len(frames) != 2 {
		t.Fatalf("after-tick flushes emitted %d frames, want 2", len(frames))
	}
	if frames[1].Effects[0].Remaining != 59 {
		t.Errorf("Remaining after one tick = %d, want 59", frames[1].Effects[0].Remaining)
	}
}

func TestFlushGmcpEffects_SendsOnEffectRemoved(t *testing.T) {
	a, fc, mgr := newEffectsGmcpActor(t, "p-1")
	fc.setActive(true)

	mgr.Apply(context.Background(), "p-1", progression.EffectTemplate{
		ID: "bless", Duration: 60,
	}, "", "")
	a.flushGmcpEffects(context.Background())

	mgr.RemoveByID(context.Background(), "p-1", "bless")
	a.flushGmcpEffects(context.Background())

	frames := effectsFrames(t, fc)
	if len(frames) != 2 {
		t.Fatalf("after-remove flushes emitted %d frames, want 2", len(frames))
	}
	if len(frames[1].Effects) != 0 {
		t.Errorf("post-remove list = %+v, want empty", frames[1].Effects)
	}
}

func TestFlushGmcpEffects_NonGmcpConnIsSilentNoOp(t *testing.T) {
	// Underlying conn doesn't implement gmcpSender — the flusher
	// must early-return rather than panic on the type assertion.
	room := &world.Room{ID: "test-room", Name: "Test"}
	a := &connActor{
		id:        "test-p-1",
		conn:      &fakeConn{id: "test-p-1"},
		playerID:  "p-1",
		room:      room,
		vitals:    combat.NewVitalsAt(50, 100),
		save:      &player.Save{ID: "p-1", Name: "p-1"},
		statBlock: progression.NewWithBase(progression.DefaultPlayerBase()),
		effects:   progression.NewEffectManager(nil, nil),
	}
	a.flushGmcpEffects(context.Background()) // no panic, no send
}

func TestFlushGmcpEffects_NilEffectManagerIsSilentNoOp(t *testing.T) {
	a, fc, _ := newEffectsGmcpActor(t, "p-1")
	a.effects = nil
	fc.setActive(true)
	a.flushGmcpEffects(context.Background())
	if got := len(fc.framesSnapshot()); got != 0 {
		t.Errorf("nil manager emitted %d frames, want 0", got)
	}
}

func TestManagerFlushGmcpEffects_FansOutToLiveActors(t *testing.T) {
	a, fc, _ := newEffectsGmcpActor(t, "p-1")
	fc.setActive(true)

	mgr := NewManager()
	mgr.Add(a)

	mgr.FlushGmcpEffects(context.Background())

	if got := len(effectsFrames(t, fc)); got != 1 {
		t.Errorf("manager flush emitted %d frames, want 1", got)
	}
}

func TestFlushGmcpEffects_ShadowResetForcesResend(t *testing.T) {
	a, fc, mgr := newEffectsGmcpActor(t, "p-1")
	fc.setActive(true)

	mgr.Apply(context.Background(), "p-1", progression.EffectTemplate{
		ID: "bless", Duration: 60,
	}, "", "")
	a.flushGmcpEffects(context.Background())
	a.flushGmcpEffects(context.Background()) // idempotent

	a.resetGmcpEffectsShadow()
	a.flushGmcpEffects(context.Background())

	if got := len(effectsFrames(t, fc)); got != 2 {
		t.Errorf("after reset frames = %d, want 2", got)
	}
}
