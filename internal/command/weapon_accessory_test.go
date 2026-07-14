package command_test

import (
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/item"
)

// modGunTpl is a mount weapon host; modLaserTpl a mount accessory that fits top
// or under-barrel, granting a hit_mod via the equip modifier path.
func modGunTpl() *item.Template {
	return &item.Template{
		ID: "sr:pistol", Name: "a heavy pistol", Type: "weapon",
		Keywords: []string{"pistol", "gun"}, EligibleSlots: []string{"wield"},
		WeaponDamage: "2d6", Mounts: []string{"barrel", "top", "under-barrel"},
	}
}

func modLaserTpl() *item.Template {
	return &item.Template{
		ID: "sr:laser-sight", Name: "a laser sight", Type: "item",
		Keywords: []string{"laser", "sight"},
		ModHost:  "weapon", AccessoryMounts: []string{"under-barrel", "top"},
		Modifiers: []item.Modifier{{Stat: "hit_mod", Value: 1}},
	}
}

func modGunEnvWithSpawn(f *eqFixture) command.Env {
	env := f.env()
	env.Spawn = &fakeSpawnService{
		store: f.store,
		items: map[string]*item.Template{"sr:laser-sight": modLaserTpl()},
	}
	return env
}

func TestModify_AttachAccessoryToWeaponMount(t *testing.T) {
	r := newRegistry(t)
	f := newEqFixture(t)
	a := newTestActor(f.room)
	gun := f.spawnInInventory(t, modGunTpl(), a)
	laser := f.spawnInInventory(t, modLaserTpl(), a)

	dispatch(t, r, f.env(), a, "modify pistol laser")

	if containsID(a.Inventory(), laser.ID()) {
		t.Error("the laser sight should be consumed from inventory on attach")
	}
	// It seats on the first free compatible mount ("top" — declared before
	// under-barrel on the gun).
	occ := gun.OccupiedMounts()
	if len(occ) != 1 || occ[0] != "top" {
		t.Fatalf("occupied mounts = %v, want [top]", occ)
	}
	if out := a.lastLine(); !strings.Contains(out, "attach") || !strings.Contains(out, "top mount") {
		t.Errorf("attach cue = %q", out)
	}
}

func TestModify_MountInfoAndOccupancyRefusal(t *testing.T) {
	r := newRegistry(t)
	f := newEqFixture(t)
	a := newTestActor(f.room)
	// A single-mount gun (only "top").
	gunTpl := modGunTpl()
	gunTpl.Mounts = []string{"top"}
	_ = f.spawnInInventory(t, gunTpl, a)
	_ = f.spawnInInventory(t, modLaserTpl(), a)
	// A second laser that also only fits top/under-barrel.
	laser2 := modLaserTpl()
	laser2.ID = "sr:laser-2"
	laser2.Keywords = []string{"designator"}
	_ = f.spawnInInventory(t, laser2, a)

	dispatch(t, r, f.env(), a, "modify pistol laser")

	// Info form shows the mount now occupied.
	dispatch(t, r, f.env(), a, "modify pistol")
	if out := a.lastLine(); !strings.Contains(out, "mounts") || !strings.Contains(out, "laser sight") {
		t.Errorf("mount info = %q", out)
	}

	// The second accessory has no free compatible mount → occupied refusal.
	dispatch(t, r, f.env(), a, "modify pistol designator")
	if out := a.lastLine(); !strings.Contains(out, "no free mount") {
		t.Errorf("occupancy refusal = %q", out)
	}
}

func TestUnmodify_DetachAccessoryFromWeapon(t *testing.T) {
	r := newRegistry(t)
	f := newEqFixture(t)
	a := newTestActor(f.room)
	gun := f.spawnInInventory(t, modGunTpl(), a)
	_ = f.spawnInInventory(t, modLaserTpl(), a)
	env := modGunEnvWithSpawn(f)

	dispatch(t, r, env, a, "modify pistol laser")
	if len(gun.InstalledMods()) != 1 {
		t.Fatal("precondition: accessory attached")
	}
	dispatch(t, r, env, a, "unmodify pistol laser")

	if got := len(gun.InstalledMods()); got != 0 {
		t.Fatalf("installed after detach = %d, want 0", got)
	}
	if got := gun.FreeMounts(); len(got) != 3 {
		t.Fatalf("free mounts after detach = %v, want all 3", got)
	}
	var back bool
	for _, it := range collectInv(f.store, a.Inventory()) {
		if it.IsAccessory() {
			back = true
		}
	}
	if !back {
		t.Error("the detached laser sight should return to inventory")
	}
	if out := a.lastLine(); !strings.Contains(out, "pocket") {
		t.Errorf("detach cue = %q", out)
	}
}
