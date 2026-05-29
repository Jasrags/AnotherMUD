package entities

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/srckey"
	"github.com/Jasrags/AnotherMUD/internal/stats"
)

// Compile-time assertion: a MobInstance satisfies the effect-target
// surface, so the effect manager can install modifiers on mobs.
var _ progression.EffectTarget = (*MobInstance)(nil)

func TestMobStatsDeriveFromTemplate(t *testing.T) {
	s := NewStore()
	inst, err := s.SpawnMob(guardTpl()) // str:12, hp_max:40
	if err != nil {
		t.Fatalf("SpawnMob: %v", err)
	}
	st := inst.Stats()
	if st.STR != 12 {
		t.Errorf("STR = %d, want 12 (from template)", st.STR)
	}
	if st.AC != combat.DefaultAC {
		t.Errorf("AC = %d, want default %d (template omitted it)", st.AC, combat.DefaultAC)
	}
	if st.HitMod != 0 {
		t.Errorf("HitMod = %d, want 0", st.HitMod)
	}
	// hp_max drives vitals max.
	if _, max := inst.Vitals().Snapshot(); max != 40 {
		t.Errorf("max HP = %d, want 40 (template hp_max)", max)
	}
}

func TestMobStatsDefaultMaxHPWhenAbsentOrZero(t *testing.T) {
	tpl := guardTpl()
	tpl.Stats = map[string]int{"hp_max": 0} // non-positive → default
	s := NewStore()
	inst, err := s.SpawnMob(tpl)
	if err != nil {
		t.Fatalf("SpawnMob: %v", err)
	}
	if _, max := inst.Vitals().Snapshot(); max != combat.DefaultMobMaxHP {
		t.Errorf("max HP = %d, want default %d for hp_max:0", max, combat.DefaultMobMaxHP)
	}
}

// An effect's modifiers installed via AddModifiers change the derived
// combat stats; RemoveBySource reverses exactly that set.
func TestMobEffectModifiersChangeStats(t *testing.T) {
	s := NewStore()
	inst, err := s.SpawnMob(guardTpl())
	if err != nil {
		t.Fatalf("SpawnMob: %v", err)
	}
	baseAC := inst.Stats().AC

	src := srckey.SourceKey("effect:bless")
	inst.AddModifiers(src, []stats.Modifier{{Stat: string(progression.StatAC), Value: 5}})
	if got := inst.Stats().AC; got != baseAC+5 {
		t.Fatalf("AC after +5 bless = %d, want %d", got, baseAC+5)
	}

	if !inst.RemoveBySource(src) {
		t.Error("RemoveBySource should report a removal")
	}
	if got := inst.Stats().AC; got != baseAC {
		t.Errorf("AC after effect expiry = %d, want %d (back to base)", got, baseAC)
	}
}

func TestMobEntityIDIsBareID(t *testing.T) {
	s := NewStore()
	inst, err := s.SpawnMob(guardTpl())
	if err != nil {
		t.Fatalf("SpawnMob: %v", err)
	}
	if inst.EntityID() != string(inst.ID()) {
		t.Errorf("EntityID() = %q, want bare id %q", inst.EntityID(), inst.ID())
	}
}
