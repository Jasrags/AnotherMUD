package progression

import (
	"context"
	"testing"
	"time"
)

// fakeEntity is a test double satisfying AlignmentEntity. Tracks
// the latest SetAlignment / SetAlignmentTag calls and the tag
// "admin" presence for HasTag.
type fakeEntity struct {
	id        string
	alignment int
	tag       string
	tags      map[string]bool
	setCalls  int
	tagCalls  int
}

func newFakeEntity(id string) *fakeEntity {
	return &fakeEntity{id: id, tags: make(map[string]bool)}
}

func (f *fakeEntity) ID() string         { return f.id }
func (f *fakeEntity) Alignment() int     { return f.alignment }
func (f *fakeEntity) SetAlignment(v int) { f.alignment = v; f.setCalls++ }
func (f *fakeEntity) SetAlignmentTag(t string) {
	f.tag = t
	f.tagCalls++
}
func (f *fakeEntity) HasTag(t string) bool { return f.tags[t] }

// captureSink records every event so tests can assert order +
// payload.
type captureSink struct {
	checks  []checkCall
	shifted []shiftedCall
	buckets []bucketCall
	rewrite int  // if non-zero, override suggestedDelta with this
	cancel  bool // if true, cancel every check
}
type checkCall struct {
	entityID, reason string
	delta            int
}
type shiftedCall struct {
	entityID, reason       string
	oldValue, newValue, dt int
	bucketChanged          bool
}
type bucketCall struct {
	entityID             string
	oldBucket, newBucket Bucket
}

func (s *captureSink) OnAlignmentShiftCheck(_ context.Context, id, reason string, d int) (int, bool) {
	s.checks = append(s.checks, checkCall{id, reason, d})
	if s.cancel {
		return d, true
	}
	if s.rewrite != 0 {
		return s.rewrite, false
	}
	return d, false
}
func (s *captureSink) OnAlignmentShifted(_ context.Context, id, reason string, ov, nv, dt int, bc bool) {
	s.shifted = append(s.shifted, shiftedCall{id, reason, ov, nv, dt, bc})
}
func (s *captureSink) OnAlignmentBucketChanged(_ context.Context, id string, ob, nb Bucket) {
	s.buckets = append(s.buckets, bucketCall{id, ob, nb})
}

func newTestManager(t *testing.T, sink AlignmentSink) *AlignmentManager {
	t.Helper()
	cfg := AlignmentConfig{
		Min: -100, Max: 100,
		EvilThreshold: -50, GoodThreshold: 50,
		HistoryCapacity: 3,
	}
	return NewAlignmentManager(cfg, sink, func() time.Time { return time.Unix(1700000000, 0) })
}

func TestAlignmentClampsOnSet(t *testing.T) {
	m := newTestManager(t, nil)
	e := newFakeEntity("p:alice")
	m.Set(context.Background(), e, 9999, "test")
	if e.alignment != 100 {
		t.Errorf("alignment = %d, want 100 (clamped to Max)", e.alignment)
	}
	m.Set(context.Background(), e, -9999, "test")
	if e.alignment != -100 {
		t.Errorf("alignment = %d, want -100 (clamped to Min)", e.alignment)
	}
}

func TestAlignmentSetIsSilent(t *testing.T) {
	sink := &captureSink{}
	m := newTestManager(t, sink)
	e := newFakeEntity("p:alice")
	m.Set(context.Background(), e, 80, "admin-grant")
	if len(sink.checks) != 0 || len(sink.shifted) != 0 || len(sink.buckets) != 0 {
		t.Errorf("Set emitted events: checks=%d shifted=%d buckets=%d", len(sink.checks), len(sink.shifted), len(sink.buckets))
	}
	if hist := m.History(e.id); len(hist) != 0 {
		t.Errorf("Set appended history: %d entries", len(hist))
	}
}

func TestAlignmentSetSyncsBucketTag(t *testing.T) {
	m := newTestManager(t, nil)
	e := newFakeEntity("m:guard")
	m.Set(context.Background(), e, 60, "")
	if e.tag != TagAlignmentGood {
		t.Errorf("tag = %q, want %q", e.tag, TagAlignmentGood)
	}
	m.Set(context.Background(), e, -60, "")
	if e.tag != TagAlignmentEvil {
		t.Errorf("tag = %q, want %q", e.tag, TagAlignmentEvil)
	}
	m.Set(context.Background(), e, 0, "")
	if e.tag != TagAlignmentNeutral {
		t.Errorf("tag = %q, want %q", e.tag, TagAlignmentNeutral)
	}
}

func TestAlignmentBucketBoundariesInclusive(t *testing.T) {
	m := newTestManager(t, nil)
	cases := []struct {
		val  int
		want Bucket
	}{
		{-50, BucketEvil},    // at threshold = evil
		{-49, BucketNeutral}, // strictly above = neutral
		{49, BucketNeutral},  // strictly below = neutral
		{50, BucketGood},     // at threshold = good
		{0, BucketNeutral},
	}
	for _, c := range cases {
		e := newFakeEntity("e")
		e.alignment = c.val
		if got := m.Bucket(e); got != c.want {
			t.Errorf("Bucket(%d) = %s, want %s", c.val, got, c.want)
		}
	}
}

func TestAlignmentShiftPublishesCheckAndShifted(t *testing.T) {
	sink := &captureSink{}
	m := newTestManager(t, sink)
	e := newFakeEntity("p:alice")
	e.alignment = 10

	res := m.Shift(context.Background(), e, 5, "kill:thief")
	if res.Cancelled {
		t.Fatal("Shift cancelled unexpectedly")
	}
	if res.AppliedDelta != 5 || res.NewValue != 15 {
		t.Errorf("res = %+v", res)
	}
	if len(sink.checks) != 1 || sink.checks[0].reason != "kill:thief" || sink.checks[0].delta != 5 {
		t.Errorf("checks = %+v", sink.checks)
	}
	if len(sink.shifted) != 1 || sink.shifted[0].newValue != 15 || sink.shifted[0].dt != 5 {
		t.Errorf("shifted = %+v", sink.shifted)
	}
	if len(sink.buckets) != 0 {
		t.Errorf("no bucket change expected; got %+v", sink.buckets)
	}
}

func TestAlignmentShiftListenerCanCancel(t *testing.T) {
	sink := &captureSink{cancel: true}
	m := newTestManager(t, sink)
	e := newFakeEntity("p:alice")
	e.alignment = 10
	res := m.Shift(context.Background(), e, 5, "test")
	if !res.Cancelled {
		t.Error("Shift returned Cancelled=false on cancel")
	}
	if e.alignment != 10 {
		t.Errorf("alignment changed despite cancel: %d", e.alignment)
	}
	if len(sink.shifted) != 0 {
		t.Errorf("shifted fired on cancel: %+v", sink.shifted)
	}
}

func TestAlignmentShiftListenerCanRewriteDelta(t *testing.T) {
	sink := &captureSink{rewrite: -3}
	m := newTestManager(t, sink)
	e := newFakeEntity("p:alice")
	e.alignment = 10
	res := m.Shift(context.Background(), e, 5, "test")
	if res.AppliedDelta != -3 || e.alignment != 7 {
		t.Errorf("rewrite ignored: res=%+v alignment=%d", res, e.alignment)
	}
}

func TestAlignmentShiftZeroResolvedDeltaNoEvent(t *testing.T) {
	sink := &captureSink{rewrite: 1} // rewrite cannot be zero by convention; use a positive…
	// Actually we want resolved=0 path. Easier: pass delta=0.
	sink.rewrite = 0
	m := newTestManager(t, sink)
	e := newFakeEntity("p:alice")
	e.alignment = 10
	res := m.Shift(context.Background(), e, 0, "test")
	if res.AppliedDelta != 0 {
		t.Errorf("AppliedDelta = %d, want 0", res.AppliedDelta)
	}
	if len(sink.shifted) != 0 {
		t.Error("alignment.shifted fired for zero delta")
	}
}

func TestAlignmentShiftAdminBypass(t *testing.T) {
	sink := &captureSink{}
	m := newTestManager(t, sink)
	e := newFakeEntity("admin-1")
	e.alignment = 0
	e.tags[AdminRoleTag] = true
	res := m.Shift(context.Background(), e, 30, "test")
	if e.alignment != 0 || res.AppliedDelta != 0 {
		t.Errorf("admin shifted: alignment=%d res=%+v", e.alignment, res)
	}
	if len(sink.checks) != 0 || len(sink.shifted) != 0 {
		t.Errorf("admin bypass did not suppress events: checks=%d shifted=%d", len(sink.checks), len(sink.shifted))
	}
}

func TestAlignmentShiftClampedNoNetChange(t *testing.T) {
	sink := &captureSink{}
	m := newTestManager(t, sink)
	e := newFakeEntity("p:alice")
	e.alignment = 100 // at Max
	res := m.Shift(context.Background(), e, 50, "test")
	if res.AppliedDelta != 0 {
		t.Errorf("AppliedDelta = %d at ceiling, want 0", res.AppliedDelta)
	}
	if len(sink.shifted) != 0 {
		t.Error("alignment.shifted fired when ceiling absorbed the shift")
	}
}

func TestAlignmentShiftBucketChange(t *testing.T) {
	sink := &captureSink{}
	m := newTestManager(t, sink)
	e := newFakeEntity("p:alice")
	e.alignment = -49 // neutral
	res := m.Shift(context.Background(), e, -5, "evil-deed")
	if !res.BucketChanged || res.OldBucket != BucketNeutral || res.NewBucket != BucketEvil {
		t.Errorf("bucket change not detected: %+v", res)
	}
	if len(sink.buckets) != 1 || sink.buckets[0].newBucket != BucketEvil {
		t.Errorf("alignment.bucket.changed not fired: %+v", sink.buckets)
	}
	if e.tag != TagAlignmentEvil {
		t.Errorf("tag = %q, want %q", e.tag, TagAlignmentEvil)
	}
}

func TestAlignmentHistoryBoundedCapacity(t *testing.T) {
	m := newTestManager(t, &captureSink{}) // capacity = 3
	e := newFakeEntity("p:alice")
	for range 5 {
		m.Shift(context.Background(), e, 1, "i")
	}
	hist := m.History(e.id)
	if len(hist) != 3 {
		t.Errorf("history len = %d, want 3 (capacity)", len(hist))
	}
	// Oldest dropped; remaining should reflect the last three shifts
	// (alignment progressed 0→1→2→3→4→5; the last 3 entries end at
	// 3,4,5 with delta 1 each).
	wantValues := []int{3, 4, 5}
	for i, e := range hist {
		if e.NewValue != wantValues[i] {
			t.Errorf("hist[%d].NewValue = %d, want %d", i, e.NewValue, wantValues[i])
		}
	}
}

func TestAlignmentResolveBuckets(t *testing.T) {
	m := newTestManager(t, nil)
	// Config: evilThreshold = -50, goodThreshold = 50
	cases := []struct {
		name           string
		set            []string
		wantMin        *int
		wantMax        *int
		minVal         int
		maxVal         int
		minSet, maxSet bool
	}{
		{"evil", []string{"evil"}, nil, new(-50), 0, -50, false, true},
		{"good", []string{"good"}, new(50), nil, 50, 0, true, false},
		{"neutral", []string{"neutral"}, new(-49), new(49), -49, 49, true, true},
		{"evil+neutral", []string{"evil", "neutral"}, nil, new(49), 0, 49, false, true},
		{"good+neutral", []string{"good", "neutral"}, new(-49), nil, -49, 0, true, false},
		{"evil+good (degenerate)", []string{"evil", "good"}, nil, nil, 0, 0, false, false},
		{"empty", nil, nil, nil, 0, 0, false, false},
		{"all three", []string{"evil", "neutral", "good"}, nil, nil, 0, 0, false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			lo, hi := m.ResolveBuckets(c.set)
			if c.minSet {
				if lo == nil || *lo != c.minVal {
					t.Errorf("min = %v, want *%d", deref(lo), c.minVal)
				}
			} else if lo != nil {
				t.Errorf("min = *%d, want nil", *lo)
			}
			if c.maxSet {
				if hi == nil || *hi != c.maxVal {
					t.Errorf("max = %v, want *%d", deref(hi), c.maxVal)
				}
			} else if hi != nil {
				t.Errorf("max = *%d, want nil", *hi)
			}
		})
	}
}

func TestAlignmentBucketAlsoSyncsTagOnQuery(t *testing.T) {
	m := newTestManager(t, nil)
	e := newFakeEntity("e")
	e.alignment = 60 // good but no tag set yet
	if got := m.Bucket(e); got != BucketGood {
		t.Errorf("Bucket = %s, want good", got)
	}
	if e.tag != TagAlignmentGood {
		t.Errorf("Bucket() did not sync tag; got %q", e.tag)
	}
}

func TestNewAlignmentManagerPanicsOnBadConfig(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("expected panic on Min > EvilThreshold")
		}
	}()
	NewAlignmentManager(AlignmentConfig{Min: 0, Max: 100, EvilThreshold: -10, GoodThreshold: 10}, nil, nil)
}

//go:fix inline
func intp(v int) *int { return new(v) }
func deref(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}
