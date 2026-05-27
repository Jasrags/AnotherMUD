package progression

import (
	"context"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/combat"
)

// fixedRoller returns a constant value for every die rolled — keeps
// stat-growth tests deterministic without mocking math/rand.
type fixedRoller struct{ v int }

func (f fixedRoller) IntN(n int) int { return f.v % n }

// captureGranter records every (entityID, abilityID) it was asked
// to teach and returns the configured "name" + ok flag.
type captureGranter struct {
	known   map[string]string // abilityID → display name
	calls   []string          // ability ids in call order
	missing []string          // ability ids that returned ok=false
}

func (g *captureGranter) Teach(_ context.Context, _, ability string) (string, bool) {
	g.calls = append(g.calls, ability)
	if name, ok := g.known[ability]; ok {
		return name, true
	}
	g.missing = append(g.missing, ability)
	return "", false
}

// captureNotifier records every notification emitted.
type captureNotifier struct {
	msgs []string
}

func (n *captureNotifier) Notify(_ context.Context, _, msg string) {
	n.msgs = append(n.msgs, msg)
}

// captureTrains records each trains credit.
type captureTrains struct {
	total int
	calls int
}

func (c *captureTrains) CreditTrains(_ context.Context, _ string, n int) {
	c.total += n
	c.calls++
}

func TestClassPathProcessor_TeachOnMatchingLevel(t *testing.T) {
	r := NewClassRegistry()
	_ = r.Register(&Class{
		ID: "fighter", BoundTrack: "adventurer",
		Path: []ClassPathEntry{
			{Level: 1, AbilityID: "slash"},
			{Level: 1, AbilityID: "guard"},
			{Level: 2, AbilityID: "cleave"},
			{Level: 3, AbilityID: "quest-locked", UnlockedVia: "quest:reclaim-anvil"},
			{Level: 3, AbilityID: "double-strike"},
		},
	})
	g := &captureGranter{known: map[string]string{"slash": "Slash", "guard": "Guard", "cleave": "Cleave", "double-strike": "Double Strike"}}
	n := &captureNotifier{}
	p := ClassPathProcessor{Classes: r, Granter: g, Notifier: n}

	// Level 1 grants both level-1 entries.
	p.Apply(context.Background(), "p:alice", "fighter", "adventurer", 1)
	if got, want := g.calls, []string{"slash", "guard"}; !equalStrings(got, want) {
		t.Errorf("level 1 calls = %v, want %v", got, want)
	}
	if len(n.msgs) != 2 || n.msgs[0] != "You have learned Slash!" {
		t.Errorf("level 1 msgs unexpected: %v", n.msgs)
	}

	// Level 3 skips the quest-unlocked entry.
	g.calls = nil
	n.msgs = nil
	p.Apply(context.Background(), "p:alice", "fighter", "adventurer", 3)
	if got, want := g.calls, []string{"double-strike"}; !equalStrings(got, want) {
		t.Errorf("level 3 calls = %v, want %v", got, want)
	}
}

func TestClassPathProcessor_TrackGateMismatch(t *testing.T) {
	r := NewClassRegistry()
	_ = r.Register(&Class{
		ID: "fighter", BoundTrack: "adventurer",
		Path: []ClassPathEntry{{Level: 1, AbilityID: "slash"}},
	})
	g := &captureGranter{known: map[string]string{"slash": "Slash"}}
	p := ClassPathProcessor{Classes: r, Granter: g, Notifier: &captureNotifier{}}

	// Different track — no calls.
	p.Apply(context.Background(), "p:alice", "fighter", "crafting", 1)
	if len(g.calls) != 0 {
		t.Errorf("track mismatch should skip; got calls %v", g.calls)
	}
	// Empty track ("character.created") — no gate, fires.
	p.Apply(context.Background(), "p:alice", "fighter", "", 1)
	if len(g.calls) != 1 {
		t.Errorf("empty track should not gate; got calls %v", g.calls)
	}
	// Case-insensitive match.
	g.calls = nil
	p.Apply(context.Background(), "p:alice", "fighter", "ADVENTURER", 1)
	if len(g.calls) != 1 {
		t.Errorf("case-insens track match failed; got calls %v", g.calls)
	}
}

func TestClassPathProcessor_UnknownAbilityLogsAndSkips(t *testing.T) {
	r := NewClassRegistry()
	_ = r.Register(&Class{
		ID: "fighter", BoundTrack: "adventurer",
		Path: []ClassPathEntry{
			{Level: 1, AbilityID: "missing-ability"},
			{Level: 1, AbilityID: "slash"},
		},
	})
	g := &captureGranter{known: map[string]string{"slash": "Slash"}}
	n := &captureNotifier{}
	p := ClassPathProcessor{Classes: r, Granter: g, Notifier: n}
	p.Apply(context.Background(), "p:alice", "fighter", "adventurer", 1)

	if len(g.missing) != 1 || g.missing[0] != "missing-ability" {
		t.Errorf("missing ability not recorded: %v", g.missing)
	}
	if len(n.msgs) != 1 || n.msgs[0] != "You have learned Slash!" {
		t.Errorf("notify ran for unknown ability or missed valid one: %v", n.msgs)
	}
}

func TestClassPathProcessor_NoBoundTrack(t *testing.T) {
	r := NewClassRegistry()
	_ = r.Register(&Class{
		ID: "ascetic", BoundTrack: "",
		Path: []ClassPathEntry{{Level: 1, AbilityID: "meditate"}},
	})
	g := &captureGranter{known: map[string]string{"meditate": "Meditate"}}
	p := ClassPathProcessor{Classes: r, Granter: g, Notifier: &captureNotifier{}}
	p.Apply(context.Background(), "p:alice", "ascetic", "", 1)
	if len(g.calls) != 0 {
		t.Errorf("class with no bound track should skip path: %v", g.calls)
	}
}

func TestClassPathProcessor_EmptyClassID(t *testing.T) {
	r := NewClassRegistry()
	p := ClassPathProcessor{Classes: r, Granter: &captureGranter{}}
	p.Apply(context.Background(), "p:alice", "", "adventurer", 1)
	// no panic, nothing fired.
}

func TestApplyStatGrowth_RollsDiceAndAppliesBonus(t *testing.T) {
	cls := &Class{
		ID: "fighter", BoundTrack: "adventurer",
		StatGrowth:     map[StatType]combat.DiceExpr{StatHPMax: {Count: 1, Sides: 8}, StatSTR: {Count: 1, Sides: 4}},
		GrowthBonuses:  map[StatType]StatType{StatHPMax: StatCON},
		TrainsPerLevel: 5,
	}
	sb := NewWithBase(map[StatType]int{
		StatHPMax: 20,
		StatSTR:   10,
		StatCON:   14, // +2 modifier
	})
	// fixedRoller{v: 0} -> IntN(N) = 0 -> every die contributes 1 (per damage.go Roll)
	// so 1d8 = 1, +2 CON bonus -> hp_max delta = 3
	// 1d4 = 1, str delta = 1
	roller := fixedRoller{v: 0}
	tc := &captureTrains{}
	deltas := ApplyStatGrowth(context.Background(), cls, sb, roller, tc, "p:alice")

	if deltas[StatHPMax] != 3 {
		t.Errorf("hp_max delta = %d, want 3 (1 die + 2 con bonus)", deltas[StatHPMax])
	}
	if deltas[StatSTR] != 1 {
		t.Errorf("str delta = %d, want 1", deltas[StatSTR])
	}
	if sb.Base(StatHPMax) != 23 {
		t.Errorf("hp_max base = %d, want 23", sb.Base(StatHPMax))
	}
	if sb.Base(StatSTR) != 11 {
		t.Errorf("str base = %d, want 11", sb.Base(StatSTR))
	}
	if tc.total != 5 {
		t.Errorf("trains credited = %d, want 5", tc.total)
	}
}

func TestApplyStatGrowth_LowConGivesZeroBonus(t *testing.T) {
	// CON=8 → (8-10)/2 = -1 → clamped to 0.
	cls := &Class{
		StatGrowth:    map[StatType]combat.DiceExpr{StatHPMax: {Count: 1, Sides: 6}},
		GrowthBonuses: map[StatType]StatType{StatHPMax: StatCON},
	}
	sb := NewWithBase(map[StatType]int{StatHPMax: 20, StatCON: 8})
	deltas := ApplyStatGrowth(context.Background(), cls, sb, fixedRoller{}, nil, "p:bob")
	if deltas[StatHPMax] != 1 {
		t.Errorf("hp_max delta with low CON = %d, want 1", deltas[StatHPMax])
	}
}

func TestApplyStatGrowth_NilSafe(t *testing.T) {
	if got := ApplyStatGrowth(context.Background(), nil, New(), fixedRoller{}, nil, "x"); got != nil {
		t.Errorf("nil class returned non-nil: %v", got)
	}
	if got := ApplyStatGrowth(context.Background(), &Class{}, nil, fixedRoller{}, nil, "x"); got != nil {
		t.Errorf("nil statblock returned non-nil: %v", got)
	}
}

func TestApplyStatGrowth_CreditsTrainsOnlyWhenPositive(t *testing.T) {
	tc := &captureTrains{}
	cls := &Class{TrainsPerLevel: 0}
	ApplyStatGrowth(context.Background(), cls, New(), fixedRoller{}, tc, "x")
	if tc.calls != 0 {
		t.Errorf("trains credited for zero per-level: %d", tc.calls)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i, v := range a {
		if v != b[i] {
			return false
		}
	}
	return true
}
