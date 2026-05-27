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

// TestValidateName_RejectsControlAndProtocolBytes pins the defensive
// behavior that the spec letters-only whitelist already provides:
// every control byte (< 0x20), the ANSI ESC byte (0x1B), the telnet
// IAC byte (0xFF), and other non-letter bytes are rejected at the
// boundary. Closes the m7-followup-1 deferred fix — PlayerName
// injection into broadcasts is impossible at the source.
func TestValidateName_RejectsControlAndProtocolBytes(t *testing.T) {
	cfg := Config{}
	cases := []struct {
		name string
		in   string
	}{
		{"NUL byte", "Al\x00ice"},
		{"ESC (CSI prefix)", "Al\x1bice"},
		{"IAC byte", "Al\xffice"},
		{"newline", "Alice\n"},
		{"tab", "Al\tice"},
		{"backspace", "Al\bice"},
		{"DEL", "Al\x7fice"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if msg := validateName(c.in, cfg); msg == "" {
				t.Errorf("validateName(%q) = ok, want rejection", c.in)
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
