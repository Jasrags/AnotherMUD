package command_test

import (
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/progression"
)

// sheetActor is a combatActor (Actor + Combatant + the embedded
// testActor's SustenanceEntity) extended with the score sheet's
// identity/resource surface, so the `score` handler's full gather path
// can be exercised end to end.
type sheetActor struct {
	*combatActor
}

func (a *sheetActor) RaceID() string       { return "human" }
func (a *sheetActor) ClassID() string      { return "fighter" }
func (a *sheetActor) BackgroundID() string { return "soldier" }
func (a *sheetActor) Alignment() int       { return 0 }
func (a *sheetActor) AlignmentTag() string { return "alignment_neutral" } // raw tag id; sheet strips the prefix
func (a *sheetActor) Gold() int            { return 1000 }
func (a *sheetActor) Mana() int            { return 0 }
func (a *sheetActor) Movement() int        { return 0 }
func (a *sheetActor) StatValue(s progression.StatType) int {
	return map[progression.StatType]int{
		progression.StatSTR: 16, progression.StatINT: 10, progression.StatWIS: 12,
		progression.StatDEX: 14, progression.StatCON: 15, progression.StatLUCK: 8,
	}[s]
}
func (a *sheetActor) Saves() progression.Saves {
	return progression.Saves{Fortitude: 4, Reflex: 1, Will: 1}
}

func TestScore_Handler(t *testing.T) {
	f := newConsiderFixture(t)
	a := &sheetActor{combatActor: newCombatActor("Maerys", "p-1", f.room)}
	a.SetSustenance(84) // Full tier (>= 67)
	r := newRegistry(t)

	dispatchActor(t, r, f.env(), a, "score")

	out := a.lastLine()
	for _, want := range []string{
		"<highlight>Human Fighter</highlight>",
		"<title>Combat</title>",
		"<highlight>16</highlight>", // STR value
		"neutral (0)",
		"<gold>1,000</gold>",
		"Full (84/100)",
		"Fort +4  Ref +1  Will +1", // saving throws row
		"Soldier",                  // background row
	} {
		if !strings.Contains(out, want) {
			t.Errorf("score output missing %q\n--- got ---\n%s", want, out)
		}
	}
}

// `sc` is a score alias.
func TestScore_AliasSc(t *testing.T) {
	f := newConsiderFixture(t)
	a := &sheetActor{combatActor: newCombatActor("Maerys", "p-1", f.room)}
	r := newRegistry(t)
	dispatchActor(t, r, f.env(), a, "sc")
	if got := a.lastLine(); !strings.Contains(got, "Human Fighter") {
		t.Errorf("`sc` alias = %q, want the score sheet", got)
	}
}
