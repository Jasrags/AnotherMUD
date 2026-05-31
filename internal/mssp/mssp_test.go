package mssp_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/mssp"
)

// recordValue returns the byte-run between VAR<name>VAL and the
// next VAR or end-of-payload, or nil if the variable isn't found.
// Test helper that mirrors the spec §8.1 wire reader a crawler
// would write.
func recordValue(payload []byte, name string) []byte {
	const varByte = 1
	const valByte = 2
	needle := append([]byte{varByte}, name...)
	needle = append(needle, valByte)
	start := bytes.Index(payload, needle)
	if start < 0 {
		return nil
	}
	valStart := start + len(needle)
	// Find the next VAR or EOF.
	next := bytes.IndexByte(payload[valStart:], varByte)
	if next < 0 {
		return payload[valStart:]
	}
	return payload[valStart : valStart+next]
}

func TestEncode_EmitsRequiredVariables(t *testing.T) {
	cfg := mssp.Config{
		Name:     "AnotherMUD",
		Codebase: "AnotherMUD/dev",
		ANSI:     true,
		UTF8:     true,
		MCCP:     false,
		Players:  func() int { return 3 },
		Uptime:   func() int64 { return 42 },
	}
	out := mssp.Encode(cfg)

	cases := []struct {
		name string
		want string
	}{
		{"NAME", "AnotherMUD"},
		{"CODEBASE", "AnotherMUD/dev"},
		{"ANSI", "1"},
		{"UTF-8", "1"},
		{"MCCP", "0"},
		{"PLAYERS", "3"},
		{"UPTIME", "42"},
	}
	for _, tc := range cases {
		got := recordValue(out, tc.name)
		if got == nil {
			t.Errorf("%s missing from payload", tc.name)
			continue
		}
		if string(got) != tc.want {
			t.Errorf("%s = %q, want %q", tc.name, string(got), tc.want)
		}
	}
}

func TestEncode_SkipsEmptyStringFields(t *testing.T) {
	// Per the encoder doc: empty string-valued fields are omitted
	// so a crawler doesn't see noise like VAR PORT VAL "".
	cfg := mssp.Config{
		Name: "X",
		// All other string fields zero.
	}
	out := mssp.Encode(cfg)
	for _, name := range []string{"CODEBASE", "CONTACT", "HOSTNAME", "PORT",
		"CREATED", "LANGUAGE", "FAMILY", "GAMEPLAY"} {
		if recordValue(out, name) != nil {
			t.Errorf("%s should be omitted but was emitted", name)
		}
	}
}

func TestEncode_EmitsBoolsUnconditionally(t *testing.T) {
	// Bool fields emit "1" / "0" regardless of value — crawlers
	// distinguish "feature is off" from "feature unspecified."
	cfg := mssp.Config{Name: "X"}
	out := mssp.Encode(cfg)
	for _, tc := range []struct {
		name, want string
	}{
		{"CLASSES", "0"},
		{"RACES", "0"},
		{"LEVELS", "0"},
		{"EQUIPMENT", "0"},
		{"MULTIPLAYING", "0"},
		{"PLAYERKILLING", "0"},
		{"ANSI", "0"},
		{"UTF-8", "0"},
		{"GMCP", "0"},
		{"MCCP", "0"},
	} {
		got := recordValue(out, tc.name)
		if got == nil {
			t.Errorf("%s missing", tc.name)
			continue
		}
		if string(got) != tc.want {
			t.Errorf("%s = %q, want %q", tc.name, string(got), tc.want)
		}
	}
}

func TestEncode_PlayersAndUptimeReflectFactories(t *testing.T) {
	// Each Encode call must re-read PLAYERS and UPTIME so the
	// snapshot reflects current state.
	count := 0
	cfg := mssp.Config{
		Name:    "X",
		Players: func() int { count++; return count },
		Uptime:  func() int64 { return int64(count) * 10 },
	}
	first := mssp.Encode(cfg)
	second := mssp.Encode(cfg)

	if v := string(recordValue(first, "PLAYERS")); v != "1" {
		t.Errorf("first PLAYERS = %q, want 1", v)
	}
	if v := string(recordValue(second, "PLAYERS")); v != "2" {
		t.Errorf("second PLAYERS = %q, want 2", v)
	}
	if v := string(recordValue(second, "UPTIME")); v != "20" {
		t.Errorf("second UPTIME = %q, want 20", v)
	}
}

func TestEncode_NilFactoriesProduceZero(t *testing.T) {
	cfg := mssp.Config{Name: "X"} // no Players / Uptime
	out := mssp.Encode(cfg)
	if v := string(recordValue(out, "PLAYERS")); v != "0" {
		t.Errorf("nil Players → %q, want 0", v)
	}
	if v := string(recordValue(out, "UPTIME")); v != "0" {
		t.Errorf("nil Uptime → %q, want 0", v)
	}
}

func TestEncode_GameplayPipeJoined(t *testing.T) {
	cfg := mssp.Config{
		Name:     "X",
		Gameplay: []string{"Hack and Slash", "Roleplaying"},
	}
	out := mssp.Encode(cfg)
	got := string(recordValue(out, "GAMEPLAY"))
	want := "Hack and Slash|Roleplaying"
	if got != want {
		t.Errorf("GAMEPLAY = %q, want %q", got, want)
	}
}

func TestEncode_StableOrderingAcrossCalls(t *testing.T) {
	// MSSP-caching crawlers should see the same payload across
	// calls when nothing changed. Stable ordering means a byte-
	// equality check across two Encodes works.
	cfg := mssp.Config{
		Name:    "X",
		ANSI:    true,
		Players: func() int { return 5 },
		Uptime:  func() int64 { return 100 },
	}
	a := mssp.Encode(cfg)
	b := mssp.Encode(cfg)
	if !bytes.Equal(a, b) {
		t.Errorf("Encode not stable:\n a=%x\n b=%x", a, b)
	}
	// Sanity: ANSI must appear before MCCP (alpha sort).
	ai := strings.Index(string(a), "ANSI")
	mi := strings.Index(string(a), "MCCP")
	if ai < 0 || mi < 0 || ai >= mi {
		t.Errorf("alpha order broken: ANSI=%d MCCP=%d in %q", ai, mi, a)
	}
}
