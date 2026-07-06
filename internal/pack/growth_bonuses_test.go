package pack

import (
	"fmt"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/progression"
)

// TestValidateGrowthBonuses_FlagsInertLowCapSource covers the sr-m3c authoring
// guardrail: a class growth_bonuses source stat whose attribute-set cap is below
// 12 can never yield a positive d20 (v-10)/2 growth modifier, so the bonus is a
// silent no-op (SR raw-value attrs cap at 6). validateGrowthBonuses flags it.
func TestValidateGrowthBonuses_FlagsInertLowCapSource(t *testing.T) {
	regs := NewRegistries()
	// An SR-style set: strength caps at 6, so (strength-10)/2 is always <= 0.
	if err := regs.AttributeSets.Register(&progression.AttributeSet{
		ID:         "shadowrun5",
		Attributes: []progression.Attribute{{ID: "strength", Cap: 6}},
	}); err != nil {
		t.Fatalf("register set: %v", err)
	}
	if err := regs.Classes.Register(&progression.Class{
		ID:            "street-samurai",
		GrowthBonuses: map[progression.StatType]progression.StatType{"strength": "strength"},
	}); err != nil {
		t.Fatalf("register class: %v", err)
	}

	warns := validateGrowthBonuses(regs)
	if len(warns) != 1 {
		t.Fatalf("got %d warnings, want 1: %+v", len(warns), warns)
	}
	w := warns[0]
	if w.Class != "street-samurai" || w.Source != "strength" || w.Set != "shadowrun5" || w.Cap != 6 {
		t.Errorf("warning = %+v, want class street-samurai / source strength / set shadowrun5 / cap 6", w)
	}
}

// TestValidateGrowthBonuses_IgnoresReachableAndUncapped is the control: a source
// that can reach 12 (classic cap 22) or has no set cap (0) is NOT flagged — the
// growth bonus is viable there, so warning would be a false positive.
func TestValidateGrowthBonuses_IgnoresReachableAndUncapped(t *testing.T) {
	regs := NewRegistries()
	if err := regs.AttributeSets.Register(&progression.AttributeSet{
		ID:         "classic",
		Attributes: []progression.Attribute{{ID: "str", Cap: 22}, {ID: "luck", Cap: 0}},
	}); err != nil {
		t.Fatalf("register set: %v", err)
	}
	if err := regs.Classes.Register(&progression.Class{
		ID:            "warrior",
		GrowthBonuses: map[progression.StatType]progression.StatType{"str": "str", "luck": "luck"},
	}); err != nil {
		t.Fatalf("register class: %v", err)
	}
	if warns := validateGrowthBonuses(regs); len(warns) != 0 {
		t.Errorf("reachable/uncapped sources were flagged: %+v", warns)
	}
}

// TestValidateGrowthBonuses_CapBoundary pins the strict-inequality pivot: the
// d20 modifier (v-10)/2 is 0 at cap 11 (inert → warn) and 1 at cap 12 (viable →
// no warn). These two rows guard against the condition drifting to <=.
func TestValidateGrowthBonuses_CapBoundary(t *testing.T) {
	for _, tt := range []struct {
		cap      int
		wantWarn bool
	}{
		{11, true},  // (11-10)/2 = 0, clamped — never fires.
		{12, false}, // (12-10)/2 = 1 — the first viable cap.
	} {
		t.Run(fmt.Sprintf("cap%d", tt.cap), func(t *testing.T) {
			regs := NewRegistries()
			if err := regs.AttributeSets.Register(&progression.AttributeSet{
				ID:         "s",
				Attributes: []progression.Attribute{{ID: "strength", Cap: tt.cap}},
			}); err != nil {
				t.Fatalf("register set: %v", err)
			}
			if err := regs.Classes.Register(&progression.Class{
				ID:            "c",
				GrowthBonuses: map[progression.StatType]progression.StatType{"strength": "strength"},
			}); err != nil {
				t.Fatalf("register class: %v", err)
			}
			warns := validateGrowthBonuses(regs)
			if got := len(warns) > 0; got != tt.wantWarn {
				t.Errorf("cap %d: warned=%v, want %v (%+v)", tt.cap, got, tt.wantWarn, warns)
			}
		})
	}
}
