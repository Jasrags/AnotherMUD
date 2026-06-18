package auction

import (
	"errors"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/item"
)

// daggerTemplate is a minimal weapon template for serialization tests.
func daggerTemplate() *item.Template {
	return &item.Template{
		ID:           "test:iron-dagger",
		Name:         "an iron dagger",
		Type:         "weapon",
		Keywords:     []string{"dagger", "iron"},
		WeaponDamage: "1d4",
	}
}

func newTemplates(t *testing.T, tpls ...*item.Template) *item.Templates {
	t.Helper()
	r := item.NewTemplates()
	for _, tpl := range tpls {
		r.Add(tpl)
	}
	return r
}

// TestSerialize_RoundTripPreservesPropertyBag is the de-risking test for the
// auction-house §4 requirement: the escrowed item returns "with its property
// bag + decorations, intact." A crafted item carries a grade override and a
// decoration (rarity) as instance properties; both must survive the
// serialize -> persist-shape -> rehydrate cycle, even into a fresh entity
// store with reassigned ids.
func TestSerialize_RoundTripPreservesPropertyBag(t *testing.T) {
	tpls := newTemplates(t, daggerTemplate())

	// Spawn a live instance and mutate it the way a craft / decoration would.
	src := entities.NewStore()
	inst, err := src.Spawn(daggerTemplate())
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	inst.SetProperty(entities.PropGrade, "power-wrought") // craft grade override
	inst.SetProperty("rarity", "rare")                    // a decoration (reserved property)
	inst.SetProperty("fill", 3)                           // arbitrary mutable state

	si := Serialize(inst)

	if si.Template != "test:iron-dagger" {
		t.Errorf("template = %q, want test:iron-dagger", si.Template)
	}
	if si.Name != "an iron dagger" {
		t.Errorf("name cache = %q", si.Name)
	}
	// Reserved keys must NOT be serialized.
	if _, ok := si.Properties[entities.PropTemplateID]; ok {
		t.Error("template_id leaked into serialized properties")
	}

	// Rehydrate into a DIFFERENT store (simulating a reboot) and assert the
	// per-instance state came back.
	dst := entities.NewStore()
	got, err := Rehydrate(dst, tpls, si)
	if err != nil {
		t.Fatalf("rehydrate: %v", err)
	}
	if got.Grade() != "power-wrought" {
		t.Errorf("grade = %q, want power-wrought", got.Grade())
	}
	if v, ok := got.Property("rarity"); !ok || v != "rare" {
		t.Errorf("rarity = %v (ok=%v), want rare", v, ok)
	}
	if v, ok := got.Property("fill"); !ok || v != 3 {
		t.Errorf("fill = %v (ok=%v), want 3", v, ok)
	}
	// Template-derived state (weapon dice, keywords) returns via Spawn.
	if _, isWeapon := got.WeaponDamage(); !isWeapon {
		t.Error("rehydrated item lost its weapon damage")
	}
	if got.TemplateID() != "test:iron-dagger" {
		t.Errorf("template id = %q", got.TemplateID())
	}
}

// TestRehydrate_TemplateGone surfaces a vanished template as ErrTemplateGone
// rather than silently dropping real player value.
func TestRehydrate_TemplateGone(t *testing.T) {
	dst := entities.NewStore()
	_, err := Rehydrate(dst, item.NewTemplates(), SerializedItem{Template: "test:missing"})
	if err == nil {
		t.Fatal("expected error for missing template")
	}
	if !errors.Is(err, ErrTemplateGone) {
		t.Errorf("err = %v, want ErrTemplateGone", err)
	}
}
