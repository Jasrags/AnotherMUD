package main

import (
	"reflect"
	"testing"
)

func TestParseRoleSeed(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want map[string][]string
	}{
		{"empty", "", nil},
		{"whitespace", "   ", nil},
		{"single", "maerys:admin", map[string][]string{"maerys": {"admin"}}},
		{"multi-role", "jrags:admin,builder", map[string][]string{"jrags": {"admin", "builder"}}},
		{"multi-entry", "maerys:admin;jrags:admin,builder",
			map[string][]string{"maerys": {"admin"}, "jrags": {"admin", "builder"}}},
		{"normalizes case+space", " Maerys : Admin , Builder ",
			map[string][]string{"maerys": {"admin", "builder"}}},
		{"skips malformed", "noColonHere;:noName;good:admin",
			map[string][]string{"good": {"admin"}}},
		{"all malformed -> nil", "noColon;:::", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseRoleSeed(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("parseRoleSeed(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}
