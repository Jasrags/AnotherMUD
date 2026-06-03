package command

import (
	"strings"
	"testing"
)

func TestRenderScore_FullSheet(t *testing.T) {
	d := scoreData{
		Name: "Maerys", Race: "Human", Class: "Fighter",
		HasVitals: true, HP: 110, MaxHP: 110,
		HasResources: true, Mana: 0, MV: 0,
		HasStats: true, STR: 16, INT: 10, WIS: 12, DEX: 14, CON: 15, LUCK: 8,
		AC: 18, Hit: 5,
		HasAlign: true, AlignTag: "neutral", Align: 0,
		HasGold: true, Gold: 1000,
		HasSust: true, Sust: 84, SustTier: "Full",
		HasLevel: true, Track: "adventure", Level: 10, XP: 12500, XpToNext: 2500,
	}
	out := renderScore(d)
	for _, want := range []string{
		"Maerys",
		"Human Fighter — level 10 (adventure)",
		"HP 110/110   MA 0/0   MV 0/0",
		"STR 16  INT 10  WIS 12  DEX 14  CON 15  LUCK 8",
		"AC 18   Hit +5",
		"Alignment neutral (0)    Gold 1000",
		"Sustenance: Full (84/100)",
		"XP 12500   (2500 to next level)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("score sheet missing %q\n--- got ---\n%s", want, out)
		}
	}
}

func TestRenderScore_AtMaxLevel(t *testing.T) {
	d := scoreData{Name: "X", HasLevel: true, Track: "adventure", Level: 10, XP: 99999, AtMax: true}
	if out := renderScore(d); !strings.Contains(out, "(max level)") {
		t.Errorf("at-max sheet = %q, want '(max level)'", out)
	}
}

func TestRenderScore_MinimalActor(t *testing.T) {
	// Only the name set (a non-combatant / non-subject actor) → just the
	// name, no empty sections.
	if out := renderScore(scoreData{Name: "Ghost"}); strings.TrimSpace(out) != "Ghost" {
		t.Errorf("minimal sheet = %q, want just the name", out)
	}
}

func TestTitleCase(t *testing.T) {
	cases := map[string]string{"human": "Human", "fighter": "Fighter", "full": "Full", "": "", " neutral ": "Neutral"}
	for in, want := range cases {
		if got := titleCase(in); got != want {
			t.Errorf("titleCase(%q) = %q, want %q", in, got, want)
		}
	}
}
