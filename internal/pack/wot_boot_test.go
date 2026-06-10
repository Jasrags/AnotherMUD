package pack

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/light"
	"github.com/Jasrags/AnotherMUD/internal/slot"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// TestLoad_WotPackSelectionBootSwap is the M0.5 proof: selecting the `wot`
// pack boots {tapestry-core, wot} via dependency closure — it pulls in the
// engine baseline but NOT the demo `starter-world` — and spawns into the WoT
// starter room. This is the whole point of the setting-unblock work: a
// different world boots in the demo's place by pack selection alone.
func TestLoad_WotPackSelectionBootSwap(t *testing.T) {
	root, err := filepath.Abs("../../content")
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	regs := NewRegistries()
	if err := RegisterEngineBaselineProperties(regs.Properties); err != nil {
		t.Fatalf("baseline properties: %v", err)
	}
	if err := slot.RegisterEngineBaseline(regs.Slots); err != nil {
		t.Fatalf("baseline slots: %v", err)
	}
	// Select only wot; the loader's dependency closure adds tapestry-core.
	if err := Load(context.Background(), root, []string{"wot"}, regs, nil, nil, nil); err != nil {
		t.Fatalf("Load wot: %v", err)
	}

	// The WoT starter room loaded and is the kind of room a character spawns in.
	if _, err := regs.World.Room("wot:the-green"); err != nil {
		t.Errorf("wot starter room not loaded: %v", err)
	}
	if _, err := regs.World.Area("wot:emonds-field"); err != nil {
		t.Errorf("wot area not loaded: %v", err)
	}

	// The engine baseline came along via the dependency closure.
	if !regs.Races.Has("human") {
		t.Error("baseline race 'human' not loaded (tapestry-core dependency not pulled in)")
	}
	if _, ok := regs.Channels.Get("tapestry-core:ooc"); !ok {
		t.Error("baseline ooc channel not loaded (tapestry-core dependency not pulled in)")
	}

	// The demo world was NOT selected — its rooms must be absent.
	if _, err := regs.World.Room(world.RoomID("starter-world:town-square")); err == nil {
		t.Error("starter-world:town-square loaded, but only wot+baseline were selected")
	}

	// Lamp-lit village: emonds-field's area `light_floor: dim` baked onto
	// the Green, so it resolves Dim (navigable, full render) at night —
	// while a westwood wilds room stays Gloom (description withheld, bring
	// a torch). This is the village/hamlet light split (light-and-darkness
	// §2.4 floor cascade).
	green, err := regs.World.Room("wot:the-green")
	if err != nil {
		t.Fatalf("the-green missing: %v", err)
	}
	if got, ok := green.PropertyString("light_floor"); !ok || got != "dim" {
		t.Errorf("the-green light_floor = (%q,%v), want (dim,true) baked from emonds-field", got, ok)
	}
	res := light.NewResolver(light.DefaultConfig(), fixedNight{})
	if got := res.Effective(green, light.Black, light.Black); got != light.Dim {
		t.Errorf("the-green at night = %v, want Dim (village lamps lift gloom)", got)
	}

	// Map POI derivation: the forge holds Haral the smith (shop + trainer)
	// so it resolves to a shop marker; the open Green has no fixture.
	if forge, err := regs.World.Room("wot:the-forge"); err == nil {
		if forge.POI != "shop" {
			t.Errorf("the-forge POI = %q, want shop (Haral is a shop+trainer NPC)", forge.POI)
		}
	}
	if green.POI != "" {
		t.Errorf("the-green POI = %q, want empty (no fixture)", green.POI)
	}
	// The Winespring Inn rest rooms (healing_rate, no vendor) read as inns.
	for _, id := range []world.RoomID{"wot:inn-common-room", "wot:inn-guestroom"} {
		if r, err := regs.World.Room(id); err == nil && r.POI != "inn" {
			t.Errorf("%s POI = %q, want inn (healing_rate set, no shop)", id, r.POI)
		}
	}
	if wild, err := regs.World.Room("wot:deep-westwood"); err == nil {
		if got := res.Effective(wild, light.Black, light.Black); got != light.Gloom {
			t.Errorf("deep-westwood at night = %v, want Gloom (wilds stay dark)", got)
		}
	}
}

// fixedNight is a light.PeriodSource pinned to night for the boot test.
type fixedNight struct{}

func (fixedNight) CurrentPeriod() string { return "night" }
