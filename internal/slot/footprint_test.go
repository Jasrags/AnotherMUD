package slot

import (
	"errors"
	"testing"
)

func baselineRegistry(t *testing.T) *Registry {
	t.Helper()
	r := NewRegistry()
	if err := RegisterEngineBaseline(r); err != nil {
		t.Fatalf("RegisterEngineBaseline: %v", err)
	}
	return r
}

func TestFreeKey(t *testing.T) {
	r := baselineRegistry(t)

	// cap-1 free slot → bare name.
	if got, err := r.FreeKey("wield", nil); err != nil || got != "wield" {
		t.Errorf("FreeKey(wield, empty) = %q, %v; want wield", got, err)
	}
	// cap-1 occupied → index-0 key (the displacement target).
	if got, err := r.FreeKey("wield", map[string]bool{"wield": true}); err != nil || got != "wield" {
		t.Errorf("FreeKey(wield, full) = %q, %v; want wield (displace)", got, err)
	}
	// cap-2 finger: first free is finger:0.
	if got, err := r.FreeKey("finger", nil); err != nil || got != "finger:0" {
		t.Errorf("FreeKey(finger, empty) = %q, %v; want finger:0", got, err)
	}
	// cap-2 finger with :0 taken → finger:1.
	if got, err := r.FreeKey("finger", map[string]bool{"finger:0": true}); err != nil || got != "finger:1" {
		t.Errorf("FreeKey(finger, {0}) = %q, %v; want finger:1", got, err)
	}
	// cap-2 finger full → index 0 (displace).
	if got, err := r.FreeKey("finger", map[string]bool{"finger:0": true, "finger:1": true}); err != nil || got != "finger:0" {
		t.Errorf("FreeKey(finger, full) = %q, %v; want finger:0", got, err)
	}
	// unregistered slot.
	if _, err := r.FreeKey("nonesuch", nil); !errors.Is(err, ErrNotFound) {
		t.Errorf("FreeKey(nonesuch) err = %v, want ErrNotFound", err)
	}
}

func TestFootprint(t *testing.T) {
	r := baselineRegistry(t)

	// Non-spanning: footprint is just the target key.
	fp, err := r.Footprint("wield", nil, nil)
	if err != nil || len(fp) != 1 || fp[0] != "wield" {
		t.Errorf("Footprint(wield) = %v, %v; want [wield]", fp, err)
	}

	// Two-handed: target wield + companion offhand, target first.
	fp, err = r.Footprint("wield", []string{"offhand"}, nil)
	if err != nil || len(fp) != 2 || fp[0] != "wield" || fp[1] != "offhand" {
		t.Errorf("Footprint(wield,+offhand) = %v, %v; want [wield offhand]", fp, err)
	}

	// Companion into a multi-cap slot picks the lowest free index, and a
	// second companion on the same base takes the next index.
	fp, err = r.Footprint("head", []string{"finger", "finger"}, nil)
	if err != nil || len(fp) != 3 || fp[0] != "head" || fp[1] != "finger:0" || fp[2] != "finger:1" {
		t.Errorf("Footprint(head,+finger,+finger) = %v, %v; want [head finger:0 finger:1]", fp, err)
	}

	// occupied is honored and NOT mutated.
	occ := map[string]bool{"finger:0": true}
	fp, err = r.Footprint("head", []string{"finger"}, occ)
	if err != nil || fp[1] != "finger:1" {
		t.Errorf("Footprint with finger:0 occupied = %v; want companion finger:1", fp)
	}
	if len(occ) != 1 {
		t.Errorf("Footprint mutated caller's occupied map: %v", occ)
	}

	// Unregistered companion.
	if _, err := r.Footprint("wield", []string{"nonesuch"}, nil); !errors.Is(err, ErrNotFound) {
		t.Errorf("Footprint unregistered companion err = %v, want ErrNotFound", err)
	}
}
