package command

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/visibility"
)

// fakeRoller is a deterministic progression.Roller: IntN always returns n,
// so ResolveSkillCheck sees a d20 face of n+1.
type fakeRoller struct{ n int }

func (r fakeRoller) IntN(int) int { return r.n }

// fakePerceiver records contest outcomes and reports a fixed perception
// bonus. `set[i]` holds the remembered outcome; presence = already contested.
type fakePerceiver struct {
	bonus int
	set   map[uint64]bool
}

func (p *fakePerceiver) PerceptionBonus() int { return p.bonus }
func (p *fakePerceiver) ContestOutcome(i uint64) (won, done bool) {
	won, done = p.set[i]
	return won, done
}
func (p *fakePerceiver) RecordContest(i uint64, won bool) {
	if p.set == nil {
		p.set = map[uint64]bool{}
	}
	p.set[i] = won
}

func hideLayer(score int, inst uint64) visibility.Layer {
	return visibility.Layer{Source: visibility.SourceHide, Score: score, Instance: inst}
}

// A won perception contest pierces the hide and is remembered (§4.1/§4.2).
func TestVisObserver_ContestWinRecordsPierce(t *testing.T) {
	per := &fakePerceiver{bonus: 2}
	// roll 19 (IntN 18 + 1) + bonus 2 = 21 >= score 15 → win.
	o := visObserver{id: "obs", per: per, roller: fakeRoller{n: 18}}
	if !o.Contest(hideLayer(15, 7)) {
		t.Fatal("a beating roll should win the contest")
	}
	if !per.set[7] {
		t.Error("a won contest must record the pierce for sticky memory")
	}
	if !o.AlreadyPierced(7) {
		t.Error("AlreadyPierced should report the recorded instance")
	}
}

// A lost contest does not pierce, but it IS recorded (sticky loss) so the
// same instance is not re-rolled this room.
func TestVisObserver_ContestLoseRecordsStickyLoss(t *testing.T) {
	per := &fakePerceiver{bonus: 0}
	// roll 3 (IntN 2 + 1) + 0 = 3 < score 15 → lose.
	o := visObserver{id: "obs", per: per, roller: fakeRoller{n: 2}}
	if o.Contest(hideLayer(15, 9)) {
		t.Fatal("a failing roll should lose the contest")
	}
	won, done := per.ContestOutcome(9)
	if !done || won {
		t.Errorf("a lost contest must be recorded as done+lost; got won=%v done=%v", won, done)
	}
	if o.AlreadyPierced(9) {
		t.Error("a lost contest is not a pierce")
	}
}

// The HIGH fix: a lost contest is sticky — a SECOND contest against the same
// instance returns the remembered loss WITHOUT re-rolling, even if the roller
// would now win. This stops render+resolve in one command (or repeated looks)
// from giving the observer extra rolls against a hidden target.
func TestVisObserver_LostContestNotRerolled(t *testing.T) {
	per := &fakePerceiver{bonus: 0}
	loser := visObserver{id: "obs", per: per, roller: fakeRoller{n: 0}} // roll 1 → lose
	if loser.Contest(hideLayer(15, 4)) {
		t.Fatal("first contest should lose")
	}
	// A roller that would now WIN — but the sticky loss must hold.
	wouldWin := visObserver{id: "obs", per: per, roller: fakeRoller{n: 19}} // roll 20 (nat-20)
	if wouldWin.Contest(hideLayer(15, 4)) {
		t.Error("a remembered loss must not be re-rolled into a win this room")
	}
}

// An observer with no perceiver or roller cannot pierce roll-gated
// concealment (the degraded path for a viewer with no perception wired).
func TestVisObserver_NoPerceptionCannotContest(t *testing.T) {
	if (visObserver{id: "o"}).Contest(hideLayer(1, 1)) {
		t.Error("no perceiver/roller must not pierce")
	}
	noRoller := visObserver{id: "o", per: &fakePerceiver{}}
	if noRoller.Contest(hideLayer(1, 1)) {
		t.Error("no roller must not pierce")
	}
}

// End-to-end through CanSee: a hidden target is seen iff the contest is won;
// once pierced, a second CanSee skips the roll (sticky), so even a now-losing
// roller still sees the target.
func TestVisObserver_CanSeeHiddenStickiness(t *testing.T) {
	per := &fakePerceiver{bonus: 5}
	winner := visObserver{id: "obs", per: per, roller: fakeRoller{n: 18}} // roll 19
	tgt := visTarget{id: "rogue", layers: []visibility.Layer{hideLayer(12, 3)}}

	if !visibility.CanSee(winner, tgt) {
		t.Fatal("winning the contest should reveal the hidden target")
	}
	// Now switch to a roller that would always lose; sticky memory should
	// still report the target as seen (no re-roll).
	sticky := visObserver{id: "obs", per: per, roller: fakeRoller{n: 0}} // roll 1 (nat-1)
	if !visibility.CanSee(sticky, tgt) {
		t.Error("a remembered pierce must keep the target visible without re-rolling")
	}
}
