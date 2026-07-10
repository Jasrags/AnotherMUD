package command

import (
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/progression"
)

// srAttrSet is a minimal Shadowrun-shaped attribute set — enough for the
// label path to resolve "reaction" → "Reaction" the way the score sheet does.
func srAttrSet() *progression.AttributeSet {
	return &progression.AttributeSet{
		ID: "shadowrun-primaries",
		Attributes: []progression.Attribute{
			{ID: "body", Name: "Body", Abbrev: "BOD"},
			{ID: "reaction", Name: "Reaction", Abbrev: "REA"},
			{ID: "strength", Name: "Strength", Abbrev: "STR"},
			{ID: "intuition", Name: "Intuition", Abbrev: "INT"},
		},
	}
}

// spawnInstance builds a live ItemInstance from tpl via a throwaway store —
// itemEffectSummary reads the instance's applied modifier list + armor bonus.
func spawnInstance(t *testing.T, tpl *item.Template) *entities.ItemInstance {
	t.Helper()
	store := entities.NewStore()
	inst, err := store.Spawn(tpl)
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	return inst
}

func TestItemEffectSummary_ModifiersUseAttributeSetLabels(t *testing.T) {
	// wired reflexes: a single attribute modifier resolves to the set's Name.
	inst := spawnInstance(t, &item.Template{
		ID: "sr:wired-reflexes", Name: "wired reflexes", Type: "item",
		Modifiers: []item.Modifier{{Stat: "reaction", Value: 2}},
	})
	if got, want := itemEffectSummary(srAttrSet(), inst), "+2 Reaction"; got != want {
		t.Errorf("summary = %q, want %q", got, want)
	}
}

func TestItemEffectSummary_MultipleModifiersJoined(t *testing.T) {
	// muscle replacement: two modifiers, joined in declared order.
	inst := spawnInstance(t, &item.Template{
		ID: "sr:muscle", Name: "muscle replacement", Type: "item",
		Modifiers: []item.Modifier{{Stat: "strength", Value: 1}, {Stat: "body", Value: 1}},
	})
	if got, want := itemEffectSummary(srAttrSet(), inst), "+1 Strength, +1 Body"; got != want {
		t.Errorf("summary = %q, want %q", got, want)
	}
}

func TestItemEffectSummary_ArmorBonusRendered(t *testing.T) {
	// armored jacket: structured armor bonus, no sign, after any modifiers.
	inst := spawnInstance(t, &item.Template{
		ID: "sr:jacket", Name: "an armored jacket", Type: "item",
		ArmorBonus: 3,
	})
	if got, want := itemEffectSummary(srAttrSet(), inst), "Armor 3"; got != want {
		t.Errorf("summary = %q, want %q", got, want)
	}
}

func TestItemEffectSummary_UnknownStatFallsBackToHumanizedKey(t *testing.T) {
	// A stat outside the attribute set (an engine vital) still reads cleanly:
	// "hit_mod" → "Hit mod" (the shared titleCase helper capitalizes the first
	// word). A nil set forces the same fallback path.
	inst := spawnInstance(t, &item.Template{
		ID: "sr:smartgun", Name: "a smartgun", Type: "item",
		Modifiers: []item.Modifier{{Stat: "hit_mod", Value: 1}},
	})
	if got, want := itemEffectSummary(nil, inst), "+1 Hit mod"; got != want {
		t.Errorf("summary = %q, want %q", got, want)
	}
}

func TestItemEffectSummary_NoMechanicsYieldsEmpty(t *testing.T) {
	// A plain item (no modifiers, no armor) gets no tail — the worn line stays
	// clean rather than showing an empty "()".
	inst := spawnInstance(t, &item.Template{
		ID: "sr:trophy", Name: "a chrome trophy", Type: "item",
	})
	if got := itemEffectSummary(srAttrSet(), inst); got != "" {
		t.Errorf("summary = %q, want empty", got)
	}
}

func TestWeaponAmmoState_HolderFedLoaded(t *testing.T) {
	// A holder-fed pistol with a clip inserted reports the clip's rounds.
	inst := spawnInstance(t, &item.Template{
		ID: "sr:predator", Name: "an Ares Predator V", Type: "weapon",
		AcceptsHolder: "heavy-pistol",
	})
	inst.SetInsertedHolder("sr:predator-clip", 15, "")
	label, loaded := weaponAmmoState(inst)
	if label != "15 rds" || !loaded {
		t.Errorf("state = (%q, %v), want (\"15 rds\", true)", label, loaded)
	}
}

func TestWeaponAmmoState_HolderFedEmpty(t *testing.T) {
	// No clip inserted → "empty", uncolored-loaded false. This is jasrags's
	// actual state: gun equipped, clips loose in inventory.
	inst := spawnInstance(t, &item.Template{
		ID: "sr:predator", Name: "an Ares Predator V", Type: "weapon",
		AcceptsHolder: "heavy-pistol",
	})
	label, loaded := weaponAmmoState(inst)
	if label != "empty" || loaded {
		t.Errorf("state = (%q, %v), want (\"empty\", false)", label, loaded)
	}
}

func TestWeaponAmmoState_MagazineShowsLoadedOverCapacity(t *testing.T) {
	inst := spawnInstance(t, &item.Template{
		ID: "sr:revolver", Name: "a revolver", Type: "weapon", Magazine: 6,
	})
	inst.SetMagazineLoaded(4)
	label, loaded := weaponAmmoState(inst)
	if label != "4/6 rds" || !loaded {
		t.Errorf("state = (%q, %v), want (\"4/6 rds\", true)", label, loaded)
	}
}

func TestWeaponAmmoState_NonFirearmSilent(t *testing.T) {
	// A melee weapon (no holder, no magazine) reports nothing — the worn line
	// carries no ammo tag for anything `reload` doesn't apply to.
	inst := spawnInstance(t, &item.Template{
		ID: "sr:katana", Name: "a katana", Type: "weapon", WeaponDamage: "2d6",
	})
	if label, loaded := weaponAmmoState(inst); label != "" || loaded {
		t.Errorf("state = (%q, %v), want empty", label, loaded)
	}
}

func TestRenderEquipRows_EffectAndAmmoTags(t *testing.T) {
	// The worn view renders the effect tail in <subtle>(...)</subtle> and the
	// firearm load state in a color tag keyed on AmmoLoaded: green when there
	// are rounds, red when empty. Both are absent on rows that don't set them.
	rows := []equipRow{
		{Slot: "wield", Name: "<item>an Ares Predator V</item>", Ammo: "15 rds", AmmoLoaded: true},
		{Slot: "cyberware", Name: "<item>wired reflexes</item>", Effect: "+2 Reaction"},
		{Slot: "cloak", Name: "<subtle>(empty)</subtle>"},
	}
	got := renderEquipRows("You are wearing:", rows)
	for _, want := range []string{
		"<good>[15 rds]</good>",          // loaded firearm → green
		"<subtle>(+2 Reaction)</subtle>", // effect tail
	} {
		if !strings.Contains(got, want) {
			t.Errorf("render missing %q\n--- got ---\n%s", want, got)
		}
	}
	// The empty cloak row carries neither tag.
	if strings.Contains(got, "[]") || strings.Contains(got, "()") {
		t.Errorf("empty row should carry no tag, got:\n%s", got)
	}
}

func TestRenderEquipRows_EmptyFirearmTagIsDanger(t *testing.T) {
	rows := []equipRow{{Slot: "wield", Name: "<item>an Ares Predator V</item>", Ammo: "empty", AmmoLoaded: false}}
	got := renderEquipRows("You are wearing:", rows)
	if !strings.Contains(got, "<danger>[empty]</danger>") {
		t.Errorf("empty firearm should render a danger tag, got:\n%s", got)
	}
}
