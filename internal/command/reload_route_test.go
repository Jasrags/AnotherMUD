package command

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/slot"
)

// namesWieldedWeapon is the routing guard that keeps `reload ares` from being
// read as "fill a carried clip named ares" — the firearm and its clip share
// the name-word "ares" ("an Ares Predator V" vs "an Ares Predator V clip"), so
// a weapon-naming token must route to a weapon reload, not a clip fill.

func predatorTpl() *item.Template {
	return &item.Template{
		ID: "sr:ares-predator-v", Name: "an Ares Predator V", Type: "weapon",
		Keywords:      []string{"predator", "ares predator", "ares", "pistol", "gun"},
		AcceptsHolder: "heavy-pistol",
	}
}

// wield spawns tpl into a fresh store and returns the store plus a wield-slot
// equipment map pointing at it — the two inputs namesWieldedWeapon reads.
func wield(t *testing.T, tpl *item.Template) (*entities.Store, map[string]entities.EntityID) {
	t.Helper()
	store := entities.NewStore()
	inst, err := store.Spawn(tpl)
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	return store, map[string]entities.EntityID{slot.WieldSlot: inst.ID()}
}

func TestNamesWieldedWeapon_MatchesByKeyword(t *testing.T) {
	store, equip := wield(t, predatorTpl())
	for _, tok := range []string{"ares", "predator", "gun", "pistol"} {
		if !namesWieldedWeapon(store, equip, tok) {
			t.Errorf("%q should name the wielded Predator", tok)
		}
	}
}

func TestNamesWieldedWeapon_RejectsNonMatch(t *testing.T) {
	store, equip := wield(t, predatorTpl())
	// "clip" names the ammunition, not the gun — it must NOT route to a weapon
	// reload, so `reload clip` still fills the carried clip.
	for _, tok := range []string{"clip", "katana", "sword"} {
		if namesWieldedWeapon(store, equip, tok) {
			t.Errorf("%q should not name the wielded weapon", tok)
		}
	}
}

func TestNamesWieldedWeapon_EmptyWieldSlot(t *testing.T) {
	store := entities.NewStore()
	// Nothing wielded → no token names a wielded weapon (so `reload ares` with
	// an unarmed hand falls through to the clip-fill path as before).
	if namesWieldedWeapon(store, map[string]entities.EntityID{}, "ares") {
		t.Error("no wielded weapon should match any token")
	}
}

func TestNamesWieldedWeapon_NilStore(t *testing.T) {
	if namesWieldedWeapon(nil, map[string]entities.EntityID{slot.WieldSlot: "x"}, "ares") {
		t.Error("nil store must be safe and return false")
	}
}
