package combat

import "testing"

func TestDefaultPlayerStats(t *testing.T) {
	s := DefaultPlayerStats()
	if s.HitMod != 0 || s.AC != 10 || s.STR != 10 {
		t.Errorf("DefaultPlayerStats() = %+v, want {0, 10, 10}", s)
	}
}

func TestFromTemplateStatsDefaults(t *testing.T) {
	s, maxHP := FromTemplateStats(nil)
	if maxHP != DefaultMobMaxHP {
		t.Errorf("nil stats: maxHP = %d, want %d", maxHP, DefaultMobMaxHP)
	}
	if s.AC != DefaultAC || s.STR != DefaultSTR || s.HitMod != 0 {
		t.Errorf("nil stats: block = %+v, want defaults", s)
	}
}

func TestFromTemplateStatsReadsKnownKeys(t *testing.T) {
	in := map[string]int{
		StatKeyHPMax:  40,
		StatKeyHitMod: 3,
		StatKeyAC:     15,
		StatKeySTR:    14,
		"ignored":     999,
	}
	s, maxHP := FromTemplateStats(in)
	if maxHP != 40 {
		t.Errorf("maxHP = %d, want 40", maxHP)
	}
	want := Stats{HitMod: 3, AC: 15, STR: 14}
	if s != want {
		t.Errorf("stats = %+v, want %+v", s, want)
	}
}

func TestFromTemplateStatsNonPositiveHPMaxFallsBack(t *testing.T) {
	in := map[string]int{StatKeyHPMax: 0}
	_, maxHP := FromTemplateStats(in)
	if maxHP != DefaultMobMaxHP {
		t.Errorf("hp_max=0 maxHP = %d, want default %d", maxHP, DefaultMobMaxHP)
	}
	in[StatKeyHPMax] = -10
	_, maxHP = FromTemplateStats(in)
	if maxHP != DefaultMobMaxHP {
		t.Errorf("hp_max=-10 maxHP = %d, want default %d", maxHP, DefaultMobMaxHP)
	}
}

func TestFromTemplateStatsAllowsNegativeHitMod(t *testing.T) {
	in := map[string]int{StatKeyHitMod: -2}
	s, _ := FromTemplateStats(in)
	if s.HitMod != -2 {
		t.Errorf("HitMod = %d, want -2 (negatives are valid)", s.HitMod)
	}
}
