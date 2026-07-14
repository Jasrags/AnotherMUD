package biome

import (
	"context"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/world"
)

// fakeHazardSink records every Harm call and answers protection/occupancy
// from injected maps, so the service's decision flow is asserted without a
// real session or combat manager.
type fakeHazardSink struct {
	occupants map[world.RoomID][]string
	protected map[string]string         // victimID -> the immunity key they hold
	resist    map[string]map[string]int // victimID -> damageType -> soak
	harmed    []harmCall
}

type harmCall struct {
	victimID   string
	roomID     world.RoomID
	amount     int
	damageType string
	message    string
}

func (f *fakeHazardSink) OccupantsInRoom(roomID world.RoomID) []string {
	return f.occupants[roomID]
}

func (f *fakeHazardSink) HasProtection(victimID, protectionKey string) bool {
	return f.protected[victimID] == protectionKey
}

func (f *fakeHazardSink) Resistance(victimID, damageType string) int {
	return f.resist[victimID][damageType]
}

func (f *fakeHazardSink) Harm(_ context.Context, victimID string, roomID world.RoomID, amount int, damageType, message string) {
	f.harmed = append(f.harmed, harmCall{victimID, roomID, amount, damageType, message})
}

// hazardRegistry builds a registry with one pack biome carrying the given
// hazard, keyed by terrain "toxic".
func hazardRegistry(t *testing.T, h *Hazard) *Registry {
	t.Helper()
	r := NewRegistry()
	if err := r.RegisterPack("test", &Biome{ID: "toxic", Hazard: h}); err != nil {
		t.Fatalf("register toxic biome: %v", err)
	}
	return r
}

// roomLister is a trivial RoomLister over a fixed room slice.
type roomLister []*world.Room

func (r roomLister) Rooms() []*world.Room { return []*world.Room(r) }

func TestHazardService_HarmsUnprotectedOccupants(t *testing.T) {
	reg := hazardRegistry(t, &Hazard{Damage: 4, DamageType: "radiation", ProtectionKey: "rad-shielded", Message: "It sears."})
	rooms := roomLister{{ID: "glow", Terrain: "toxic"}}
	sink := &fakeHazardSink{
		occupants: map[world.RoomID][]string{"glow": {"runner", "shielded-runner"}},
		protected: map[string]string{"shielded-runner": "rad-shielded"},
	}
	svc := NewHazardService(reg, rooms, sink)

	svc.Tick(context.Background())

	if len(sink.harmed) != 1 {
		t.Fatalf("want 1 harm (only the unprotected runner), got %d: %+v", len(sink.harmed), sink.harmed)
	}
	got := sink.harmed[0]
	if got.victimID != "runner" {
		t.Errorf("harmed victim = %q, want the unprotected runner", got.victimID)
	}
	if got.amount != 4 || got.damageType != "radiation" || got.roomID != "glow" || got.message != "It sears." {
		t.Errorf("payload mismatch: %+v", got)
	}
}

func TestHazardService_ResistanceMitigatesPayload(t *testing.T) {
	reg := hazardRegistry(t, &Hazard{Damage: 4, DamageType: "radiation", Message: "It sears."})
	rooms := roomLister{{ID: "glow", Terrain: "toxic"}}
	sink := &fakeHazardSink{
		occupants: map[world.RoomID][]string{"glow": {"bare", "shielded", "sealed"}},
		// "shielded" soaks 3 of 4 radiation (partial); "sealed" soaks 4+ (full).
		resist: map[string]map[string]int{
			"shielded": {"radiation": 3},
			"sealed":   {"radiation": 5},
		},
	}
	svc := NewHazardService(reg, rooms, sink)

	svc.Tick(context.Background())

	// bare takes 4, shielded takes 1, sealed takes nothing (fully absorbed).
	if len(sink.harmed) != 2 {
		t.Fatalf("want 2 harms (sealed fully soaks), got %d: %+v", len(sink.harmed), sink.harmed)
	}
	got := map[string]int{}
	for _, h := range sink.harmed {
		got[h.victimID] = h.amount
	}
	if got["bare"] != 4 {
		t.Errorf("bare amount = %d, want 4 (no resistance)", got["bare"])
	}
	if got["shielded"] != 1 {
		t.Errorf("shielded amount = %d, want 1 (4 - 3 soak)", got["shielded"])
	}
	if _, harmed := got["sealed"]; harmed {
		t.Error("sealed should take no damage (resistance >= payload)")
	}
}

func TestHazardService_ResistanceIgnoredWhenHazardUntyped(t *testing.T) {
	// A hazard with no damage type takes no resistance — resistance is per-type.
	reg := hazardRegistry(t, &Hazard{Damage: 3, Message: "Fumes."})
	rooms := roomLister{{ID: "ash", Terrain: "toxic"}}
	sink := &fakeHazardSink{
		occupants: map[world.RoomID][]string{"ash": {"runner"}},
		resist:    map[string]map[string]int{"runner": {"radiation": 99}},
	}
	svc := NewHazardService(reg, rooms, sink)

	svc.Tick(context.Background())

	if len(sink.harmed) != 1 || sink.harmed[0].amount != 3 {
		t.Fatalf("untyped hazard should ignore resistance and deal 3, got %+v", sink.harmed)
	}
}

func TestHazardService_NoProtectionKeyHarmsEveryone(t *testing.T) {
	reg := hazardRegistry(t, &Hazard{Damage: 2, Message: "Toxic."})
	rooms := roomLister{{ID: "ash", Terrain: "toxic"}}
	sink := &fakeHazardSink{
		occupants: map[world.RoomID][]string{"ash": {"a", "b"}},
		// "b" holds a key, but the hazard names none, so it must not exempt.
		protected: map[string]string{"b": "rad-shielded"},
	}
	svc := NewHazardService(reg, rooms, sink)

	svc.Tick(context.Background())

	if len(sink.harmed) != 2 {
		t.Fatalf("want 2 harms (no gate exempts anyone), got %d", len(sink.harmed))
	}
}

func TestHazardService_SkipsHarmlessAndInertBiomes(t *testing.T) {
	reg := NewRegistry()
	// A biome with no hazard, and one with a zero-damage (inert) hazard.
	if err := reg.RegisterPack("test", &Biome{ID: "sprawl"}); err != nil {
		t.Fatal(err)
	}
	if err := reg.RegisterPack("test", &Biome{ID: "fizzle", Hazard: &Hazard{Damage: 0, Message: "nothing"}}); err != nil {
		t.Fatal(err)
	}
	rooms := roomLister{{ID: "street", Terrain: "sprawl"}, {ID: "dud", Terrain: "fizzle"}}
	sink := &fakeHazardSink{occupants: map[world.RoomID][]string{"street": {"x"}, "dud": {"y"}}}
	svc := NewHazardService(reg, rooms, sink)

	svc.Tick(context.Background())

	if len(sink.harmed) != 0 {
		t.Fatalf("harmless + inert biomes must harm no one, got %d: %+v", len(sink.harmed), sink.harmed)
	}
}

func TestHazardService_UnregisteredTerrainIsHarmless(t *testing.T) {
	reg := hazardRegistry(t, &Hazard{Damage: 5})
	// A room whose terrain has no registered biome resolves to nothing.
	rooms := roomLister{{ID: "void", Terrain: "no-such-biome"}}
	sink := &fakeHazardSink{occupants: map[world.RoomID][]string{"void": {"lost"}}}
	svc := NewHazardService(reg, rooms, sink)

	svc.Tick(context.Background())

	if len(sink.harmed) != 0 {
		t.Fatalf("unregistered terrain must be harmless, got %d", len(sink.harmed))
	}
}

func TestHazardService_NilDepsAreNoOp(t *testing.T) {
	// Must not panic with any nil dependency.
	(*HazardService)(nil).Tick(context.Background())
	NewHazardService(nil, nil, nil).Tick(context.Background())
}

func TestDecode_Hazard(t *testing.T) {
	const y = `
id: toxic
name: the toxic zone
hazard:
  damage: 6
  damage_type: radiation
  protection_key: rad-shielded
  message: The air sears your lungs.
`
	b, err := Decode([]byte(y))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !b.Hazard.Active() {
		t.Fatal("hazard should be active")
	}
	if b.Hazard.Damage != 6 || b.Hazard.DamageType != "radiation" ||
		b.Hazard.ProtectionKey != "rad-shielded" || b.Hazard.Message != "The air sears your lungs." {
		t.Errorf("decoded hazard mismatch: %+v", b.Hazard)
	}
}

func TestDecode_NoHazardIsNil(t *testing.T) {
	b, err := Decode([]byte("id: sprawl\nname: the sprawl\n"))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if b.Hazard != nil {
		t.Errorf("a biome with no hazard block must decode to a nil Hazard, got %+v", b.Hazard)
	}
	if b.Hazard.Active() {
		t.Error("nil hazard must be inert")
	}
}
