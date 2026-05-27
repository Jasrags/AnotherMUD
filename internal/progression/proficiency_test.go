package progression_test

import (
	"reflect"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/progression"
)

func newManagerWith(t *testing.T, abilities ...*progression.Ability) *progression.ProficiencyManager {
	t.Helper()
	reg := progression.NewAbilityRegistry()
	for _, a := range abilities {
		if err := reg.Register(a); err != nil {
			t.Fatalf("Register(%s): %v", a.ID, err)
		}
	}
	return progression.NewProficiencyManager(reg, progression.DefaultProficiencyConfig())
}

func TestProficiencyManager_LearnEstablishesCapAndClamps(t *testing.T) {
	m := newManagerWith(t,
		&progression.Ability{ID: "kick", DisplayName: "Kick", Type: progression.AbilityActive, Category: progression.AbilitySkill, DefaultCap: 50},
		&progression.Ability{ID: "punch", DisplayName: "Punch", Type: progression.AbilityActive, Category: progression.AbilitySkill},
	)
	// Registry-provided DefaultCap wins on first Learn.
	m.Learn("e-1", "kick", 200)
	if c := m.Cap("e-1", "kick"); c != 50 {
		t.Errorf("Cap(kick) = %d, want 50", c)
	}
	if v, _ := m.Proficiency("e-1", "kick"); v != 50 {
		t.Errorf("Proficiency(kick) clamped to %d, want 50", v)
	}
	// Ability without DefaultCap uses manager DefaultLearnCap.
	m.Learn("e-1", "punch", 5)
	if c := m.Cap("e-1", "punch"); c != 100 {
		t.Errorf("Cap(punch) = %d, want 100 (DefaultLearnCap)", c)
	}
	if v, _ := m.Proficiency("e-1", "punch"); v != 5 {
		t.Errorf("Proficiency(punch) = %d, want 5", v)
	}
	// Value below 1 is floor-clamped to 1.
	m.Learn("e-1", "punch", 0)
	if v, _ := m.Proficiency("e-1", "punch"); v != 1 {
		t.Errorf("Proficiency(punch) after Learn(0) = %d, want 1", v)
	}
}

func TestProficiencyManager_HasAndForgetPreserveCap(t *testing.T) {
	m := newManagerWith(t, &progression.Ability{ID: "kick", DisplayName: "Kick", Type: progression.AbilityActive, Category: progression.AbilitySkill, DefaultCap: 75})
	m.Learn("e-1", "kick", 20)
	if !m.Has("e-1", "kick") {
		t.Fatalf("Has(kick) = false after Learn")
	}
	m.Forget("e-1", "kick")
	if m.Has("e-1", "kick") {
		t.Errorf("Has(kick) = true after Forget")
	}
	if c := m.Cap("e-1", "kick"); c != 75 {
		t.Errorf("Cap after Forget = %d, want 75 (preserved)", c)
	}
	// Re-learn with explicit value uses the preserved cap.
	m.Learn("e-1", "kick", 50)
	if v, _ := m.Proficiency("e-1", "kick"); v != 50 {
		t.Errorf("Proficiency after re-learn = %d, want 50", v)
	}
}

func TestProficiencyManager_SetCapReclamps(t *testing.T) {
	m := newManagerWith(t, &progression.Ability{ID: "kick", DisplayName: "Kick", Type: progression.AbilityActive, Category: progression.AbilitySkill})
	m.Learn("e-1", "kick", 80)
	m.SetCap("e-1", "kick", 40)
	if v, _ := m.Proficiency("e-1", "kick"); v != 40 {
		t.Errorf("Proficiency after cap drop = %d, want 40 (re-clamped)", v)
	}
	// SetCap on unlearned ability records the cap but doesn't create a prof entry.
	m.SetCap("e-2", "kick", 25)
	if m.Has("e-2", "kick") {
		t.Errorf("SetCap on unlearned ability created proficiency entry")
	}
	if c := m.Cap("e-2", "kick"); c != 25 {
		t.Errorf("Cap(e-2,kick) = %d, want 25", c)
	}
	// Clamp on too-high SetCap.
	m.SetCap("e-1", "kick", 500)
	if c := m.Cap("e-1", "kick"); c != 100 {
		t.Errorf("Cap after SetCap(500) = %d, want 100", c)
	}
}

func TestProficiencyManager_AddProficiencyClamps(t *testing.T) {
	m := newManagerWith(t, &progression.Ability{ID: "kick", DisplayName: "Kick", Type: progression.AbilityActive, Category: progression.AbilitySkill, DefaultCap: 50})
	m.Learn("e-1", "kick", 45)
	m.AddProficiency("e-1", "kick", 100)
	if v, _ := m.Proficiency("e-1", "kick"); v != 50 {
		t.Errorf("AddProficiency past cap = %d, want 50", v)
	}
	m.AddProficiency("e-1", "kick", -200)
	if v, _ := m.Proficiency("e-1", "kick"); v != 1 {
		t.Errorf("AddProficiency below 1 = %d, want 1", v)
	}
	// AddProficiency on previously-unset (but capped) entry seeds at 1 then adds.
	m.SetCap("e-2", "kick", 25)
	m.AddProficiency("e-2", "kick", 5)
	if v, _ := m.Proficiency("e-2", "kick"); v != 6 {
		t.Errorf("AddProficiency seeded = %d, want 6", v)
	}
}

func TestProficiencyManager_GetCapImplementsSeam(t *testing.T) {
	m := newManagerWith(t, &progression.Ability{ID: "kick", DisplayName: "Kick", Type: progression.AbilityActive, Category: progression.AbilitySkill, DefaultCap: 50})
	// Unknown entity: cap is DefaultUnsetCap, prof 0, learned false.
	capV, prof, learned := m.GetCap("ghost", "kick")
	if capV != 100 || prof != 0 || learned {
		t.Errorf("GetCap(ghost,kick) = (%d,%d,%v), want (100,0,false)", capV, prof, learned)
	}
	// SetCap then GetCap: cap reported, still not learned.
	m.SetCap("e-1", "kick", 25)
	capV, prof, learned = m.GetCap("e-1", "kick")
	if capV != 25 || prof != 0 || learned {
		t.Errorf("GetCap after SetCap = (%d,%d,%v), want (25,0,false)", capV, prof, learned)
	}
	// Learn then GetCap: learned true.
	m.Learn("e-1", "kick", 10)
	capV, prof, learned = m.GetCap("e-1", "kick")
	if capV != 25 || prof != 10 || !learned {
		t.Errorf("GetCap after Learn = (%d,%d,%v), want (25,10,true)", capV, prof, learned)
	}
}

func TestProficiencyManager_AbilityName(t *testing.T) {
	m := newManagerWith(t, &progression.Ability{ID: "kick", DisplayName: "Kick", Type: progression.AbilityActive, Category: progression.AbilitySkill})
	if name, ok := m.AbilityName("KICK"); !ok || name != "Kick" {
		t.Errorf("AbilityName(KICK) = (%q,%v), want (Kick,true)", name, ok)
	}
	if _, ok := m.AbilityName("unknown"); ok {
		t.Errorf("AbilityName(unknown) ok=true")
	}
	// Nil-registry tolerant.
	bare := progression.NewProficiencyManager(nil, progression.DefaultProficiencyConfig())
	if _, ok := bare.AbilityName("anything"); ok {
		t.Errorf("nil-registry AbilityName ok=true")
	}
}

func TestProficiencyManager_SnapshotAndRestore(t *testing.T) {
	m := newManagerWith(t,
		&progression.Ability{ID: "kick", DisplayName: "Kick", Type: progression.AbilityActive, Category: progression.AbilitySkill, DefaultCap: 50},
		&progression.Ability{ID: "heal", DisplayName: "Heal", Type: progression.AbilityActive, Category: progression.AbilitySpell, DefaultCap: 75},
	)
	m.Learn("e-1", "kick", 25)
	m.Learn("e-1", "heal", 10)
	m.SetCap("e-1", "heal", 30)

	snap := m.Snapshot("e-1")
	if snap.Proficiency["kick"] != 25 || snap.Proficiency["heal"] != 10 {
		t.Errorf("Snapshot proficiency = %+v", snap.Proficiency)
	}
	if snap.Cap["kick"] != 50 || snap.Cap["heal"] != 30 {
		t.Errorf("Snapshot cap = %+v", snap.Cap)
	}

	// Mutation of returned maps must not affect manager state.
	snap.Proficiency["kick"] = 999
	if v, _ := m.Proficiency("e-1", "kick"); v != 25 {
		t.Errorf("Snapshot returned aliased map: proficiency = %d", v)
	}

	// Restore into a fresh manager round-trips.
	fresh := newManagerWith(t,
		&progression.Ability{ID: "kick", DisplayName: "Kick", Type: progression.AbilityActive, Category: progression.AbilitySkill, DefaultCap: 50},
		&progression.Ability{ID: "heal", DisplayName: "Heal", Type: progression.AbilityActive, Category: progression.AbilitySpell, DefaultCap: 75},
	)
	fresh.Restore("e-1", m.Snapshot("e-1"))
	want := []progression.ProficiencyEntry{{ID: "heal", Value: 10}, {ID: "kick", Value: 25}}
	got := fresh.LearnedAbilities("e-1")
	if !reflect.DeepEqual(got, want) {
		t.Errorf("LearnedAbilities after Restore = %+v, want %+v", got, want)
	}
	if c := fresh.Cap("e-1", "heal"); c != 30 {
		t.Errorf("Cap(heal) after Restore = %d, want 30", c)
	}
}

func TestProficiencyManager_RestoreClampsDirtyValues(t *testing.T) {
	m := newManagerWith(t)
	m.Restore("e-1", progression.AbilitySnapshot{
		Proficiency: map[string]int{"kick": 500, "punch": -10},
		Cap:         map[string]int{"kick": 200, "punch": 25},
	})
	if v, _ := m.Proficiency("e-1", "kick"); v != 100 {
		t.Errorf("Restore proficiency >cap clamped to %d, want 100", v)
	}
	if c := m.Cap("e-1", "kick"); c != 100 {
		t.Errorf("Restore cap >100 clamped to %d, want 100", c)
	}
	if v, _ := m.Proficiency("e-1", "punch"); v != 1 {
		t.Errorf("Restore negative proficiency floor = %d, want 1", v)
	}
}

func TestProficiencyManager_RestoreEmptyNoop(t *testing.T) {
	m := newManagerWith(t)
	m.Learn("e-1", "kick", 5) // implicit ability with default-cap fallback
	m.Restore("e-1", progression.AbilitySnapshot{})
	if !m.Has("e-1", "kick") {
		t.Errorf("empty Restore wiped state")
	}
}

func TestProficiencyManager_DropClearsEntity(t *testing.T) {
	m := newManagerWith(t)
	m.Learn("e-1", "kick", 10)
	m.Drop("e-1")
	if m.Has("e-1", "kick") {
		t.Errorf("Has after Drop = true")
	}
	if got := m.LearnedAbilities("e-1"); got != nil {
		t.Errorf("LearnedAbilities after Drop = %+v, want nil", got)
	}
}

func TestProficiencyManager_NormalizesIds(t *testing.T) {
	m := newManagerWith(t, &progression.Ability{ID: "kick", DisplayName: "Kick", Type: progression.AbilityActive, Category: progression.AbilitySkill})
	m.Learn(" E-1 ", " KICK ", 50)
	if !m.Has("e-1", "kick") {
		t.Errorf("Has(e-1,kick) = false after whitespace+caps Learn")
	}
	if v, _ := m.Proficiency("E-1", "Kick"); v != 50 {
		t.Errorf("Proficiency lookup case-insensitive failed: %d", v)
	}
}

func TestProficiencyManager_EmptyIdsAreNoops(t *testing.T) {
	m := newManagerWith(t)
	m.Learn("", "kick", 10)
	m.Learn("e-1", "", 10)
	if got := m.LearnedAbilities(""); got != nil {
		t.Errorf("LearnedAbilities('') = %+v", got)
	}
	if v, ok := m.Proficiency("", "kick"); v != 0 || ok {
		t.Errorf("Proficiency('',kick) = (%d,%v)", v, ok)
	}
}
