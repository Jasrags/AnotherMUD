package slot

import (
	"errors"
	"testing"
)

func TestIsValidName(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"wield", true},
		{"left_hand", true},
		{"slot_2", true},
		{"a", true},
		{"", false},
		{"Wield", false},       // uppercase
		{"left-hand", false},   // hyphen
		{"left hand", false},   // space
		{"_wield", false},      // leading underscore
		{"wield_", false},      // trailing underscore
		{"left__hand", false},  // consecutive underscores
		{"2hand", false},       // leading digit
		{"weird/slash", false}, // punctuation
		{"über", false},        // non-ascii
	}
	for _, tc := range cases {
		if got := IsValidName(tc.in); got != tc.want {
			t.Errorf("IsValidName(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestBuildKeyCap1(t *testing.T) {
	got, err := BuildKey("wield", 0, 1)
	if err != nil {
		t.Fatalf("BuildKey: %v", err)
	}
	if got != "wield" {
		t.Errorf("BuildKey(cap=1) = %q, want %q", got, "wield")
	}
}

func TestBuildKeyCap1RejectsNonZeroIndex(t *testing.T) {
	if _, err := BuildKey("wield", 1, 1); !errors.Is(err, ErrIndexOutOfMax) {
		t.Errorf("err = %v, want ErrIndexOutOfMax", err)
	}
}

func TestBuildKeyMultiCap(t *testing.T) {
	cases := []struct {
		index int
		want  string
	}{
		{0, "finger:0"},
		{1, "finger:1"},
	}
	for _, tc := range cases {
		got, err := BuildKey("finger", tc.index, 2)
		if err != nil {
			t.Fatalf("BuildKey: %v", err)
		}
		if got != tc.want {
			t.Errorf("BuildKey(%d, max=2) = %q, want %q", tc.index, got, tc.want)
		}
	}
}

func TestBuildKeyOutOfRange(t *testing.T) {
	if _, err := BuildKey("finger", 2, 2); !errors.Is(err, ErrIndexOutOfMax) {
		t.Errorf("err = %v, want ErrIndexOutOfMax", err)
	}
	if _, err := BuildKey("finger", -1, 2); !errors.Is(err, ErrIndexOutOfMax) {
		t.Errorf("err = %v, want ErrIndexOutOfMax", err)
	}
	if _, err := BuildKey("finger", 0, 0); !errors.Is(err, ErrIndexOutOfMax) {
		t.Errorf("err = %v, want ErrIndexOutOfMax", err)
	}
}

func TestParseKey(t *testing.T) {
	cases := []struct {
		in     string
		name   string
		index  int
		wantOK bool
	}{
		{"wield", "wield", 0, true},
		{"finger:0", "finger", 0, true},
		{"finger:2", "finger", 2, true},
		{"", "", 0, false},
		{":finger", "", 0, false},
		{"finger:", "", 0, false},
		{"finger:abc", "", 0, false},
		{"finger:-1", "", 0, false},
	}
	for _, tc := range cases {
		name, idx, err := ParseKey(tc.in)
		if tc.wantOK {
			if err != nil {
				t.Errorf("ParseKey(%q) err = %v, want nil", tc.in, err)
				continue
			}
			if name != tc.name || idx != tc.index {
				t.Errorf("ParseKey(%q) = (%q, %d), want (%q, %d)", tc.in, name, idx, tc.name, tc.index)
			}
		} else {
			if err == nil {
				t.Errorf("ParseKey(%q) returned nil err, want error", tc.in)
			}
		}
	}
}

func TestBuildAndParseRoundTrip(t *testing.T) {
	cases := []struct {
		name  string
		index int
		max   int
	}{
		{"wield", 0, 1},
		{"finger", 0, 2},
		{"finger", 1, 2},
	}
	for _, tc := range cases {
		key, err := BuildKey(tc.name, tc.index, tc.max)
		if err != nil {
			t.Fatalf("BuildKey: %v", err)
		}
		gotName, gotIdx, err := ParseKey(key)
		if err != nil {
			t.Fatalf("ParseKey: %v", err)
		}
		if gotName != tc.name || gotIdx != tc.index {
			t.Errorf("round trip for (%q,%d,max=%d): got (%q,%d)",
				tc.name, tc.index, tc.max, gotName, gotIdx)
		}
	}
}
