package session

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/entities"
)

// WieldedFirearmReload is the read-only peek behind the autoreload branch
// (autoreload.md §3/§4): it reports whether the wielded weapon is an autoreload
// firearm and whether a reload would find ammo — without mutating anything.
func TestWieldedFirearmReload_HolderFed(t *testing.T) {
	store := entities.NewStore()
	a := newEqActor(t, store)
	spawnHolderGun(t, store, a)

	// A holder-fed pistol with no clip carried: it's a firearm, but no reload.
	if isFirearm, canReload := a.WieldedFirearmReload(); !isFirearm || canReload {
		t.Fatalf("no clip: (isFirearm=%v canReload=%v), want (true, false)", isFirearm, canReload)
	}

	// Carry a loaded clip → a reload is now available.
	clip := spawnClip(t, store, a, "sr:spare", 5)
	if isFirearm, canReload := a.WieldedFirearmReload(); !isFirearm || !canReload {
		t.Fatalf("loaded clip: (isFirearm=%v canReload=%v), want (true, true)", isFirearm, canReload)
	}

	// The peek is read-only: the clip is still carried and still full.
	if _, ok := store.GetByID(clip.ID()); !ok {
		t.Error("peek consumed the clip — it must be read-only")
	}
	if clip.MagazineLoaded() != 5 {
		t.Errorf("peek changed clip load to %d, want 5 (read-only)", clip.MagazineLoaded())
	}

	// An empty clip (0 rounds) does not count as available ammo.
	clip.SetMagazineLoaded(0)
	if isFirearm, canReload := a.WieldedFirearmReload(); !isFirearm || canReload {
		t.Fatalf("empty clip: (isFirearm=%v canReload=%v), want (true, false)", isFirearm, canReload)
	}
}

// A wielded non-firearm (no holder, no magazine) reports isFirearm=false so the
// autoreload trigger falls through to the default dry narration.
func TestWieldedFirearmReload_NonFirearm(t *testing.T) {
	store := entities.NewStore()
	a := newEqActor(t, store)
	// No weapon wielded at all → not a firearm.
	if isFirearm, canReload := a.WieldedFirearmReload(); isFirearm || canReload {
		t.Fatalf("bare hands: (isFirearm=%v canReload=%v), want (false, false)", isFirearm, canReload)
	}
}
