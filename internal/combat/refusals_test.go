package combat

import (
	"context"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/world"
)

// mapTagSource is the test-side TagSource: an explicit set of
// (room, tag) and (id, tag) pairs flagged true. Anything not in the
// set returns false. The two indexes are nested maps for ergonomic
// table-driven setup.
type mapTagSource struct {
	roomTags   map[world.RoomID]map[string]bool
	entityTags map[CombatantID]map[string]bool
}

func newMapTagSource() *mapTagSource {
	return &mapTagSource{
		roomTags:   map[world.RoomID]map[string]bool{},
		entityTags: map[CombatantID]map[string]bool{},
	}
}

func (t *mapTagSource) tagRoom(r world.RoomID, tag string) {
	if t.roomTags[r] == nil {
		t.roomTags[r] = map[string]bool{}
	}
	t.roomTags[r][tag] = true
}

func (t *mapTagSource) tagEntity(id CombatantID, tag string) {
	if t.entityTags[id] == nil {
		t.entityTags[id] = map[string]bool{}
	}
	t.entityTags[id][tag] = true
}

func (t *mapTagSource) RoomHasTag(r world.RoomID, tag string) bool {
	return t.roomTags[r][tag]
}

func (t *mapTagSource) EntityHasTag(id CombatantID, tag string) bool {
	return t.entityTags[id][tag]
}

func makeRigWithTags(t *testing.T, tags TagSource, cooldowns *FleeCooldowns, names ...string) (*Manager, *recordingSink, []CombatantID) {
	t.Helper()
	locator := MapLocator{}
	ids := make([]CombatantID, len(names))
	for i, n := range names {
		var id CombatantID
		if i%2 == 0 {
			id = NewMobCombatantID(n)
		} else {
			id = NewPlayerCombatantID(n)
		}
		ids[i] = id
		locator[id] = staticCombatant{id: id, name: n}
	}
	sink := &recordingSink{}
	mgr := NewManagerWith(ManagerConfig{
		Locator:   locator,
		Sink:      sink,
		Tags:      tags,
		Cooldowns: cooldowns,
	})
	return mgr, sink, ids
}

func TestEngageRefusesSafeRoomAttacker(t *testing.T) {
	tags := newMapTagSource()
	tags.tagRoom(testRoom, TagSafeRoom)
	mgr, sink, ids := makeRigWithTags(t, tags, nil, "a", "b")

	reason, ok := mgr.EngageWithReason(context.Background(), ids[0], ids[1], testRoom)
	if ok {
		t.Fatal("safe-room engage succeeded; want refusal")
	}
	if reason != EngageRefusalSafeRoom {
		t.Errorf("reason = %v, want EngageRefusalSafeRoom", reason)
	}
	if sink.engagedCount() != 0 {
		t.Errorf("Engagement event emitted on safe-room refusal")
	}
	if mgr.InCombat(ids[0]) || mgr.InCombat(ids[1]) {
		t.Error("state mutated on safe-room refusal")
	}
}

func TestEngageRefusesNoKillTarget(t *testing.T) {
	tags := newMapTagSource()
	mgr, sink, ids := makeRigWithTags(t, tags, nil, "a", "b")
	tags.tagEntity(ids[1], TagNoKill)

	reason, ok := mgr.EngageWithReason(context.Background(), ids[0], ids[1], testRoom)
	if ok {
		t.Fatal("no-kill engage succeeded; want refusal")
	}
	if reason != EngageRefusalNoKill {
		t.Errorf("reason = %v, want EngageRefusalNoKill", reason)
	}
	if sink.engagedCount() != 0 {
		t.Errorf("Engagement event emitted on no-kill refusal")
	}
}

func TestEngageRefusesFleeCooldownAttacker(t *testing.T) {
	cd := NewFleeCooldowns()
	cd.SetNow(100)
	mgr, sink, ids := makeRigWithTags(t, nil, cd, "a", "b")
	cd.Start(ids[0], 20) // expires at tick 120

	reason, ok := mgr.EngageWithReason(context.Background(), ids[0], ids[1], testRoom)
	if ok {
		t.Fatal("cooldown engage succeeded; want refusal")
	}
	if reason != EngageRefusalFleeCooldown {
		t.Errorf("reason = %v, want EngageRefusalFleeCooldown", reason)
	}
	if sink.engagedCount() != 0 {
		t.Error("Engagement event emitted on cooldown refusal")
	}
}

// §5.3: cooldown blocks engaging but NOT being engaged. The fleer's
// pursuer can still attack them.
func TestFleeCooldownIsAsymmetric(t *testing.T) {
	cd := NewFleeCooldowns()
	cd.SetNow(100)
	mgr, _, ids := makeRigWithTags(t, nil, cd, "fleer", "pursuer")
	cd.Start(ids[0], 20)

	// fleer→pursuer: refused.
	if _, ok := mgr.EngageWithReason(context.Background(), ids[0], ids[1], testRoom); ok {
		t.Fatal("fleer engaged pursuer during cooldown; want refusal")
	}
	// pursuer→fleer: allowed.
	if _, ok := mgr.EngageWithReason(context.Background(), ids[1], ids[0], testRoom); !ok {
		t.Fatal("pursuer→fleer engage refused; cooldown should not gate being-engaged")
	}
}

func TestFleeCooldownExpires(t *testing.T) {
	cd := NewFleeCooldowns()
	cd.SetNow(100)
	cd.Start("x:1", 20) // expires at 120
	if !cd.Active("x:1") {
		t.Fatal("Active false at tick 100 with cooldown to 120")
	}
	cd.SetNow(120)
	if cd.Active("x:1") {
		t.Error("Active true at tick 120; cooldown should expire at boundary")
	}
}

func TestFleeCooldownStartZeroIsNoop(t *testing.T) {
	cd := NewFleeCooldowns()
	cd.Start("x:1", 0)
	if cd.Active("x:1") {
		t.Error("zero-duration Start created an active cooldown")
	}
}

// SetNow must not go backwards even under racing callers.
func TestFleeCooldownSetNowMonotonic(t *testing.T) {
	cd := NewFleeCooldowns()
	cd.SetNow(50)
	cd.SetNow(10) // attempted regression — ignored.
	if got := cd.Now(); got != 50 {
		t.Errorf("Now() = %d, want 50 (regression ignored)", got)
	}
}

// Backwards-compatible: NewManager with nil tag source and nil
// cooldowns must still pass M7.2 baseline behaviors.
func TestNewManagerNilTagsAndCooldownsBehavesAsM72(t *testing.T) {
	mgr, _, ids := makeRig(t, "a", "b")
	reason, ok := mgr.EngageWithReason(context.Background(), ids[0], ids[1], testRoom)
	if !ok || reason != EngageRefusalNone {
		t.Fatalf("nil-deps engage failed: reason=%v ok=%v", reason, ok)
	}
}
