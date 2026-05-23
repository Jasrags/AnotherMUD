package login

import "testing"

func TestValidateName(t *testing.T) {
	cfg := Config{}
	tests := []struct {
		in     string
		wantOK bool
	}{
		{"Alice", true},
		{"al", true},
		{"a", false},
		{"alice1", false},
		{"alice-the-second", false},
		{"alíce", false},
		{"thisnameiswaytoolongforus", false},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			got := validateName(tc.in, cfg)
			if (got == "") != tc.wantOK {
				t.Errorf("validateName(%q) = %q, wantOK=%v", tc.in, got, tc.wantOK)
			}
		})
	}
}

func TestValidateNewPassword(t *testing.T) {
	cfg := Config{}
	if msg := validateNewPassword("12345", cfg); msg == "" {
		t.Error("short password accepted")
	}
	if msg := validateNewPassword("longenough", cfg); msg != "" {
		t.Errorf("long password rejected: %q", msg)
	}
}
