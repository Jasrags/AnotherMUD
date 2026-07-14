package entities

import (
	"errors"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/item"
)

// mountWeapon spawns a weapon host exposing the given mount points.
func mountWeapon(t *testing.T, s *Store, mounts ...string) *ItemInstance {
	t.Helper()
	it, err := s.Spawn(&item.Template{
		ID: "sr:gun", Name: "a heavy pistol", Type: "weapon",
		WeaponDamage: "1d8", Mounts: mounts,
	})
	if err != nil {
		t.Fatalf("spawn weapon: %v", err)
	}
	return it
}

// accessory spawns a mount accessory that fits the given mounts.
func accessory(t *testing.T, s *Store, id string, mods []item.Modifier, mounts ...string) *ItemInstance {
	t.Helper()
	it, err := s.Spawn(&item.Template{
		ID: item.TemplateID(id), Name: id, Type: "item",
		ModHost: "weapon", AccessoryMounts: mounts, Modifiers: mods,
	})
	if err != nil {
		t.Fatalf("spawn accessory: %v", err)
	}
	return it
}

func TestAttachAccessory_PicksFreeCompatibleMount(t *testing.T) {
	s := NewStore()
	gun := mountWeapon(t, s, "barrel", "top", "under-barrel")
	// A laser sight fits top OR under-barrel; the first free in declared order wins.
	laser := accessory(t, s, "sr:laser", []item.Modifier{{Stat: "hit_mod", Value: 1}}, "top", "under-barrel")

	mount, err := gun.AttachAccessory(laser)
	if err != nil {
		t.Fatalf("AttachAccessory: %v", err)
	}
	if mount != "top" {
		t.Fatalf("chosen mount = %q, want top (first free compatible)", mount)
	}
	if got := gun.OccupiedMounts(); len(got) != 1 || got[0] != "top" {
		t.Fatalf("occupied = %v, want [top]", got)
	}
	free := gun.FreeMounts()
	if len(free) != 2 || free[0] != "barrel" || free[1] != "under-barrel" {
		t.Fatalf("free mounts = %v, want [barrel under-barrel]", free)
	}
	// The accessory's modifier merges into the host's effective Modifiers (§6).
	var hit int
	for _, m := range gun.Modifiers() {
		if m.Stat == "hit_mod" {
			hit += m.Value
		}
	}
	if hit != 1 {
		t.Fatalf("effective hit_mod = %d, want 1", hit)
	}
}

func TestAttachAccessory_OneAccessoryPerMount(t *testing.T) {
	s := NewStore()
	gun := mountWeapon(t, s, "barrel")
	if _, err := gun.AttachAccessory(accessory(t, s, "sr:silencer", nil, "barrel")); err != nil {
		t.Fatalf("attach 1: %v", err)
	}
	// A second barrel accessory has no free barrel — occupied, not incompatible.
	_, err := gun.AttachAccessory(accessory(t, s, "sr:longbarrel", nil, "barrel"))
	if !errors.Is(err, ErrMountOccupied) {
		t.Fatalf("second barrel attach err = %v, want ErrMountOccupied", err)
	}
}

func TestAttachAccessory_RefusesIncompatibleAndNonMountHost(t *testing.T) {
	s := NewStore()
	gun := mountWeapon(t, s, "barrel", "top")
	// A stock accessory fits no mount this gun exposes.
	_, err := gun.AttachAccessory(accessory(t, s, "sr:stock", nil, "stock"))
	if !errors.Is(err, ErrModIncompatible) {
		t.Fatalf("incompatible attach err = %v, want ErrModIncompatible", err)
	}
	// A non-mount host (no mounts) cannot take accessories.
	plain, _ := s.Spawn(&item.Template{ID: "sr:club", Name: "a club", Type: "weapon", WeaponDamage: "1d6"})
	if _, err := plain.AttachAccessory(accessory(t, s, "sr:laser", nil, "top")); !errors.Is(err, ErrNotModifiable) {
		t.Fatalf("non-mount host err = %v, want ErrNotModifiable", err)
	}
	// A capacity mod (no accessory mounts) is not a mount accessory.
	armorMod := armorMod(t, s, "sr:weave", 3, nil, 0)
	if _, err := gun.AttachAccessory(armorMod); !errors.Is(err, ErrNotAModification) {
		t.Fatalf("capacity-mod-as-accessory err = %v, want ErrNotAModification", err)
	}
}

func TestRestoreAccessory_ReseatsMountFromTemplate(t *testing.T) {
	s := NewStore()
	gun := mountWeapon(t, s, "barrel", "top")
	accTpl := &item.Template{
		ID: "sr:laser", Name: "a laser sight", Type: "item",
		ModHost: "weapon", AccessoryMounts: []string{"top"},
		Modifiers: []item.Modifier{{Stat: "hit_mod", Value: 1}},
	}
	gun.RestoreInstalledMod(accTpl)

	occ := gun.OccupiedMounts()
	if len(occ) != 1 || occ[0] != "top" {
		t.Fatalf("restored occupied mounts = %v, want [top]", occ)
	}
	if mods := gun.InstalledMods(); len(mods) != 1 || mods[0].Mount != "top" {
		t.Fatalf("restored InstalledMods = %+v, want one on top", mods)
	}
}

func TestInstallMod_RefusesMountAccessory(t *testing.T) {
	s := NewStore()
	// A capacity host that happens to carry the "weapon" tag must still refuse a
	// mount accessory (it installs by the mount rule, not capacity).
	host, _ := s.Spawn(&item.Template{
		ID: "sr:odd", Name: "an odd host", Type: "item",
		Tags: []string{"weapon"}, Capacity: 9,
	})
	acc := accessory(t, s, "sr:laser", nil, "top")
	if err := host.InstallMod(acc); !errors.Is(err, ErrNotAModification) {
		t.Fatalf("InstallMod(accessory) err = %v, want ErrNotAModification", err)
	}
	if got := len(host.InstalledMods()); got != 0 {
		t.Fatalf("installed mods = %d, want 0 (accessory refused)", got)
	}
}

func TestRestoreAccessory_DroppedWhenHostHasNoMounts(t *testing.T) {
	s := NewStore()
	// A weapon that lost its mounts (content shrank to zero) must not silently
	// re-add a persisted accessory mountless.
	plain, _ := s.Spawn(&item.Template{ID: "sr:club", Name: "a club", Type: "weapon", WeaponDamage: "1d6"})
	accTpl := &item.Template{
		ID: "sr:laser", Name: "a laser sight", Type: "item",
		ModHost: "weapon", AccessoryMounts: []string{"top"},
		Modifiers: []item.Modifier{{Stat: "hit_mod", Value: 1}},
	}
	plain.RestoreInstalledMod(accTpl)
	if got := len(plain.InstalledMods()); got != 0 {
		t.Fatalf("installed mods = %d, want 0 (accessory dropped — no mounts)", got)
	}
	// And its effect must NOT leak into the effective modifiers.
	for _, m := range plain.Modifiers() {
		if m.Stat == "hit_mod" {
			t.Fatal("dropped accessory's hit_mod leaked into effective modifiers")
		}
	}
}

func TestRemoveMod_WorksForAccessories(t *testing.T) {
	s := NewStore()
	gun := mountWeapon(t, s, "top")
	_, _ = gun.AttachAccessory(accessory(t, s, "sr:laser-sight", nil, "top"))
	removed, ok := gun.RemoveMod("laser")
	if !ok || removed.Mount != "top" {
		t.Fatalf("RemoveMod = %+v, %v; want the top accessory", removed, ok)
	}
	if got := gun.FreeMounts(); len(got) != 1 || got[0] != "top" {
		t.Fatalf("free mounts after detach = %v, want [top]", got)
	}
}
