package main

import "testing"

func TestAffinityPotency(t *testing.T) {
	const weak = 0.5
	cases := []struct {
		name     string
		gender   string
		elements []string
		want     float64
	}{
		// Female: strong in air/water/spirit.
		{"female firebolt (fire) weak", "female", []string{"fire"}, weak},
		{"female healing (water) strong", "female", []string{"water"}, 1.0},
		{"female warding (air+spirit) strong", "female", []string{"air", "spirit"}, 1.0},
		{"female bonds (air) strong", "female", []string{"air"}, 1.0},
		// Male: strong in earth/fire/spirit.
		{"male firebolt (fire) strong", "male", []string{"fire"}, 1.0},
		{"male healing (water) weak", "male", []string{"water"}, weak},
		{"male warding (air+spirit) weak (air)", "male", []string{"air", "spirit"}, weak},
		{"male bonds (air) weak", "male", []string{"air"}, weak},
		// Weakest element governs a multi-element weave.
		{"female mixed strong+weak → weak", "female", []string{"water", "fire"}, weak},
		// Safe degradation: no elements / unset / unknown gender → full.
		{"no elements → full", "female", nil, 1.0},
		{"unset gender → full", "", []string{"fire"}, 1.0},
		{"unknown gender → full", "draghkar", []string{"fire"}, 1.0},
		// Normalization: case + whitespace.
		{"case/space normalized", "Female", []string{"  WATER  "}, 1.0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := affinityPotency(tc.gender, tc.elements, weak); got != tc.want {
				t.Errorf("affinityPotency(%q, %v) = %v, want %v", tc.gender, tc.elements, got, tc.want)
			}
		})
	}
}

func TestScaleByPotency(t *testing.T) {
	cases := []struct {
		amount  int
		potency float64
		want    int
	}{
		{8, 1.0, 8},  // full potency unchanged
		{8, 1.5, 8},  // >1 never amplifies here (guard)
		{8, 0.5, 4},  // halved
		{5, 0.5, 3},  // 2.5 rounds to 3
		{1, 0.5, 1},  // 0.5 rounds to 1
		{3, 0.5, 2},  // 1.5 rounds to 2
		{2, 0.25, 1}, // 0.5 rounds to 1
	}
	for _, tc := range cases {
		if got := scaleByPotency(tc.amount, tc.potency); got != tc.want {
			t.Errorf("scaleByPotency(%d, %v) = %d, want %d", tc.amount, tc.potency, got, tc.want)
		}
	}
}
