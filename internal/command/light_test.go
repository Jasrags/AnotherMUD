package command_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/light"
)

// torchTpl is a fuel-burning light source (gloom-level, 100 fuel).
func torchTpl() *item.Template {
	return &item.Template{
		ID:         "tapestry-core:torch",
		Name:       "a torch",
		Type:       "light",
		Keywords:   []string{"torch"},
		Properties: map[string]any{"slot": "light", "light": "gloom", "fuel": 100},
	}
}

// everlampTpl is a permanent source (no fuel property).
func everlampTpl() *item.Template {
	return &item.Template{
		ID:         "tapestry-core:everlamp",
		Name:       "an everburning lamp",
		Type:       "light",
		Keywords:   []string{"lamp", "everlamp"},
		Properties: map[string]any{"light": "dim"},
	}
}

func spentTorchTpl() *item.Template {
	return &item.Template{
		ID:         "tapestry-core:spent-torch",
		Name:       "a guttered torch",
		Type:       "light",
		Keywords:   []string{"torch", "guttered"},
		Properties: map[string]any{"light": "gloom", "fuel": 0},
	}
}

func TestLight_IgnitesSource(t *testing.T) {
	f := newEqFixture(t)
	a := newTestActor(f.room)
	torch := f.spawnInInventory(t, torchTpl(), a)
	r := newRegistry(t)

	dispatch(t, r, f.env(), a, "light torch")

	if !light.IsLit(torch) {
		t.Fatal("torch not lit after `light torch`")
	}
	if got := a.lastLine(); !strings.Contains(got, "You light") {
		t.Fatalf("last line = %q, want a `You light` confirmation", got)
	}
}

func TestLight_AlreadyLit(t *testing.T) {
	f := newEqFixture(t)
	a := newTestActor(f.room)
	f.spawnInInventory(t, torchTpl(), a)
	r := newRegistry(t)

	dispatch(t, r, f.env(), a, "light torch")
	dispatch(t, r, f.env(), a, "light torch")
	if got := a.lastLine(); !strings.Contains(got, "already lit") {
		t.Fatalf("relight last line = %q, want `already lit`", got)
	}
}

func TestLight_NotASource(t *testing.T) {
	f := newEqFixture(t)
	a := newTestActor(f.room)
	f.spawnInInventory(t, sword(), a)
	r := newRegistry(t)

	dispatch(t, r, f.env(), a, "light sword")
	if got := a.lastLine(); !strings.Contains(got, "not a light source") {
		t.Fatalf("last line = %q, want `not a light source`", got)
	}
}

func TestLight_SpentFuelCannotRelight(t *testing.T) {
	f := newEqFixture(t)
	a := newTestActor(f.room)
	torch := f.spawnInInventory(t, spentTorchTpl(), a)
	r := newRegistry(t)

	dispatch(t, r, f.env(), a, "light torch")
	if light.IsLit(torch) {
		t.Fatal("spent torch should not light")
	}
	if got := a.lastLine(); !strings.Contains(got, "spent") {
		t.Fatalf("last line = %q, want `spent`", got)
	}
}

func TestLight_PermanentSourceAlwaysLights(t *testing.T) {
	f := newEqFixture(t)
	a := newTestActor(f.room)
	lamp := f.spawnInInventory(t, everlampTpl(), a)
	r := newRegistry(t)

	dispatch(t, r, f.env(), a, "light lamp")
	if !light.IsLit(lamp) {
		t.Fatal("permanent source did not light")
	}
}

func TestExtinguish_PutsOut(t *testing.T) {
	f := newEqFixture(t)
	a := newTestActor(f.room)
	torch := f.spawnInInventory(t, torchTpl(), a)
	r := newRegistry(t)

	dispatch(t, r, f.env(), a, "light torch")
	dispatch(t, r, f.env(), a, "extinguish torch")
	if light.IsLit(torch) {
		t.Fatal("torch still lit after extinguish")
	}
	if got := a.lastLine(); !strings.Contains(got, "You extinguish") {
		t.Fatalf("last line = %q, want `You extinguish`", got)
	}
}

func TestExtinguish_NotLit(t *testing.T) {
	f := newEqFixture(t)
	a := newTestActor(f.room)
	f.spawnInInventory(t, torchTpl(), a)
	r := newRegistry(t)

	dispatch(t, r, f.env(), a, "extinguish torch")
	if got := a.lastLine(); !strings.Contains(got, "not lit") {
		t.Fatalf("last line = %q, want `not lit`", got)
	}
}

func TestLight_ResolvesEquippedSource(t *testing.T) {
	// A torch already worn in the light slot is still lightable.
	f := newEqFixture(t)
	a := newTestActor(f.room)
	torch := f.spawnInInventory(t, torchTpl(), a)
	r := newRegistry(t)

	dispatch(t, r, f.env(), a, "equip torch light")
	dispatch(t, r, f.env(), a, "light torch")
	if !light.IsLit(torch) {
		t.Fatal("equipped torch not lit via `light torch`")
	}
}

func TestEquip_AutoLightOnEquip(t *testing.T) {
	f := newEqFixture(t)
	a := newTestActor(f.room)
	torch := f.spawnInInventory(t, torchTpl(), a)
	r := newRegistry(t)

	env := f.env()
	cfg := light.DefaultConfig()
	cfg.AutoLightOnEquip = true
	env.Light = light.NewResolver(cfg, nil)

	if err := r.Dispatch(context.Background(), env, a, "equip torch light"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if !light.IsLit(torch) {
		t.Fatal("auto-light-on-equip did not light the torch")
	}
	if got := a.lastLine(); !strings.Contains(got, "flares to life") {
		t.Fatalf("last line = %q, want `flares to life`", got)
	}
}

func TestEquip_NoAutoLightWhenOff(t *testing.T) {
	f := newEqFixture(t)
	a := newTestActor(f.room)
	torch := f.spawnInInventory(t, torchTpl(), a)
	r := newRegistry(t)

	env := f.env() // default config has AutoLightOnEquip=false; env.Light nil too
	env.Light = light.NewResolver(light.DefaultConfig(), nil)

	if err := r.Dispatch(context.Background(), env, a, "equip torch light"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if light.IsLit(torch) {
		t.Fatal("torch auto-lit despite AutoLightOnEquip=false")
	}
}
