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
	// The sheet is now a framed, color-tagged bento panel; assert on the
	// tag-wrapped values + section headers rather than the old plain lines.
	for _, want := range []string{
		"Maerys",                               // name
		"<highlight>Human Fighter</highlight>", // identity
		"<title>Character</title>",
		"<title>Combat</title>",
		"<title>Attributes</title>",
		"<title>Purse & Training</title>",
		"<highlight>16</highlight>", // STR value
		"<highlight>18</highlight>", // AC value
		"<highlight>+5</highlight>", // hit bonus
		"neutral (0)",               // alignment text
		"<gold>1,000</gold>",        // purse, thousands-separated
		"Full (84/100)",             // sustenance text
		"XP 12,500  (2,500 to next level)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("score sheet missing %q\n--- got ---\n%s", want, out)
		}
	}
}

func TestRenderScore_TierColors(t *testing.T) {
	// Low HP and low sustenance switch from the healthy tag to danger.
	d := scoreData{
		Name: "Hurt", HasVitals: true, HP: 5, MaxHP: 100,
		HasStats: true, STR: 10, INT: 10, WIS: 10, DEX: 10, CON: 10, LUCK: 10,
		HasSust: true, Sust: 4, SustTier: "Starving",
	}
	out := renderScore(d)
	if !strings.Contains(out, "<danger>5 / 100</danger>") {
		t.Errorf("low HP not danger-tagged\n--- got ---\n%s", out)
	}
	if !strings.Contains(out, "<danger>Starving (4/100)</danger>") {
		t.Errorf("low sustenance not danger-tagged\n--- got ---\n%s", out)
	}
}

func TestRenderScore_Equipment(t *testing.T) {
	d := scoreData{
		Name: "Geared", HasStats: true,
		HasEquip: true,
		Equip: []equipRow{
			{Label: "wielded", Name: "<item.uncommon>a longsword</item.uncommon>"},
			{Label: "head", Name: "<subtle>(empty)</subtle>"},
		},
	}
	out := renderScore(d)
	for _, want := range []string{
		"<title>Equipment</title>",
		"<item.uncommon>a longsword</item.uncommon>",
		"<subtle>(empty)</subtle>",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("equipment section missing %q\n--- got ---\n%s", want, out)
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
