package entities

import "testing"

// A mob spawned from a template carrying `proficiencies` exposes those
// values through Proficiency; unknown abilities report (0, false).
func TestMobProficiencySeededFromTemplate(t *testing.T) {
	tpl := guardTpl()
	tpl.Proficiencies = map[string]int{"second-attack": 50}
	s := NewStore()
	inst, err := s.SpawnMob(tpl)
	if err != nil {
		t.Fatalf("SpawnMob: %v", err)
	}

	if v, ok := inst.Proficiency("second-attack"); !ok || v != 50 {
		t.Errorf("Proficiency(second-attack) = (%d, %v), want (50, true)", v, ok)
	}
	if v, ok := inst.Proficiency("parry"); ok || v != 0 {
		t.Errorf("Proficiency(parry) = (%d, %v), want (0, false) — not seeded", v, ok)
	}
}

// Proficiency normalizes the lookup key (case + surrounding space) so a
// caller passing a registry id in any casing still resolves.
func TestMobProficiencyLookupIsNormalized(t *testing.T) {
	tpl := guardTpl()
	tpl.Proficiencies = map[string]int{"second-attack": 50}
	s := NewStore()
	inst, _ := s.SpawnMob(tpl)

	if v, ok := inst.Proficiency("  Second-Attack "); !ok || v != 50 {
		t.Errorf("normalized lookup = (%d, %v), want (50, true)", v, ok)
	}
}

// A mob whose template declares no proficiencies carries a nil map, and
// Proficiency is safe against it (no panic, always (0, false)).
func TestMobProficiencyNilWhenAbsent(t *testing.T) {
	s := NewStore()
	inst, err := s.SpawnMob(guardTpl()) // guardTpl declares none
	if err != nil {
		t.Fatalf("SpawnMob: %v", err)
	}
	if v, ok := inst.Proficiency("second-attack"); ok || v != 0 {
		t.Errorf("Proficiency on no-passive mob = (%d, %v), want (0, false)", v, ok)
	}
}

// The instance copies the template's proficiency map, so mutating the
// template after spawn cannot reach into a live mob.
func TestMobProficiencyIsDefensiveCopy(t *testing.T) {
	tpl := guardTpl()
	tpl.Proficiencies = map[string]int{"second-attack": 50}
	s := NewStore()
	inst, _ := s.SpawnMob(tpl)

	tpl.Proficiencies["second-attack"] = 99 // mutate template post-spawn
	tpl.Proficiencies["parry"] = 30

	if v, _ := inst.Proficiency("second-attack"); v != 50 {
		t.Errorf("instance proficiency = %d, want 50 (template mutation must not leak in)", v)
	}
	if _, ok := inst.Proficiency("parry"); ok {
		t.Error("instance gained parry from a post-spawn template mutation")
	}
}
