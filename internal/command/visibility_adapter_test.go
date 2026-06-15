package command

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/visibility"
)

// fakeRoller is a deterministic progression.Roller: IntN always returns n,
// so ResolveSkillCheck sees a d20 face of n+1.
type fakeRoller struct{ n int }

func (r fakeRoller) IntN(int) int { return r.n }

// fakePerceiver records pierces and reports a fixed perception bonus.
type fakePerceiver struct {
	bonus int
	set   map[uint64]bool
}

func (p *fakePerceiver) PerceptionBonus() int                { return p.bonus }
func (p *fakePerceiver) HasPiercedConcealment(i uint64) bool { return p.set[i] }
func (p *fakePerceiver) RecordConcealmentPierce(i uint64) {
	if p.set == nil {
		p.set = map[uint64]bool{}
	}
	p.set[i] = true
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

// A lost contest neither pierces nor records.
func TestVisObserver_ContestLoseNoPierce(t *testing.T) {
	per := &fakePerceiver{bonus: 0}
	// roll 3 (IntN 2 + 1) + 0 = 3 < score 15 → lose.
	o := visObserver{id: "obs", per: per, roller: fakeRoller{n: 2}}
	if o.Contest(hideLayer(15, 9)) {
		t.Fatal("a failing roll should lose the contest")
	}
	if per.set[9] {
		t.Error("a lost contest must not record a pierce")
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
