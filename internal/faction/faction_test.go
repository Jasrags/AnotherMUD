package faction

import (
	"context"
	"strings"
	"testing"
	"time"
)

// --- test doubles ---

type fakeEntity struct {
	id       string
	standing map[string]int
	tags     map[string]bool
}

func newEntity(id string, tags ...string) *fakeEntity {
	e := &fakeEntity{id: id, standing: map[string]int{}, tags: map[string]bool{}}
	for _, t := range tags {
		e.tags[t] = true
	}
	return e
}

func (f *fakeEntity) ID() string { return f.id }
func (f *fakeEntity) Standing(fid string) (int, bool) {
	v, ok := f.standing[fid]
	return v, ok
}
func (f *fakeEntity) SetStanding(fid string, v int) { f.standing[fid] = v }
func (f *fakeEntity) SetRankTag(fid, tag string) {
	prefix := RankTagPrefix(fid)
	for t := range f.tags {
		if strings.HasPrefix(t, prefix) {
			delete(f.tags, t)
		}
	}
	if tag != "" {
		f.tags[tag] = true
	}
}
func (f *fakeEntity) HasTag(t string) bool { return f.tags[t] }
func (f *fakeEntity) rankTags(fid string) []string {
	prefix := RankTagPrefix(fid)
	var out []string
	for t := range f.tags {
		if strings.HasPrefix(t, prefix) {
			out = append(out, t)
		}
	}
	return out
}

type shiftedEvt struct {
	factionID        string
	old, new, actual int
	rankChanged      bool
}
type rankEvt struct{ factionID, oldRank, newRank string }

type recordingSink struct {
	rewriteTo *int
	cancel    bool

	checks      int
	shifted     []shiftedEvt
	rankChanged []rankEvt
}

func (s *recordingSink) OnShiftCheck(_ context.Context, _, _, _ string, d int) (int, bool) {
	s.checks++
	if s.cancel {
		return d, true
	}
	if s.rewriteTo != nil {
		return *s.rewriteTo, false
	}
	return d, false
}
func (s *recordingSink) OnShifted(_ context.Context, _, fid, _ string, o, n, a int, rc bool) {
	s.shifted = append(s.shifted, shiftedEvt{fid, o, n, a, rc})
}
func (s *recordingSink) OnRankChanged(_ context.Context, _, fid, or, nr string) {
	s.rankChanged = append(s.rankChanged, rankEvt{fid, or, nr})
}

func fixedClock() func() time.Time {
	t := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	return func() time.Time { return t }
}

// --- §2.1 definition defaults ---

func TestRegistry_DefaultsFillOmittedFields(t *testing.T) {
	r := NewRegistry(DefaultConfig())
	// No ladder, no bounds, no starting → all defaults.
	def := r.Add(Definition{ID: "core:watch", Name: "City Watch"})
	if len(def.Ranks) != 6 {
		t.Fatalf("expected default 6-rank ladder, got %d", len(def.Ranks))
	}
	if def.Min != -1000 || def.Max != 1000 {
		t.Errorf("expected default bounds ±1000, got [%d,%d]", def.Min, def.Max)
	}
	if def.Starting != 0 {
		t.Errorf("expected default starting 0, got %d", def.Starting)
	}
}

func TestRegistry_ExplicitLadderAndBounds(t *testing.T) {
	r := NewRegistry(DefaultConfig())
	def := r.AddWithFlags(Definition{
		ID:       "core:tower",
		Ranks:    []Rank{{"Shunned", -100}, {"Known", 0}, {"Trusted", 50}},
		Min:      -100,
		Max:      100,
		Starting: -20,
	}, true, true, true)
	if def.Min != -100 || def.Max != 100 {
		t.Errorf("explicit bounds not honored: [%d,%d]", def.Min, def.Max)
	}
	if def.Starting != -20 {
		t.Errorf("explicit starting not honored: %d", def.Starting)
	}
	if def.RankOf(-20) != "Shunned" || def.RankOf(0) != "Known" || def.RankOf(75) != "Trusted" {
		t.Errorf("custom ladder ranks wrong: %q %q %q", def.RankOf(-20), def.RankOf(0), def.RankOf(75))
	}
}

// --- §3.5 model ---

func TestManager_UntouchedReadsStarting(t *testing.T) {
	r := NewRegistry(DefaultConfig())
	def := r.AddWithFlags(Definition{ID: "core:guild", Starting: 100}, false, false, true)
	m := NewManager(r, nil, fixedClock())
	e := newEntity("p1")
	if got := m.Get(e, def); got != 100 {
		t.Errorf("untouched Get = %d, want starting 100", got)
	}
}

func TestManager_RankAndTagSync(t *testing.T) {
	r := NewRegistry(DefaultConfig())
	def := r.Add(Definition{ID: "core:watch"})
	m := NewManager(r, nil, fixedClock())
	e := newEntity("p1")

	// Neutral at start.
	if rk := m.Rank(e, def); rk != "Neutral" {
		t.Fatalf("start rank = %q, want Neutral", rk)
	}
	if tags := e.rankTags("core:watch"); len(tags) != 1 || tags[0] != "faction:core:watch:Neutral" {
		t.Fatalf("rank tag = %v, want [faction:core:watch:Neutral]", tags)
	}

	// Shift up into Friendly; exactly one tag, prior removed.
	m.Shift(context.Background(), e, def, 350, "test")
	if rk := m.Rank(e, def); rk != "Friendly" {
		t.Fatalf("rank after +350 = %q, want Friendly", rk)
	}
	if tags := e.rankTags("core:watch"); len(tags) != 1 || tags[0] != "faction:core:watch:Friendly" {
		t.Fatalf("rank tag after shift = %v, want single Friendly tag", tags)
	}
}

func TestManager_ClampToBounds(t *testing.T) {
	r := NewRegistry(DefaultConfig())
	def := r.Add(Definition{ID: "core:watch"})
	m := NewManager(r, nil, fixedClock())
	e := newEntity("p1")
	m.Shift(context.Background(), e, def, 5000, "huge")
	if got := m.Get(e, def); got != 1000 {
		t.Errorf("clamp at max: got %d, want 1000", got)
	}
}

// --- §4.2 operations ---

func TestSet_IsSilentNoHistory(t *testing.T) {
	r := NewRegistry(DefaultConfig())
	def := r.Add(Definition{ID: "core:watch"})
	sink := &recordingSink{}
	m := NewManager(r, sink, fixedClock())
	e := newEntity("p1")

	m.Set(context.Background(), e, def, 800, "admin")
	if got := m.Get(e, def); got != 800 {
		t.Errorf("Set value = %d, want 800", got)
	}
	if rk := def.RankOf(800); rk != "Honored" || len(e.rankTags("core:watch")) != 1 {
		t.Errorf("Set did not sync rank tag (rank %q, tags %v)", rk, e.rankTags("core:watch"))
	}
	if len(sink.shifted) != 0 || sink.checks != 0 {
		t.Errorf("Set emitted events: checks=%d shifted=%d", sink.checks, len(sink.shifted))
	}
	if h := m.History("p1"); len(h) != 0 {
		t.Errorf("Set appended history: %d entries", len(h))
	}
}

func TestShift_AdminBypass(t *testing.T) {
	r := NewRegistry(DefaultConfig())
	def := r.Add(Definition{ID: "core:watch"})
	sink := &recordingSink{}
	m := NewManager(r, sink, fixedClock())
	e := newEntity("admin1", AdminRoleTag)
	res := m.Shift(context.Background(), e, def, -500, "kill")
	if res.AppliedDelta != 0 || sink.checks != 0 || len(sink.shifted) != 0 {
		t.Errorf("admin shift was not a no-op: %+v checks=%d", res, sink.checks)
	}
	if got := m.Get(e, def); got != 0 {
		t.Errorf("admin standing moved to %d, want 0", got)
	}
}

func TestShift_CancelledProducesNoShift(t *testing.T) {
	r := NewRegistry(DefaultConfig())
	def := r.Add(Definition{ID: "core:watch"})
	sink := &recordingSink{cancel: true}
	m := NewManager(r, sink, fixedClock())
	e := newEntity("p1")
	res := m.Shift(context.Background(), e, def, 400, "quest")
	if !res.Cancelled {
		t.Errorf("expected Cancelled")
	}
	if len(sink.shifted) != 0 {
		t.Errorf("cancelled shift still emitted shifted")
	}
	if got := m.Get(e, def); got != 0 {
		t.Errorf("cancelled shift moved standing to %d", got)
	}
}

func TestShift_DeltaRewritable(t *testing.T) {
	r := NewRegistry(DefaultConfig())
	def := r.Add(Definition{ID: "core:watch"})
	doubled := 800
	sink := &recordingSink{rewriteTo: &doubled}
	m := NewManager(r, sink, fixedClock())
	e := newEntity("p1")
	res := m.Shift(context.Background(), e, def, 400, "quest")
	if res.AppliedDelta != 800 {
		t.Errorf("listener rewrite ignored: applied %d, want 800", res.AppliedDelta)
	}
}

func TestShift_EventsAndRankChange(t *testing.T) {
	r := NewRegistry(DefaultConfig())
	def := r.Add(Definition{ID: "core:watch"})
	sink := &recordingSink{}
	m := NewManager(r, sink, fixedClock())
	e := newEntity("p1")

	// +350 crosses Neutral → Friendly.
	m.Shift(context.Background(), e, def, 350, "quest")
	if len(sink.shifted) != 1 || !sink.shifted[0].rankChanged {
		t.Fatalf("expected one shifted with rankChanged, got %+v", sink.shifted)
	}
	if len(sink.rankChanged) != 1 || sink.rankChanged[0].oldRank != "Neutral" || sink.rankChanged[0].newRank != "Friendly" {
		t.Fatalf("rank.changed wrong: %+v", sink.rankChanged)
	}

	// +50 stays within Friendly — shifted fires, rank.changed does NOT.
	m.Shift(context.Background(), e, def, 50, "quest")
	if len(sink.shifted) != 2 || sink.shifted[1].rankChanged {
		t.Fatalf("intra-rank shift should not flag rankChanged: %+v", sink.shifted)
	}
	if len(sink.rankChanged) != 1 {
		t.Fatalf("intra-rank shift emitted an extra rank.changed: %+v", sink.rankChanged)
	}

	// History records both, with faction id.
	h := m.History("p1")
	if len(h) != 2 || h[0].FactionID != "core:watch" || h[1].NewValue != 400 {
		t.Fatalf("history wrong: %+v", h)
	}
}

func TestShift_CombinedHistoryAcrossFactions(t *testing.T) {
	r := NewRegistry(DefaultConfig())
	watch := r.Add(Definition{ID: "core:watch"})
	guild := r.Add(Definition{ID: "core:guild"})
	m := NewManager(r, nil, fixedClock())
	e := newEntity("p1")
	m.Shift(context.Background(), e, watch, 100, "a")
	m.Shift(context.Background(), e, guild, -50, "b")
	h := m.History("p1")
	if len(h) != 2 {
		t.Fatalf("combined history = %d, want 2", len(h))
	}
	if h[0].FactionID != "core:watch" || h[1].FactionID != "core:guild" {
		t.Errorf("combined history not carrying per-record faction ids: %+v", h)
	}
}

func TestHistory_BoundedFIFO(t *testing.T) {
	cfg := DefaultConfig()
	cfg.HistoryCapacity = 3
	r := NewRegistry(cfg)
	def := r.Add(Definition{ID: "core:watch"})
	m := NewManager(r, nil, fixedClock())
	e := newEntity("p1")
	for range 5 {
		m.Shift(context.Background(), e, def, 10, "x")
	}
	h := m.History("p1")
	if len(h) != 3 {
		t.Fatalf("history cap not enforced: %d, want 3", len(h))
	}
	// Oldest dropped: remaining new values are 30,40,50.
	if h[0].NewValue != 30 || h[2].NewValue != 50 {
		t.Errorf("FIFO trim wrong: %d..%d", h[0].NewValue, h[2].NewValue)
	}
}

// --- §6.1 gating ---

func TestResolveRanks(t *testing.T) {
	r := NewRegistry(DefaultConfig())
	def := r.Add(Definition{ID: "core:watch"}) // Hostile -1000, Unfriendly -300, Neutral 0, Friendly 300, Honored 700, Allied 900
	m := NewManager(r, nil, fixedClock())

	ptr := func(p *int) string {
		if p == nil {
			return "nil"
		}
		return itoa(*p)
	}
	cases := []struct {
		name    string
		ranks   []string
		wantMin string
		wantMax string
	}{
		{"bottom rank open below", []string{"Hostile"}, "nil", "-301"},
		{"top rank open above", []string{"Allied"}, "900", "nil"},
		{"middle single", []string{"Friendly"}, "300", "699"},
		{"friendly and above", []string{"Friendly", "Honored", "Allied"}, "300", "nil"},
		{"whole ladder", []string{"Hostile", "Unfriendly", "Neutral", "Friendly", "Honored", "Allied"}, "nil", "nil"},
		{"empty set", nil, "nil", "nil"},
		{"unknown name", []string{"Bogus"}, "nil", "nil"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			lo, hi := m.ResolveRanks(def, c.ranks)
			if ptr(lo) != c.wantMin || ptr(hi) != c.wantMax {
				t.Errorf("ResolveRanks(%v) = (%s,%s), want (%s,%s)", c.ranks, ptr(lo), ptr(hi), c.wantMin, c.wantMax)
			}
		})
	}
}

func TestMeetsStanding(t *testing.T) {
	r := NewRegistry(DefaultConfig())
	def := r.Add(Definition{ID: "core:watch"})
	m := NewManager(r, nil, fixedClock())
	e := newEntity("p1")
	m.Set(context.Background(), e, def, 300, "seed") // Friendly floor
	if !m.MeetsStanding(e, def, 300) {
		t.Errorf("MeetsStanding should admit at the threshold")
	}
	if m.MeetsStanding(e, def, 301) {
		t.Errorf("MeetsStanding should refuse just above")
	}
}

// itoa avoids importing strconv just for the test table formatting.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
