package effect

import (
	"strings"
	"testing"
)

func TestDecode_BlessYAML(t *testing.T) {
	yaml := `id: bless
duration: 300
modifiers:
  - stat: hit_mod
    value: 2
flags: [blessed]
`
	tpl, err := Decode([]byte(yaml))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if tpl.ID != "bless" || tpl.Duration != 300 {
		t.Errorf("decoded %+v", tpl)
	}
	if len(tpl.Modifiers) != 1 || tpl.Modifiers[0].Stat != "hit_mod" || tpl.Modifiers[0].Value != 2 {
		t.Errorf("modifiers = %+v", tpl.Modifiers)
	}
	if len(tpl.Flags) != 1 || tpl.Flags[0] != "blessed" {
		t.Errorf("flags = %+v", tpl.Flags)
	}
}

func TestDecode_FlagOnlyEffect(t *testing.T) {
	tpl, err := Decode([]byte("id: cursed\nduration: -1\nflags: [cursed, unlucky]\n"))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if tpl.Duration != -1 {
		t.Errorf("Duration = %d, want -1 (permanent)", tpl.Duration)
	}
	if len(tpl.Modifiers) != 0 {
		t.Errorf("expected no modifiers, got %v", tpl.Modifiers)
	}
}

func TestDecode_RejectsEmptyID(t *testing.T) {
	if _, err := Decode([]byte("duration: 10\n")); err == nil ||
		!strings.Contains(err.Error(), "empty id") {
		t.Errorf("missing id: err = %v", err)
	}
}

func TestDecode_RejectsModifierWithoutStat(t *testing.T) {
	yaml := `id: bad
modifiers:
  - value: 5
`
	if _, err := Decode([]byte(yaml)); err == nil ||
		!strings.Contains(err.Error(), "empty stat") {
		t.Errorf("modifier missing stat: err = %v", err)
	}
}

func TestDecode_MalformedYAMLReturnsError(t *testing.T) {
	if _, err := Decode([]byte("id: [unclosed")); err == nil {
		t.Errorf("malformed yaml: want error")
	}
}
