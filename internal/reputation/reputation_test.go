package reputation

import (
	"context"
	"strings"
	"testing"
	"time"
)

// --- test doubles ---

type fakeEntity struct {
	id     string
	renown int
	tags   map[string]bool
}

func newEntity(id string, renown int, tags ...string) *fakeEntity {
	e := &fakeEntity{id: id, renown: renown, tags: map[string]bool{}}
	for _, t := range tags {
		e.tags[t] = true
	}
	return e
}

func (f *fakeEntity) ID() string      { return f.id }
func (f *fakeEntity) Renown() int     { return f.renown }
func (f *fakeEntity) SetRenown(v int) { f.renown = v }
func (f *fakeEntity) HasTag(t string) bool {
	return f.tags[t]
}
func (f *fakeEntity) SetTierTag(tag string) {
	for t := range f.tags {
		if strings.HasPrefix(t, TierTagPrefix) {
			delete(f.tags, t)
		}
	}
	if tag != "" {
		f.tags[tag] = true
	}
}
func (f *fakeEntity) tierTags() []string {
	var out []string
	for t := range f.tags {
		if strings.HasPrefix(t, TierTagPrefix) {
			out = append(out, t)
		}
	}
	return out
}

type shiftedEvt struct {
	old, new, actual int
	tierChanged      bool
}
type tierEvt struct{ oldTier, newTier string }

type recSink struct {
	checkDelta  int  // delta the check rewrites suggestedDelta to (default: passthrough)
	cancel      bool // whether OnShiftCheck cancels
	rewrite     bool // whether to apply checkDelta
	shifted     []shiftedEvt
	tierChanges []tierEvt
}

func (s *recSink) OnShiftCheck(_ context.Context, _, _ string, suggested int) (int, bool) {
	if s.cancel {
		return suggested, true
	}
	if s.rewrite {
		return s.checkDelta, false
	}
	return suggested, false
}
func (s *recSink) OnShifted(_ context.Context, _, _ string, old, new, actual int, tierChanged bool) {
	s.shifted = append(s.shifted, shiftedEvt{old, new, actual, tierChanged})
}
func (s *recSink) OnTierChanged(_ context.Context, _, oldTier, newTier string) {
	s.tierChanges = append(s.tierChanges, tierEvt{oldTier, newTier})
}

func fixedClock() func() time.Time {
	t := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	return func() time.Time { return t }
}

// --- TierOf: magnitude-symmetric tiers (§3, PD-5) ---

func TestTierOf_MagnitudeSymmetric(t *testing.T) {
	c := DefaultConfig()
	cases := []struct {
		value int
		want  string
	}{
		{0, "Unknown"},
		{50, "Unknown"},
		{100, "Known Locally"},
		{399, "Known Locally"},
		{400, "Known in the Region"},
		{800, "Known Throughout the Land"},
		{1000, "Known Throughout the Land"},
		// infamy (negative) resolves to the symmetric tier of equal magnitude.
		{-100, "Known Locally"},
		{-800, "Known Throughout the Land"},
		{-50, "Unknown"},
	}
	for _, tc := range cases {
		if got := c.TierOf(tc.value); got != tc.want {
			t.Errorf("TierOf(%d) = %q, want %q", tc.value, got, tc.want)
		}
	}
}

func TestTierTag_Slug(t *testing.T) {
	if got := TierTag("Known in the Region"); got != "renown:known-in-the-region" {
		t.Errorf("TierTag = %q", got)
	}
	if got := TierTag(""); got != "" {
		t.Errorf("empty tier tag = %q, want empty", got)
	}
}

// --- Get / Tier (tag sync) ---

func TestTier_SyncsTag(t *testing.T) {
	m := NewManager(DefaultConfig(), nil, fixedClock())
	e := newEntity("hero", 450)
	if tier := m.Tier(e); tier != "Known in the Region" {
		t.Fatalf("Tier = %q", tier)
	}
	if tags := e.tierTags(); len(tags) != 1 || tags[0] != "renown:known-in-the-region" {
		t.Errorf("tier tags = %v, want [renown:known-in-the-region]", tags)
	}
}

// --- Set: clamps, syncs tag, NO events / NO history ---

func TestSet_NoEventsNoHistory(t *testing.T) {
	s := &recSink{}
	m := NewManager(DefaultConfig(), s, fixedClock())
	e := newEntity("hero", 0)

	m.Set(context.Background(), e, 5000, "admin") // clamps to Max 1000
	if e.renown != 1000 {
		t.Errorf("renown = %d, want clamped to 1000", e.renown)
	}
	if len(s.shifted) != 0 || len(s.tierChanges) != 0 {
		t.Errorf("Set emitted events: shifted=%d tier=%d", len(s.shifted), len(s.tierChanges))
	}
	if h := m.History("hero"); len(h) != 0 {
		t.Errorf("Set appended history: %v", h)
	}
	if tags := e.tierTags(); len(tags) != 1 || tags[0] != "renown:known-throughout-the-land" {
		t.Errorf("tier tag after Set = %v", tags)
	}
}

// --- Shift: applies, clamps, emits, crosses tier ---

func TestShift_AppliesAndEmits(t *testing.T) {
	s := &recSink{}
	m := NewManager(DefaultConfig(), s, fixedClock())
	e := newEntity("hero", 50) // Unknown

	res := m.Shift(context.Background(), e, 100, "deed") // → 150, Known Locally
	if res.NewValue != 150 || res.AppliedDelta != 100 {
		t.Fatalf("res = %+v", res)
	}
	if !res.TierChanged || res.OldTier != "Unknown" || res.NewTier != "Known Locally" {
		t.Errorf("tier change = %+v", res)
	}
	if len(s.shifted) != 1 || s.shifted[0] != (shiftedEvt{50, 150, 100, true}) {
		t.Errorf("shifted = %+v", s.shifted)
	}
	if len(s.tierChanges) != 1 || s.tierChanges[0] != (tierEvt{"Unknown", "Known Locally"}) {
		t.Errorf("tierChanges = %+v", s.tierChanges)
	}
	if h := m.History("hero"); len(h) != 1 || h[0].NewValue != 150 || h[0].NewTier != "Known Locally" {
		t.Errorf("history = %+v", h)
	}
}

func TestShift_ClampsAtMax(t *testing.T) {
	s := &recSink{}
	m := NewManager(DefaultConfig(), s, fixedClock())
	e := newEntity("hero", 950)
	res := m.Shift(context.Background(), e, 500, "deed") // clamps to 1000
	if res.NewValue != 1000 || res.AppliedDelta != 50 {
		t.Errorf("res = %+v, want clamp to 1000 (applied 50)", res)
	}
}

func TestShift_NoTierChangeWithinBand(t *testing.T) {
	s := &recSink{}
	m := NewManager(DefaultConfig(), s, fixedClock())
	e := newEntity("hero", 100)                         // Known Locally
	res := m.Shift(context.Background(), e, 50, "deed") // → 150, still Known Locally
	if res.TierChanged {
		t.Errorf("tier should not change within a band: %+v", res)
	}
	if len(s.tierChanges) != 0 {
		t.Errorf("no tier event expected, got %+v", s.tierChanges)
	}
}

func TestShift_CancelledLeavesEverythingUnchanged(t *testing.T) {
	s := &recSink{cancel: true}
	m := NewManager(DefaultConfig(), s, fixedClock())
	e := newEntity("hero", 200)
	res := m.Shift(context.Background(), e, 100, "deed")
	if !res.Cancelled || e.renown != 200 {
		t.Errorf("cancel: res=%+v renown=%d", res, e.renown)
	}
	if len(s.shifted) != 0 || len(m.History("hero")) != 0 {
		t.Errorf("cancelled shift left side effects")
	}
}

func TestShift_CheckRewritesDelta(t *testing.T) {
	s := &recSink{rewrite: true, checkDelta: 25} // a listener scales the gain down
	m := NewManager(DefaultConfig(), s, fixedClock())
	e := newEntity("hero", 0)
	res := m.Shift(context.Background(), e, 100, "deed")
	if res.AppliedDelta != 25 || e.renown != 25 {
		t.Errorf("rewrite: applied=%d renown=%d, want 25/25", res.AppliedDelta, e.renown)
	}
}

func TestShift_AdminImmune(t *testing.T) {
	s := &recSink{}
	m := NewManager(DefaultConfig(), s, fixedClock())
	e := newEntity("staff", 0, AdminRoleTag)
	res := m.Shift(context.Background(), e, 500, "deed")
	if e.renown != 0 || res.AppliedDelta != 0 {
		t.Errorf("admin should be renown-immune: renown=%d res=%+v", e.renown, res)
	}
	if len(s.shifted) != 0 {
		t.Errorf("admin shift emitted events")
	}
}

// --- Recognition check (§6) ---

func TestCheck_Recognition(t *testing.T) {
	m := NewManager(DefaultConfig(), nil, fixedClock())

	// Unknown (renown 0) auto-fails — cannot be recognized.
	if m.Check(newEntity("nobody", 0), 100, 10) {
		t.Error("renown 0 must auto-fail recognition")
	}
	// Famous: magnitude 200 + die 10 >= difficulty 150 → recognized.
	if !m.Check(newEntity("hero", 200), 10, 150) {
		t.Error("renowned character should pass an easy check")
	}
	// Infamous: magnitude works on |value| too.
	if !m.Check(newEntity("villain", -200), 10, 150) {
		t.Error("infamous character (negative renown) should be recognized by magnitude")
	}
	// Too obscure for a hard check.
	if m.Check(newEntity("minor", 50), 5, 200) {
		t.Error("low renown should fail a hard check")
	}
}

// TestRecognized covers the pure recognition rule directly (the reusable
// primitive effective-renown consumers call without an Entity).
func TestRecognized(t *testing.T) {
	cases := []struct {
		name                    string
		renown, die, difficulty int
		want                    bool
	}{
		{"zero renown never recognized", 0, 100, 1, false},
		{"fame passes when magnitude+die clears difficulty", 200, 10, 150, true},
		{"infamy (negative) recognized by magnitude", -200, 10, 150, true},
		{"obscure fails a hard check", 50, 5, 200, false},
		{"exactly meets difficulty", 90, 10, 100, true},
	}
	for _, tc := range cases {
		if got := Recognized(tc.renown, tc.die, tc.difficulty); got != tc.want {
			t.Errorf("%s: Recognized(%d,%d,%d) = %v, want %v",
				tc.name, tc.renown, tc.die, tc.difficulty, got, tc.want)
		}
	}
}

// --- History bounded ---

func TestHistory_BoundedFIFO(t *testing.T) {
	cfg := DefaultConfig()
	cfg.HistoryCapacity = 3
	m := NewManager(cfg, &recSink{}, fixedClock())
	e := newEntity("hero", 0)
	for range 5 {
		m.Shift(context.Background(), e, 10, "deed")
	}
	h := m.History("hero")
	if len(h) != 3 {
		t.Fatalf("history len = %d, want 3 (capped)", len(h))
	}
	// Oldest dropped: the three kept end at 50 (the last three of 10,20,30,40,50).
	if h[0].NewValue != 30 || h[2].NewValue != 50 {
		t.Errorf("ring = [%d..%d], want 30..50", h[0].NewValue, h[2].NewValue)
	}
}

// --- zero-value Config is safe (normalize) ---

func TestNewManager_NormalizesZeroConfig(t *testing.T) {
	m := NewManager(Config{}, nil, fixedClock())
	c := m.Config()
	if len(c.Ladder) == 0 || c.Max != 1000 || c.HistoryCapacity < 1 {
		t.Errorf("zero config not normalized: %+v", c)
	}
}
