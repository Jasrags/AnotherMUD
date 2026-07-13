package visibility

import (
	"slices"
	"testing"
)

// fakeTarget is a test Target carrying a fixed set of layers.
type fakeTarget struct {
	id     string
	layers []Layer
}

func (t fakeTarget) VisibilityID() string       { return t.id }
func (t fakeTarget) ConcealmentLayers() []Layer { return t.layers }

// fakeObserver is a configurable test Observer. Contest records the
// instances it was asked to contest and returns contestWins.
type fakeObserver struct {
	id            string
	bypass        bool
	piercesDark   bool
	seesInvisible bool
	adminRank     int
	detectsHidden bool
	pierced       map[uint64]bool // sticky memory
	contestWins   bool
	contested     []uint64 // instances Contest() was invoked for
}

func (o *fakeObserver) VisibilityID() string         { return o.id }
func (o *fakeObserver) Bypass() bool                 { return o.bypass }
func (o *fakeObserver) PiercesDarkness() bool        { return o.piercesDark }
func (o *fakeObserver) SeesInvisible() bool          { return o.seesInvisible }
func (o *fakeObserver) AdminRank() int               { return o.adminRank }
func (o *fakeObserver) DetectsHidden() bool          { return o.detectsHidden }
func (o *fakeObserver) AlreadyPierced(i uint64) bool { return o.pierced[i] }
func (o *fakeObserver) Contest(l Layer) bool {
	o.contested = append(o.contested, l.Instance)
	return o.contestWins
}

func hideLayer(score int, instance uint64) Layer {
	return Layer{Source: SourceHide, Score: score, Instance: instance}
}

// quest-spawns.md Phase 2: a foreign quest spawn (a SourceQuestSpawn layer with
// no configured bypass, Score 0) is an existence gate that NOTHING pierces — no
// perception, no see-invisible, and no admin rank however high. It fails CLOSED
// by default.
func TestCanSee_QuestSpawn_ClosedByDefault(t *testing.T) {
	tgt := fakeTarget{id: "chip", layers: []Layer{{Source: SourceQuestSpawn}}} // Score 0 = no bypass
	// A maximally-capable observer still cannot see a foreign spawn.
	o := &fakeObserver{
		id: "bystander", piercesDark: true, seesInvisible: true,
		adminRank: 99, detectsHidden: true, contestWins: true,
	}
	if CanSee(o, tgt) {
		t.Fatal("a foreign quest spawn with no configured bypass must be invisible regardless of observer capabilities")
	}
	if len(o.contested) != 0 {
		t.Fatalf("the existence gate must not run a perception contest, got %d", len(o.contested))
	}
}

// With a staff-bypass configured (Score = the minimum admin rank), a staff
// observer of at least that rank pierces the gate while an ordinary viewer does
// not (§10 admin bypass, mirroring SourceAdminInvis).
func TestCanSee_QuestSpawn_StaffBypass(t *testing.T) {
	tgt := fakeTarget{id: "chip", layers: []Layer{{Source: SourceQuestSpawn, Score: 1}}}
	if staff := (&fakeObserver{id: "gm", adminRank: 1}); !CanSee(staff, tgt) {
		t.Fatal("a staff observer at the bypass rank must see the foreign spawn")
	}
	if pleb := (&fakeObserver{id: "pleb", adminRank: 0}); CanSee(pleb, tgt) {
		t.Fatal("an ordinary viewer must not pierce the staff-bypass gate")
	}
}

// A bypassing caller (admin inspection verb) still short-circuits before the
// quest-spawn layer is consulted (§2.1), independent of Score.
func TestCanSee_QuestSpawn_BypassStillWins(t *testing.T) {
	tgt := fakeTarget{id: "chip", layers: []Layer{{Source: SourceQuestSpawn}}}
	o := &fakeObserver{id: "admin", bypass: true}
	if !CanSee(o, tgt) {
		t.Fatal("a bypassing caller must see even a foreign quest spawn")
	}
}

// §2.3: an entity with no concealment layers is visible to every observer
// (legacy parity).
func TestCanSee_NoLayers_LegacyVisible(t *testing.T) {
	o := &fakeObserver{id: "obs"}
	tgt := fakeTarget{id: "tgt"} // no layers
	if !CanSee(o, tgt) {
		t.Fatal("an unconcealed target must be visible (legacy parity)")
	}
}

// §2.3: an observer always sees itself even while concealed.
func TestCanSee_SelfAlwaysVisible(t *testing.T) {
	o := &fakeObserver{id: "rogue"} // cannot pierce anything
	self := fakeTarget{id: "rogue", layers: []Layer{hideLayer(99, 1), {Source: SourceMagicalInvis}}}
	if !CanSee(o, self) {
		t.Fatal("an observer must always see itself, even concealed (§2.1)")
	}
}

// §2.3: a caller with Bypass set sees any target.
func TestCanSee_BypassSeesEverything(t *testing.T) {
	o := &fakeObserver{id: "admin", bypass: true} // no counters, but bypass
	tgt := fakeTarget{id: "ghost", layers: []Layer{{Source: SourceMagicalInvis}, hideLayer(50, 7)}}
	if !CanSee(o, tgt) {
		t.Fatal("a bypassing observer must see any target (§2.1)")
	}
}

// §2.3: a target with two layers is visible only to an observer that pierces
// BOTH (AND composition).
func TestCanSee_TwoLayers_RequiresBoth(t *testing.T) {
	twoLayer := fakeTarget{id: "tgt", layers: []Layer{
		{Source: SourceMagicalInvis},
		hideLayer(10, 3),
	}}

	// Pierces invis but not hide → not seen.
	onlyInvis := &fakeObserver{id: "a", seesInvisible: true, contestWins: false}
	if CanSee(onlyInvis, twoLayer) {
		t.Error("piercing only one of two layers must not reveal the target")
	}

	// Pierces hide but not invis → not seen.
	onlyHide := &fakeObserver{id: "b", seesInvisible: false, contestWins: true}
	if CanSee(onlyHide, twoLayer) {
		t.Error("piercing only the hide layer must not reveal an also-invisible target")
	}

	// Pierces both → seen.
	both := &fakeObserver{id: "c", seesInvisible: true, contestWins: true}
	if !CanSee(both, twoLayer) {
		t.Error("piercing both layers must reveal the target")
	}
}

// Flag-gated pierce rules: darkness, magical invis, admin invis.
func TestCanSee_FlagGatedSources(t *testing.T) {
	tests := []struct {
		name  string
		layer Layer
		obs   *fakeObserver
		want  bool
	}{
		{"dark unpierced", Layer{Source: SourceDarkness}, &fakeObserver{id: "o"}, false},
		{"dark with light", Layer{Source: SourceDarkness}, &fakeObserver{id: "o", piercesDark: true}, true},
		{"invis unpierced", Layer{Source: SourceMagicalInvis}, &fakeObserver{id: "o"}, false},
		{"invis with see_invisible", Layer{Source: SourceMagicalInvis}, &fakeObserver{id: "o", seesInvisible: true}, true},
		{"admin-invis lower rank", Layer{Source: SourceAdminInvis, Score: 5}, &fakeObserver{id: "o", adminRank: 4}, false},
		{"admin-invis equal rank", Layer{Source: SourceAdminInvis, Score: 5}, &fakeObserver{id: "o", adminRank: 5}, true},
		{"admin-invis greater rank", Layer{Source: SourceAdminInvis, Score: 5}, &fakeObserver{id: "o", adminRank: 9}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tgt := fakeTarget{id: "tgt", layers: []Layer{tc.layer}}
			if got := CanSee(tc.obs, tgt); got != tc.want {
				t.Errorf("CanSee = %v, want %v", got, tc.want)
			}
		})
	}
}

// Roll-gated pierce: detect trait and sticky memory skip the contest;
// otherwise the contest decides.
func TestCanSee_RollGatedPaths(t *testing.T) {
	tgt := fakeTarget{id: "tgt", layers: []Layer{hideLayer(10, 42)}}

	// detect_hidden auto-pierces without contesting.
	det := &fakeObserver{id: "o", detectsHidden: true}
	if !CanSee(det, tgt) {
		t.Error("detect_hidden must auto-pierce hide")
	}
	if len(det.contested) != 0 {
		t.Error("detect_hidden must skip the perception contest")
	}

	// Sticky memory skips the contest.
	sticky := &fakeObserver{id: "o", pierced: map[uint64]bool{42: true}}
	if !CanSee(sticky, tgt) {
		t.Error("a remembered pierce must reveal the target")
	}
	if len(sticky.contested) != 0 {
		t.Error("sticky memory must skip the perception contest")
	}

	// No trait, no memory → the contest decides (and is invoked once).
	winner := &fakeObserver{id: "o", contestWins: true}
	if !CanSee(winner, tgt) {
		t.Error("winning the contest must reveal the target")
	}
	if !slices.Equal(winner.contested, []uint64{42}) {
		t.Errorf("contest invoked for %v, want [42] exactly once", winner.contested)
	}

	loser := &fakeObserver{id: "o", contestWins: false}
	if CanSee(loser, tgt) {
		t.Error("losing the contest must hide the target")
	}
}

// Sneak resolves through the SAME roll-gated path as hide (§3.2) — guard
// against someone splitting the SourceHide/SourceSneak switch arm.
func TestCanSee_SneakUsesRollGatedPath(t *testing.T) {
	tgt := fakeTarget{id: "tgt", layers: []Layer{{Source: SourceSneak, Score: 10, Instance: 5}}}

	winner := &fakeObserver{id: "o", contestWins: true}
	if !CanSee(winner, tgt) {
		t.Error("winning the contest must reveal a sneaking target")
	}
	if !slices.Equal(winner.contested, []uint64{5}) {
		t.Errorf("sneak contest invoked for %v, want [5]", winner.contested)
	}

	loser := &fakeObserver{id: "o", contestWins: false}
	if CanSee(loser, tgt) {
		t.Error("losing the contest must hide a sneaking target")
	}

	// detect_hidden auto-pierces sneak without a contest, same as hide.
	det := &fakeObserver{id: "o", detectsHidden: true}
	if !CanSee(det, tgt) || len(det.contested) != 0 {
		t.Error("detect_hidden must auto-pierce sneak without contesting")
	}
}

// An unknown source fails open — visibility is not security (§1.2).
func TestCanSee_UnknownSourceFailsOpen(t *testing.T) {
	o := &fakeObserver{id: "o"}
	tgt := fakeTarget{id: "tgt", layers: []Layer{{Source: SourceType("future-thing")}}}
	if !CanSee(o, tgt) {
		t.Fatal("an unknown concealment source must fail open (§1.2)")
	}
}

// §2.3: Visible omits exactly the unseeable occupants and keeps the observer.
func TestVisible_FiltersAndKeepsSelf(t *testing.T) {
	o := &fakeObserver{id: "me", contestWins: false} // pierces nothing
	occupants := []fakeTarget{
		{id: "me"},    // self — always kept
		{id: "plain"}, // no layers — visible
		{id: "hidden", layers: []Layer{hideLayer(10, 1)}},            // unpierced — removed
		{id: "ghost", layers: []Layer{{Source: SourceMagicalInvis}}}, // unpierced — removed
	}
	got := Visible(o, occupants)
	var gotIDs []string
	for _, t := range got {
		gotIDs = append(gotIDs, t.id)
	}
	want := []string{"me", "plain"}
	if !slices.Equal(gotIDs, want) {
		t.Errorf("Visible = %v, want %v", gotIDs, want)
	}
}

func TestVisible_NilInputNilResult(t *testing.T) {
	o := &fakeObserver{id: "o"}
	if got := Visible[fakeTarget](o, nil); got != nil {
		t.Errorf("Visible(nil) = %v, want nil", got)
	}
}

// Visible returns nil (not an empty non-nil slice) when every target is
// concealed, so a caller may treat nil as "nothing to show" — and the
// all-concealed render path allocates nothing.
func TestVisible_AllConcealedReturnsNil(t *testing.T) {
	o := &fakeObserver{id: "me", contestWins: false} // pierces nothing
	occupants := []fakeTarget{
		{id: "a", layers: []Layer{hideLayer(10, 1)}},
		{id: "b", layers: []Layer{{Source: SourceMagicalInvis}}},
	}
	if got := Visible(o, occupants); got != nil {
		t.Errorf("Visible(all concealed) = %v, want nil", got)
	}
}

// RollGated classifies sources correctly.
func TestSourceType_RollGated(t *testing.T) {
	for _, s := range []SourceType{SourceHide, SourceSneak} {
		if !s.RollGated() {
			t.Errorf("%s should be roll-gated", s)
		}
	}
	for _, s := range []SourceType{SourceMagicalInvis, SourceAdminInvis, SourceDarkness} {
		if s.RollGated() {
			t.Errorf("%s should be flag-gated", s)
		}
	}
}
