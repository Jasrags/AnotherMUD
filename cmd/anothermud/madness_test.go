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
