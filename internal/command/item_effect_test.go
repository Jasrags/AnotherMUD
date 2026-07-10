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

func TestWeaponAmmoState_HolderFedLoadedGraded(t *testing.T) {
	// A holder-fed pistol with an APDS clip reports count + the graded type.
	inst := spawnInstance(t, &item.Template{
		ID: "sr:predator", Name: "an Ares Predator V", Type: "weapon",
		AcceptsHolder: "heavy-pistol",
	})
	inst.SetInsertedHolder("sr:predator-clip", 15, "apds")
	count, typ, loaded := weaponAmmoState(inst)
	if count != "15 rds" || typ != "APDS" || !loaded {
		t.Errorf("state = (%q, %q, %v), want (\"15 rds\", \"APDS\", true)", count, typ, loaded)
	}
}

func TestWeaponAmmoState_HolderFedLoadedStandard(t *testing.T) {
	// An ungraded clip reads as "standard".
	inst := spawnInstance(t, &item.Template{
		ID: "sr:predator", Name: "an Ares Predator V", Type: "weapon",
		AcceptsHolder: "heavy-pistol",
	})
	inst.SetInsertedHolder("sr:predator-clip", 15, "")
	count, typ, loaded := weaponAmmoState(inst)
	if count != "15 rds" || typ != "standard" || !loaded {
		t.Errorf("state = (%q, %q, %v), want (\"15 rds\", \"standard\", true)", count, typ, loaded)
	}
}

func TestWeaponAmmoState_HolderFedEmpty(t *testing.T) {
	// No clip inserted → "empty", no type, not loaded. This is jasrags's actual
	// state: gun equipped, clips loose in inventory.
	inst := spawnInstance(t, &item.Template{
		ID: "sr:predator", Name: "an Ares Predator V", Type: "weapon",
		AcceptsHolder: "heavy-pistol",
	})
	count, typ, loaded := weaponAmmoState(inst)
	if count != "empty" || typ != "" || loaded {
		t.Errorf("state = (%q, %q, %v), want (\"empty\", \"\", false)", count, typ, loaded)
	}
}

func TestWeaponAmmoState_MagazineShowsLoadedOverCapacity(t *testing.T) {
	inst := spawnInstance(t, &item.Template{
		ID: "sr:revolver", Name: "a revolver", Type: "weapon", Magazine: 6,
	})
	inst.SetMagazineLoaded(4)
	count, typ, loaded := weaponAmmoState(inst)
	if count != "4/6 rds" || typ != "standard" || !loaded {
		t.Errorf("state = (%q, %q, %v), want (\"4/6 rds\", \"standard\", true)", count, typ, loaded)
	}
}

func TestWeaponAmmoState_NonFirearmSilent(t *testing.T) {
	// A melee weapon (no holder, no magazine) reports nothing — the worn line
	// carries no ammo tag for anything `reload` doesn't apply to.
	inst := spawnInstance(t, &item.Template{
		ID: "sr:katana", Name: "a katana", Type: "weapon", WeaponDamage: "2d6",
	})
	if count, typ, loaded := weaponAmmoState(inst); count != "" || typ != "" || loaded {
		t.Errorf("state = (%q, %q, %v), want all-empty", count, typ, loaded)
	}
}

func TestAmmoTypeLabel(t *testing.T) {
	if got := ammoTypeLabel("apds"); got != "APDS" {
		t.Errorf("apds → %q, want APDS", got)
	}
	if got := ammoTypeLabel(""); got != "standard" {
		t.Errorf("ungraded → %q, want standard", got)
	}
}

func TestAmmoTypeTag_SpecialPopsStandardSubtle(t *testing.T) {
	if got := ammoTypeTag("APDS"); got != "<highlight>[APDS]</highlight>" {
		t.Errorf("APDS tag = %q, want highlight", got)
	}
	if got := ammoTypeTag("standard"); got != "<subtle>[standard]</subtle>" {
		t.Errorf("standard tag = %q, want subtle", got)
	}
	if got := ammoTypeTag(""); got != "" {
		t.Errorf("empty label → %q, want empty", got)
	}
}

func TestRenderEquipRows_EffectAndAmmoTags(t *testing.T) {
	// The worn view renders the effect tail in <subtle>(...)</subtle> and the
	// firearm load state in a color tag keyed on AmmoLoaded: green when there
	// are rounds, red when empty. Both are absent on rows that don't set them.
	rows := []equipRow{
		{Slot: "wield", Name: "<item>an Ares Predator V</item>", Ammo: "15 rds", AmmoType: "APDS", AmmoLoaded: true},
		{Slot: "cyberware", Name: "<item>wired reflexes</item>", Effect: "+2 Reaction"},
		{Slot: "cloak", Name: "<subtle>(empty)</subtle>"},
	}
	got := renderEquipRows("You are wearing:", rows)
	for _, want := range []string{
		"<good>[15 rds]</good>",          // loaded firearm count → green
		"<highlight>[APDS]</highlight>",  // special ammo type → pops
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

func clipTpl() *item.Template {
	return &item.Template{
		ID: "sr:predator-clip", Name: "an Ares Predator V clip", Type: "item",
		HolderFits: "heavy-pistol", Magazine: 15,
	}
}

func TestAmmoHolderTags_LoadedShowsCountAndType(t *testing.T) {
	clip := spawnInstance(t, clipTpl())
	clip.SetMagazineLoaded(15)
	clip.SetHolderAmmoGrade("apds")
	got := ammoHolderTags(clip)
	for _, want := range []string{"<good>[15/15]</good>", "<highlight>[APDS]</highlight>"} {
		if !strings.Contains(got, want) {
			t.Errorf("clip tags %q missing %q", got, want)
		}
	}
}

func TestAmmoHolderTags_StandardIsSubtle(t *testing.T) {
	clip := spawnInstance(t, clipTpl())
	clip.SetMagazineLoaded(15) // ungraded rounds
	got := ammoHolderTags(clip)
	if !strings.Contains(got, "<subtle>[standard]</subtle>") {
		t.Errorf("ungraded clip should read standard/subtle, got %q", got)
	}
}

func TestAmmoHolderTags_EmptyClip(t *testing.T) {
	clip := spawnInstance(t, clipTpl()) // spawns empty (loaded 0)
	got := ammoHolderTags(clip)
	if got != "  <danger>[empty]</danger>" {
		t.Errorf("empty clip tags = %q, want a single danger [empty]", got)
	}
}

func TestAmmoHolderTags_NonHolderSilent(t *testing.T) {
	// An ordinary item (not a holder) gets no annotation.
	round := spawnInstance(t, &item.Template{ID: "sr:apds", Name: "an APDS round", Type: "item"})
	if got := ammoHolderTags(round); got != "" {
		t.Errorf("non-holder should have no tags, got %q", got)
	}
}
