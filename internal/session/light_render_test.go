package session

import (
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/gameclock"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/light"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// These exercise the session render path end to end (the same helper the
// login spawn + link-dead reconnect use) to prove the light resolver is
// actually consulted there, not just in command unit tests.

func TestRenderRoomForReconnect_BlackVault(t *testing.T) {
	vault := &world.Room{ID: "x:vault", Name: "Forge Vault", Description: "Coins in the dark.",
		Terrain: world.TerrainUnderground}
	a, _ := newFakeActor("c1", "p1", "acc1", "Human", vault)
	cfg := Config{
		Items:     entities.NewStore(),
		Placement: entities.NewPlacement(),
		Light:     light.NewResolver(light.DefaultConfig(), fixedClockPeriod(gameclock.PeriodDay)),
	}

	out := renderRoomForReconnect(a, cfg)
	if !strings.Contains(out, "can see nothing") {
		t.Fatalf("underground vault should render black:\n%s", out)
	}
	if strings.Contains(out, "Coins in the dark") || strings.Contains(out, "Forge Vault") {
		t.Fatalf("black render leaked room detail:\n%s", out)
	}
}

func TestRenderRoomForReconnect_TorchLightsTheVault(t *testing.T) {
	vault := &world.Room{ID: "x:vault", Name: "Forge Vault", Description: "Coins glint here.",
		Terrain: world.TerrainUnderground}
	store := entities.NewStore()
	a, _ := newFakeActor("c1", "p1", "acc1", "Human", vault)
	cfg := Config{
		Items:     store,
		Placement: entities.NewPlacement(),
		Light:     light.NewResolver(light.DefaultConfig(), fixedClockPeriod(gameclock.PeriodDay)),
	}

	// A lit torch in the light slot lifts the vault to gloom — the room
	// renders its obscured (not suppressed) form, naming the room.
	torch, err := store.Spawn(&item.Template{
		ID: "x:torch", Name: "a torch", Type: "light",
		Properties: map[string]any{"light": "gloom"},
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	torch.SetProperty(light.PropItemLit, true)
	// Place the lit torch directly in the light slot (white-box: avoid
	// the full Equip stat-block plumbing the fake actor lacks).
	a.equipment = map[string]entities.EntityID{"light": torch.ID()}

	out := renderRoomForReconnect(a, cfg)
	if strings.Contains(out, "can see nothing") {
		t.Fatalf("a lit torch should lift the vault above black:\n%s", out)
	}
	if !strings.Contains(out, "Forge Vault") {
		t.Fatalf("gloom render should still anchor the room name:\n%s", out)
	}
}

func TestRenderRoomForReconnect_DwarfSeesInTheDark(t *testing.T) {
	vault := &world.Room{ID: "x:vault", Name: "Forge Vault", Terrain: world.TerrainUnderground}
	a, _ := newFakeActor("c1", "p1", "acc1", "Dwarf", vault)
	// Give the actor the darkvision racial tag.
	a.racialTags = []string{light.DarkvisionFlag}
	cfg := Config{
		Items:     entities.NewStore(),
		Placement: entities.NewPlacement(),
		Light:     light.NewResolver(light.DefaultConfig(), fixedClockPeriod(gameclock.PeriodDay)),
	}

	out := renderRoomForReconnect(a, cfg)
	if strings.Contains(out, "can see nothing") {
		t.Fatalf("a darkvision viewer should not be fully blind underground:\n%s", out)
	}
	if !strings.Contains(out, "Forge Vault") {
		t.Fatalf("darkvision (gloom floor) render should anchor the room name:\n%s", out)
	}
}
