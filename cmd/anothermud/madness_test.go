package main

import "testing"

// madnessManifestation escalates the inflicted condition by band as saidin taint
// deepens (WoT S2 Phase 4+): a faint whisper merely tires, a shadow brings
// terror, the clamor of voices whites the world out.
func TestMadnessManifestation(t *testing.T) {
	cases := []struct {
		madness    int
		wantEffect string
	}{
		{25, "fatigued"},
		{49, "fatigued"},
		{50, "frightened"},
		{74, "frightened"},
		{75, "stunned"},
		{500, "stunned"},
	}
	for _, tc := range cases {
		got, msg := madnessManifestation(tc.madness)
		if got != tc.wantEffect {
			t.Errorf("madnessManifestation(%d) effect = %q, want %q", tc.madness, got, tc.wantEffect)
		}
		if msg == "" {
			t.Errorf("madnessManifestation(%d) returned an empty cue", tc.madness)
		}
	}
}

// Mental Stability raises the manifestation floor only when the channeler has
// the feat (WoT S2 Phase 4+).
func TestEffectiveMadnessThreshold(t *testing.T) {
	if got := effectiveMadnessThreshold(25, false, 25); got != 25 {
		t.Errorf("no feat: threshold = %d, want 25 (unchanged)", got)
	}
	if got := effectiveMadnessThreshold(25, true, 25); got != 50 {
		t.Errorf("with Mental Stability: threshold = %d, want 50 (base + bonus)", got)
	}
	if got := effectiveMadnessThreshold(25, true, 0); got != 25 {
		t.Errorf("zero bonus makes the feat inert: threshold = %d, want 25", got)
	}
}
