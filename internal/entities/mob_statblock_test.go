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

// TestMobVitalsTrackHPMaxChanges pins the M14.1 vital-reclamp seam
// end-to-end on the mob side: an effect raising hp_max increases
// the mob's Vitals.Max ceiling; removing it drops max back down and
// clamps current HP if needed.
func TestMobVitalsTrackHPMaxChanges(t *testing.T) {
	s := NewStore()
	inst, err := s.SpawnMob(guardTpl()) // hp_max:40
	if err != nil {
		t.Fatalf("SpawnMob: %v", err)
	}
	if got := inst.Vitals().Max(); got != 40 {
		t.Fatalf("initial Vitals.Max = %d, want 40", got)
	}

	// Boost hp_max by +20. Vitals.Max should follow; Vitals.Current
	// stays at 40 (SetMax does not auto-heal).
	src := srckey.SourceKey("effect:vitality")
	inst.AddModifiers(src, []stats.Modifier{
		{Stat: string(progression.StatHPMax), Value: 20},
	})
	if cur, max := inst.Vitals().Snapshot(); max != 60 || cur != 40 {
		t.Errorf("after +20 hp_max: snapshot = (%d, %d), want (40, 60)", cur, max)
	}

	// Take 30 damage. Current 40 → 10.
	if cur := inst.Vitals().ApplyDamage(30); cur != 10 {
		t.Fatalf("ApplyDamage(30) returned %d, want 10", cur)
	}

	// Remove the effect: hp_max drops back to 40. Current is 10,
	// well below 40, so no down-clamp.
	if !inst.RemoveBySource(src) {
		t.Fatal("RemoveBySource: nothing removed")
	}
	cur, max := inst.Vitals().Snapshot()
	if max != 40 {
		t.Errorf("after effect remove: Vitals.Max = %d, want 40", max)
	}
	if cur != 10 {
		t.Errorf("after effect remove: Vitals.Current = %d, want 10 (below new max, no clamp)", cur)
	}
}

// TestMobVitalsClampDownWhenHPMaxDrops confirms that SetMax also
// clamps current down when the new max is below current.
func TestMobVitalsClampDownWhenHPMaxDrops(t *testing.T) {
	s := NewStore()
	inst, err := s.SpawnMob(guardTpl())
	if err != nil {
		t.Fatalf("SpawnMob: %v", err)
	}

	// Boost +20 → max=60, current stays at 40 (SetMax does not
	// auto-heal).
	src := srckey.SourceKey("effect:bigger")
	inst.AddModifiers(src, []stats.Modifier{
		{Stat: string(progression.StatHPMax), Value: 20},
	})
	if cur, max := inst.Vitals().Snapshot(); cur != 40 || max != 60 {
		t.Errorf("post-boost snapshot = (%d, %d), want (40, 60)", cur, max)
	}
	// Heal to full (current 40 → 60).
	_ = inst.Vitals().Heal(20)
	if cur, max := inst.Vitals().Snapshot(); cur != 60 || max != 60 {
		t.Errorf("post-heal snapshot = (%d, %d), want (60, 60)", cur, max)
	}

	// Yank the effect: max drops to 40; current (60) clamps to 40.
	inst.RemoveBySource(src)
	cur, max := inst.Vitals().Snapshot()
	if max != 40 || cur != 40 {
		t.Errorf("post-remove snapshot = (%d, %d), want (40, 40) clamped", cur, max)
	}
}

// TestMobVitalsTrackBaseHPMaxChanges confirms the listener also
// fires when the base hp_max changes directly (e.g., a level-up
// path that calls AdjustBase rather than installing a modifier).
func TestMobVitalsTrackBaseHPMaxChanges(t *testing.T) {
	s := NewStore()
	inst, err := s.SpawnMob(guardTpl())
	if err != nil {
		t.Fatalf("SpawnMob: %v", err)
	}
	inst.statBlock.AdjustBase(progression.StatHPMax, 5)
	if got := inst.Vitals().Max(); got != 45 {
		t.Errorf("Vitals.Max after AdjustBase +5 = %d, want 45", got)
	}
}
