package command_test

import (
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/gameclock"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/light"
	"github.com/Jasrags/AnotherMUD/internal/mob"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

func TestRenderRoom_BlackSuppressesEverything(t *testing.T) {
	f := newRenderFixture()
	f.placeItem(t, &item.Template{ID: "x:well", Name: "a stone well", Type: "fixture"})
	out := command.RenderRoom(f.room, f.place, f.store, nil, nil, nil, light.Black, nil, nil)

	if strings.Contains(out, "Town Square") {
		t.Errorf("black render leaked the room name:\n%s", out)
	}
	if strings.Contains(out, "cobblestone") {
		t.Errorf("black render leaked the description:\n%s", out)
	}
	if strings.Contains(out, "well") || strings.Contains(out, "Exits") {
		t.Errorf("black render leaked occupants/exits:\n%s", out)
	}
	if !strings.Contains(out, "can see nothing") {
		t.Errorf("black render missing the dark line:\n%s", out)
	}
}

func TestRenderRoom_GloomObscures(t *testing.T) {
	f := newRenderFixture()
	// A door on the north exit must NOT show its state at gloom.
	f.room.Exits[world.DirNorth] = world.Exit{
		Target: "tapestry-core:forge",
		Door:   &world.DoorState{Closed: true},
	}
	f.placeItem(t, &item.Template{ID: "x:well", Name: "a stone well", Type: "fixture"})
	f.placeMob(t, &mob.Template{ID: "x:rat", Name: "a giant rat", Keywords: []string{"rat"}})

	out := command.RenderRoom(f.room, f.place, f.store, nil, nil, nil, light.Gloom, nil, nil, "Bob")

	if !strings.Contains(out, "Town Square") {
		t.Errorf("gloom should still anchor the room name:\n%s", out)
	}
	if strings.Contains(out, "cobblestone") {
		t.Errorf("gloom leaked the room prose:\n%s", out)
	}
	if !strings.Contains(out, "too dark") {
		t.Errorf("gloom missing the terse dark line:\n%s", out)
	}
	// Occupant identities hidden: no real names, only coarse shapes.
	if strings.Contains(out, "giant rat") || strings.Contains(out, "Bob") {
		t.Errorf("gloom leaked occupant identity:\n%s", out)
	}
	if !strings.Contains(out, "You can make out:") {
		t.Errorf("gloom should report coarse presence:\n%s", out)
	}
	// Bare exits: the direction shows, the door state does not.
	if !strings.Contains(out, "<exit>north</exit>") {
		t.Errorf("gloom missing bare exit direction:\n%s", out)
	}
	if strings.Contains(out, "closed") {
		t.Errorf("gloom leaked door state:\n%s", out)
	}
	// Items are not made out at all in gloom.
	if strings.Contains(out, "well") {
		t.Errorf("gloom leaked an item:\n%s", out)
	}
}

func TestRenderRoom_GloomEmptyRoomNoExits(t *testing.T) {
	f := newRenderFixture()
	f.room.Exits = nil // no exits
	out := command.RenderRoom(f.room, f.place, f.store, nil, nil, nil, light.Gloom, nil, nil)
	if !strings.Contains(out, "<subtle>Exits:</subtle> none") {
		t.Errorf("gloom exitless room should report no exits:\n%s", out)
	}
	if strings.Contains(out, "You can make out:") {
		t.Errorf("empty gloom room should report no occupants:\n%s", out)
	}
}

func TestRenderRoom_DimIsFullButMuted(t *testing.T) {
	f := newRenderFixture()
	full := command.RenderRoom(f.room, f.place, f.store, nil, nil, nil, light.Lit, nil, nil)
	dim := command.RenderRoom(f.room, f.place, f.store, nil, nil, nil, light.Dim, nil, nil)

	// Dim keeps the full body (name, prose, exits) ...
	if !strings.Contains(dim, "Town Square") || !strings.Contains(dim, "cobblestone") {
		t.Errorf("dim dropped body content:\n%s", dim)
	}
	if !strings.Contains(dim, "<exit>north</exit>") {
		t.Errorf("dim dropped exits:\n%s", dim)
	}
	// ... but mutes the description with the {dim} attribute, so it
	// differs from the lit render.
	if dim == full {
		t.Error("dim render identical to lit; expected muted prose")
	}
	if !strings.Contains(dim, "{dim}") {
		t.Errorf("dim render missing the muted-prose attribute:\n%s", dim)
	}
}

func TestRenderRoom_LitUnchanged(t *testing.T) {
	f := newRenderFixture()
	f.placeItem(t, &item.Template{ID: "x:well", Name: "a stone well", Type: "fixture"})
	out := command.RenderRoom(f.room, f.place, f.store, nil, nil, nil, light.Lit, nil, nil)
	if !strings.Contains(out, "Town Square") || !strings.Contains(out, "cobblestone") {
		t.Errorf("lit render dropped body:\n%s", out)
	}
	if !strings.Contains(out, "You see here:") {
		t.Errorf("lit render dropped occupants:\n%s", out)
	}
}

// lightViewer is a stub satisfying command.LightViewer + HasTag +
// HasEquippedCapability for the EffectiveLight gather tests. `tags`
// models racial-flag sourcing (darkvision/thermographic/low-light); `caps`
// models gear-sourced vision modes (a cybereye enhancement's grants).
type lightViewer struct {
	equip      map[string]entities.EntityID
	darkvision bool
	tags       map[string]bool
	caps       map[string]bool
}

func (v lightViewer) Equipment() map[string]entities.EntityID { return v.equip }
func (v lightViewer) HasTag(tag string) bool {
	if v.darkvision && tag == light.DarkvisionFlag {
		return true
	}
	return v.tags[tag]
}
func (v lightViewer) HasEquippedCapability(key string) bool { return v.caps[key] }

func newLightResolver(period string) *light.Resolver {
	return light.NewResolver(light.DefaultConfig(), fixedPeriodSource(period))
}

type fixedPeriodSource string

func (f fixedPeriodSource) CurrentPeriod() string { return string(f) }

func TestEffectiveLight_NilResolverIsLit(t *testing.T) {
	f := newRenderFixture()
	if got := command.EffectiveLight(nil, f.room, lightViewer{}, f.store, f.place); got != light.Lit {
		t.Fatalf("nil resolver = %v, want Lit", got)
	}
}

func TestEffectiveLight_UndergroundDarkThenLitByHeldTorch(t *testing.T) {
	f := newRenderFixture()
	f.room.Terrain = world.TerrainUnderground
	res := newLightResolver(gameclock.PeriodDay)

	// No light: black underground.
	if got := command.EffectiveLight(res, f.room, lightViewer{}, f.store, f.place); got != light.Black {
		t.Fatalf("underground no light = %v, want Black", got)
	}

	// A lit torch in the light slot raises it.
	torch, err := f.store.Spawn(&item.Template{
		ID: "x:torch", Name: "a torch", Type: "light",
		Properties: map[string]any{"light": "gloom"},
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	torch.SetProperty(light.PropItemLit, true)
	v := lightViewer{equip: map[string]entities.EntityID{"light": torch.ID()}}
	if got := command.EffectiveLight(res, f.room, v, f.store, f.place); got != light.Gloom {
		t.Fatalf("underground + held torch = %v, want Gloom", got)
	}
}

func TestEffectiveLight_UnlitHeldTorchDoesNotLight(t *testing.T) {
	f := newRenderFixture()
	f.room.Terrain = world.TerrainUnderground
	res := newLightResolver(gameclock.PeriodDay)
	torch, _ := f.store.Spawn(&item.Template{
		ID: "x:torch", Name: "a torch", Type: "light",
		Properties: map[string]any{"light": "gloom"},
	})
	// not lit
	v := lightViewer{equip: map[string]entities.EntityID{"light": torch.ID()}}
	if got := command.EffectiveLight(res, f.room, v, f.store, f.place); got != light.Black {
		t.Fatalf("underground + unlit torch = %v, want Black", got)
	}
}

func TestEffectiveLight_RoomLuminousItemRaises(t *testing.T) {
	f := newRenderFixture()
	f.room.Terrain = world.TerrainUnderground
	res := newLightResolver(gameclock.PeriodDay)
	lamp := f.placeItem(t, &item.Template{
		ID: "x:lamp", Name: "a glowing lamp", Type: "light",
		Properties: map[string]any{"light": "dim"},
	})
	lamp.SetProperty(light.PropItemLit, true)
	if got := command.EffectiveLight(res, f.room, lightViewer{}, f.store, f.place); got != light.Dim {
		t.Fatalf("underground + room lamp = %v, want Dim", got)
	}
}

func TestEffectiveLight_DarkvisionFloor(t *testing.T) {
	f := newRenderFixture()
	f.room.Terrain = world.TerrainUnderground
	res := newLightResolver(gameclock.PeriodDay)
	v := lightViewer{darkvision: true}
	if got := command.EffectiveLight(res, f.room, v, f.store, f.place); got != light.Gloom {
		t.Fatalf("underground + darkvision = %v, want Gloom", got)
	}
}

// Vision modes (light-and-darkness §4). Thermographic is an unconditional
// floor (gear or racial), like darkvision; low-light is a conditional lift
// that only helps when the room already affords some light.

func TestEffectiveLight_ThermographicFromCyber(t *testing.T) {
	f := newRenderFixture()
	f.room.Terrain = world.TerrainUnderground
	res := newLightResolver(gameclock.PeriodDay)
	// A cybereye-granted thermographic mode floors a pitch-black room to Gloom,
	// exactly like racial darkvision.
	v := lightViewer{caps: map[string]bool{light.ThermographicFlag: true}}
	if got := command.EffectiveLight(res, f.room, v, f.store, f.place); got != light.Gloom {
		t.Fatalf("underground + thermographic = %v, want Gloom", got)
	}
}

func TestEffectiveLight_ThermographicFromRace(t *testing.T) {
	f := newRenderFixture()
	f.room.Terrain = world.TerrainUnderground
	res := newLightResolver(gameclock.PeriodDay)
	// A metatype racial tag sources the same floor as the cyber path.
	v := lightViewer{tags: map[string]bool{light.ThermographicFlag: true}}
	if got := command.EffectiveLight(res, f.room, v, f.store, f.place); got != light.Gloom {
		t.Fatalf("underground + racial thermographic = %v, want Gloom", got)
	}
}

func TestEffectiveLight_LowLightNoLiftInTotalDark(t *testing.T) {
	f := newRenderFixture()
	f.room.Terrain = world.TerrainUnderground
	res := newLightResolver(gameclock.PeriodDay)
	// Low-light amplifies existing light; a sealed pitch-black room offers none,
	// so the viewer stays blind (Black) — the SR distinction from thermographic.
	v := lightViewer{caps: map[string]bool{light.LowLightFlag: true}}
	if got := command.EffectiveLight(res, f.room, v, f.store, f.place); got != light.Black {
		t.Fatalf("underground + low-light (no ambient) = %v, want Black", got)
	}
}

func TestEffectiveLight_LowLightAmplifiesFaintLight(t *testing.T) {
	f := newRenderFixture()
	f.room.Terrain = world.TerrainUnderground
	res := newLightResolver(gameclock.PeriodDay)
	// A gloom-luminous item gives the room a faint glow (base Gloom); low-light
	// pulls it up to clear (Dim).
	lamp := f.placeItem(t, &item.Template{
		ID: "x:emberlamp", Name: "a dying ember", Type: "light",
		Properties: map[string]any{"light": "gloom"},
	})
	lamp.SetProperty(light.PropItemLit, true)
	// Baseline: a plain viewer sees only Gloom.
	if got := command.EffectiveLight(res, f.room, lightViewer{}, f.store, f.place); got != light.Gloom {
		t.Fatalf("underground + gloom ember (plain) = %v, want Gloom", got)
	}
	// Low-light lifts that faint glow to Dim.
	v := lightViewer{caps: map[string]bool{light.LowLightFlag: true}}
	if got := command.EffectiveLight(res, f.room, v, f.store, f.place); got != light.Dim {
		t.Fatalf("underground + gloom ember + low-light = %v, want Dim", got)
	}
}

func TestEffectiveLight_ThermographicPlusLowLightStaysGloomInDark(t *testing.T) {
	f := newRenderFixture()
	f.room.Terrain = world.TerrainUnderground
	res := newLightResolver(gameclock.PeriodDay)
	// A runner with BOTH cyber vision modes in a sealed pitch-black room:
	// thermographic floors to Gloom (heat shapes), but low-light finds no REAL
	// light to amplify (roomLight is Black), so it does NOT lift to Dim. Low-light
	// amplifies photons, not a thermographic floor.
	v := lightViewer{caps: map[string]bool{
		light.ThermographicFlag: true,
		light.LowLightFlag:      true,
	}}
	if got := command.EffectiveLight(res, f.room, v, f.store, f.place); got != light.Gloom {
		t.Fatalf("underground + thermographic + low-light (no real light) = %v, want Gloom", got)
	}
}

func TestEffectiveLight_LowLightAmplifiesHeldTorch(t *testing.T) {
	f := newRenderFixture()
	f.room.Terrain = world.TerrainUnderground
	res := newLightResolver(gameclock.PeriodDay)
	// A held torch is REAL light; low-light amplifies it from Gloom to Dim.
	torch, err := f.store.Spawn(&item.Template{
		ID: "x:torch2", Name: "a torch", Type: "light",
		Properties: map[string]any{"light": "gloom"},
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	torch.SetProperty(light.PropItemLit, true)
	v := lightViewer{
		equip: map[string]entities.EntityID{"light": torch.ID()},
		caps:  map[string]bool{light.LowLightFlag: true},
	}
	if got := command.EffectiveLight(res, f.room, v, f.store, f.place); got != light.Dim {
		t.Fatalf("underground + held torch + low-light = %v, want Dim", got)
	}
}

func TestDaylight_ReportsPeriodAndLight(t *testing.T) {
	store := entities.NewStore()
	place := entities.NewPlacement()
	cave := &world.Room{ID: "x:cave", Name: "Cave", Terrain: world.TerrainUnderground}
	a := newTestActor(cave)
	env := command.Env{Items: store, Placement: place, Light: newLightResolver(gameclock.PeriodNight)}

	r := newRegistry(t)
	dispatch(t, r, env, a, "daylight")
	got := a.lastLine()
	if !strings.Contains(got, "night") {
		t.Fatalf("daylight should report the period, got %q", got)
	}
	if !strings.Contains(got, "pitch black") {
		t.Fatalf("daylight in an unlit cave should report blackness, got %q", got)
	}
}

func TestDaylight_NilResolverSteady(t *testing.T) {
	a := newTestActor(&world.Room{ID: "x:road", Name: "Road"})
	r := newRegistry(t)
	dispatch(t, r, command.Env{}, a, "daylight")
	if got := a.lastLine(); !strings.Contains(got, "steady") {
		t.Fatalf("daylight with no resolver = %q, want the steady fallback", got)
	}
}
