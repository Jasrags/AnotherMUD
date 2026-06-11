package session

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/progression"
)

// combat carries the save-axis names as plain strings (it cannot import
// progression — that would close an import cycle, see combat/saves.go), so
// the two const sets are duplicated by necessity. This guard fails the build
// if a future rename drifts them apart, which would silently break save-event
// display / GMCP correlation. session is a natural home: it imports both.
func TestSaveAxisStringsMatchProgression(t *testing.T) {
	cases := []struct {
		combatStr string
		prog      progression.SaveType
	}{
		{combat.SaveAxisFortitude, progression.SaveFortitude},
		{combat.SaveAxisReflex, progression.SaveReflex},
		{combat.SaveAxisWill, progression.SaveWill},
	}
	for _, c := range cases {
		if c.combatStr != string(c.prog) {
			t.Errorf("axis drift: combat %q != progression %q", c.combatStr, c.prog)
		}
	}
}
