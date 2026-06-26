package command

import "testing"

// stubRenown is a minimal renownView for the recognition-note tests.
type stubRenown struct {
	name     string
	renown   int
	tier     string
	infamous bool
}

func (s stubRenown) Name() string         { return s.name }
func (s stubRenown) EffectiveRenown() int { return s.renown }
func (s stubRenown) RenownTier() string   { return s.tier }
func (s stubRenown) Infamous() bool       { return s.infamous }

// fixedIntN is a deterministic Roller: IntN always returns n, so the recognition
// die (IntN(20)+1) is n+1.
type fixedIntN int

func (f fixedIntN) IntN(int) int { return int(f) }

// TestRecognitionLine covers reputation.md §6: the look-at-player recognition
// note. die = 9 (fixedIntN(8)); difficulty 100 throughout.
func TestRecognitionLine(t *testing.T) {
	tests := []struct {
		label string
		rep   stubRenown
		want  string
	}{
		{"unknown (renown 0) draws no note",
			stubRenown{name: "Nobody", renown: 0, tier: "Unknown"}, ""},
		{"below difficulty draws no note", // 50 + 9 = 59 < 100
			stubRenown{name: "Minor", renown: 50, tier: "Unknown"}, ""},
		{"famous names the tier", // 500 + 9 >= 100
			stubRenown{name: "Hero", renown: 500, tier: "Known in the Region"},
			"You recognize Hero — known in the region."},
		{"infamous reads as notorious",
			stubRenown{name: "Villain", renown: -500, tier: "Known Throughout the Land", infamous: true},
			"You recognize Villain — a notorious name, known throughout the land."},
		{"recognized but low tier stays plain", // 95 + 9 = 104 >= 100, tier still Unknown
			stubRenown{name: "Localish", renown: 95, tier: "Unknown"},
			"You recognize Localish."},
		{"infamous but low tier",
			stubRenown{name: "Thug", renown: 95, tier: "Unknown", infamous: true},
			"You recognize Thug, and the name sits ill with you."},
	}
	for _, tc := range tests {
		c := &Context{SkillRoller: fixedIntN(8), RecognitionDifficulty: 100}
		if got := c.recognitionLine(tc.rep); got != tc.want {
			t.Errorf("%s: recognitionLine = %q, want %q", tc.label, got, tc.want)
		}
	}
}

// A missing roller (or nil target) yields no note and never panics.
func TestRecognitionLine_NoRoller(t *testing.T) {
	c := &Context{RecognitionDifficulty: 100} // no SkillRoller
	if got := c.recognitionLine(stubRenown{name: "X", renown: 999, tier: "Known Locally"}); got != "" {
		t.Errorf("no roller should draw no note, got %q", got)
	}
}
