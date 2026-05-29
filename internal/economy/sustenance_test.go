package economy

import "testing"

// fakeSustEntity is a minimal SustenanceEntity for service tests.
type fakeSustEntity struct {
	id  string
	val int
}

func (f *fakeSustEntity) ID() string          { return f.id }
func (f *fakeSustEntity) Sustenance() int     { return f.val }
func (f *fakeSustEntity) SetSustenance(v int) { f.val = v }

func TestTierOf(t *testing.T) {
	cfg := DefaultSustenanceConfig()
	tests := []struct {
		name  string
		value int
		want  Tier
	}{
		{"max is full", 100, TierFull},
		{"full boundary inclusive", 67, TierFull},
		{"just below full is hungry", 66, TierHungry},
		{"hungry boundary inclusive", 34, TierHungry},
		{"just below hungry is famished", 33, TierFamished},
		{"zero is famished", 0, TierFamished},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cfg.TierOf(tt.value); got != tt.want {
				t.Fatalf("TierOf(%d) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

func TestGetRegenMultiplier(t *testing.T) {
	cfg := DefaultSustenanceConfig()
	tests := []struct {
		value int
		want  float64
	}{
		{100, 1.0},
		{67, 1.0},
		{66, 0.5},
		{34, 0.5},
		{33, 0.0},
		{0, 0.0},
	}
	for _, tt := range tests {
		if got := cfg.GetRegenMultiplier(tt.value); got != tt.want {
			t.Errorf("GetRegenMultiplier(%d) = %v, want %v", tt.value, got, tt.want)
		}
	}
}

func TestServiceSetClamps(t *testing.T) {
	svc := NewSustenanceService(DefaultSustenanceConfig())
	e := &fakeSustEntity{id: "p1", val: 50}

	if got := svc.Set(e, 200); got != MaxSustenance || e.val != MaxSustenance {
		t.Fatalf("Set(200) = %d (entity %d), want %d", got, e.val, MaxSustenance)
	}
	if got := svc.Set(e, -5); got != 0 || e.val != 0 {
		t.Fatalf("Set(-5) = %d (entity %d), want 0", got, e.val)
	}
	if got := svc.Set(e, 75); got != 75 || e.val != 75 {
		t.Fatalf("Set(75) = %d (entity %d), want 75", got, e.val)
	}
}

func TestServiceAddClamps(t *testing.T) {
	svc := NewSustenanceService(DefaultSustenanceConfig())
	e := &fakeSustEntity{id: "p1", val: 90}

	if got := svc.Add(e, 20); got != MaxSustenance { // 90+20 caps at 100
		t.Fatalf("Add(20) from 90 = %d, want %d", got, MaxSustenance)
	}
	e.val = 5
	if got := svc.Add(e, -20); got != 0 { // floors at 0
		t.Fatalf("Add(-20) from 5 = %d, want 0", got)
	}
}

func TestServiceDrain(t *testing.T) {
	svc := NewSustenanceService(DefaultSustenanceConfig()) // DrainAmount 1
	e := &fakeSustEntity{id: "p1", val: 35}

	// 35 -> 34 stays hungry (boundary inclusive).
	if v, tier := svc.Drain(e); v != 34 || tier != TierHungry {
		t.Fatalf("Drain from 35 = (%d, %q), want (34, hungry)", v, tier)
	}
	// 34 -> 33 crosses into famished.
	if v, tier := svc.Drain(e); v != 33 || tier != TierFamished {
		t.Fatalf("Drain from 34 = (%d, %q), want (33, famished)", v, tier)
	}

	// Floors at zero and reports famished.
	e.val = 0
	if v, tier := svc.Drain(e); v != 0 || tier != TierFamished {
		t.Fatalf("Drain from 0 = (%d, %q), want (0, famished)", v, tier)
	}
}

func TestServiceNilSafe(t *testing.T) {
	svc := NewSustenanceService(DefaultSustenanceConfig())
	if got := svc.Read(nil); got != 0 {
		t.Errorf("Read(nil) = %d, want 0", got)
	}
	if got := svc.Set(nil, 50); got != 0 {
		t.Errorf("Set(nil) = %d, want 0", got)
	}
	if got := svc.Add(nil, 10); got != 0 {
		t.Errorf("Add(nil) = %d, want 0", got)
	}
	if v, tier := svc.Drain(nil); v != 0 || tier != TierFamished {
		t.Errorf("Drain(nil) = (%d, %q), want (0, famished)", v, tier)
	}
}

func TestPerShopDrainAmount(t *testing.T) {
	cfg := DefaultSustenanceConfig()
	cfg.DrainAmount = 5
	svc := NewSustenanceService(cfg)
	e := &fakeSustEntity{id: "p1", val: 12}
	if v, _ := svc.Drain(e); v != 7 {
		t.Fatalf("Drain with amount 5 from 12 = %d, want 7", v)
	}
}
