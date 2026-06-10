package pack

import (
	"context"
	"path/filepath"
	"testing"

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
	if _, err := regs.World.Room("wot:emonds-field-green"); err != nil {
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
}
