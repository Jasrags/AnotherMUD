package session

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/progression"
)

// TestFillVitals_RefillsAfterHpMaxRaise covers the creation-time refill a
// metatype hp_max bonus needs (sr-m3c-deferred-fixes, Physical-monitor MVP): a
// stat_bonuses hp_max grant raises the Vitals ceiling via OnMaxChange, but
// Vitals.SetMax deliberately leaves current alone (a raise never auto-heals), so
// a freshly-created troll would spawn at 20/26. FillVitals tops current to the
// new max. Human (no hp_max bonus) is already full, so FillVitals is a no-op.
func TestFillVitals_RefillsAfterHpMaxRaise(t *testing.T) {
	a := &connActor{
		statBlock: progression.NewWithBase(map[progression.StatType]int{progression.StatHPMax: 20}),
		vitals:    combat.NewVitals(20),
	}
	// Mirror session.go's login wiring: an hp_max change moves the ceiling.
	a.statBlock.OnMaxChange(progression.StatHPMax, func(_, newMax int) { a.vitals.SetMax(newMax) })

	// A metatype hp_max bonus (+6, the troll) raises max but leaves current.
	a.statBlock.AdjustBase(progression.StatHPMax, 6)
	if cur, max := a.vitals.Snapshot(); cur != 20 || max != 26 {
		t.Fatalf("after hp_max +6: %d/%d, want 20/26 (SetMax leaves current alone)", cur, max)
	}

	a.FillVitals()
	if cur, max := a.vitals.Snapshot(); cur != 26 || max != 26 {
		t.Errorf("after FillVitals: %d/%d, want 26/26 (topped to max)", cur, max)
	}
}

// TestFillVitals_NoOpWhenFull is the human case: already full, FillVitals leaves
// current unchanged (Heal caps at max).
func TestFillVitals_NoOpWhenFull(t *testing.T) {
	a := &connActor{vitals: combat.NewVitals(20)}
	a.FillVitals()
	if cur, max := a.vitals.Snapshot(); cur != 20 || max != 20 {
		t.Errorf("FillVitals on a full 20/20: %d/%d, want 20/20", cur, max)
	}
}
