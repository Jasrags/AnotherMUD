package command

import (
	"strings"
	"testing"
)

// The score sheet's saidin-taint row (WoT S2 Phase 4+): shown only once accrued,
// with the number coarsened into an ominous band so it reads as dread.

func TestMadnessBand(t *testing.T) {
	cases := []struct {
		madness int
		want    string
	}{
		{1, "faint whisper"},
		{24, "faint whisper"},
		{25, "shadow on your mind"},
		{49, "shadow on your mind"},
		{50, "voices clamor"},
		{74, "voices clamor"},
		{75, "madness has you"},
		{200, "madness has you"},
	}
	for _, tc := range cases {
		if got := madnessBand(tc.madness); !strings.Contains(got, tc.want) {
			t.Errorf("madnessBand(%d) = %q, want substring %q", tc.madness, got, tc.want)
		}
	}
}

func TestRenderScore_MadnessRowGatedOnAccrual(t *testing.T) {
	base := scoreData{Name: "Lews", HasStats: true}

	// No taint → no Madness row at all (a fighter / a clean channeler / a woman).
	if out := renderScore(base); strings.Contains(strings.ToLower(out), "madness") {
		t.Errorf("clean sheet showed a Madness row:\n%s", out)
	}

	// Tainted → the row appears with the band label.
	tainted := base
	tainted.HasMadness = true
	tainted.Madness = 60
	out := renderScore(tainted)
	if !strings.Contains(strings.ToLower(out), "madness") {
		t.Errorf("tainted sheet missing the Madness row:\n%s", out)
	}
	if !strings.Contains(out, "voices clamor") {
		t.Errorf("Madness row missing the band label for 60:\n%s", out)
	}
}
