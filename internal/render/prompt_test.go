package render

import (
	"strings"
	"testing"
)

func TestRenderPromptTokens(t *testing.T) {
	v := PromptVitals{HP: 12, MaxHP: 20, Mana: 5, MaxMana: 8, MV: 30, MaxMV: 40, Gold: 99}
	tests := []struct {
		name string
		tmpl string
		want string
	}{
		{"all tokens", "{hp}/{maxhp} {mana}/{maxmana} {mv}/{maxmv} {gold}", "12/20 5/8 30/40 99"},
		{"case insensitive", "{HP}/{MaxHP}", "12/20"},
		{"unknown token empties", "a{maxhpp}b", "ab"},
		{"keeps surrounding text", "HP: {hp} done", "HP: 12 done"},
		// A letters-shaped brace is a token: unknown → empty (§7.2).
		// Prompt templates use <...> color tags, not {...} braces (§7.1).
		{"unknown letters token empties", "{r}red", "red"},
		{"numeric brace left verbatim", "{1}", "{1}"},
		{"unterminated brace left verbatim", "{hp", "{hp"},
		{"no braces", "plain prompt>", "plain prompt>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RenderPrompt(tt.tmpl, v); got != tt.want {
				t.Errorf("RenderPrompt(%q) = %q, want %q", tt.tmpl, got, tt.want)
			}
		})
	}
}

func TestRenderPromptDefaultTemplate(t *testing.T) {
	v := PromptVitals{HP: 7, MaxHP: 10, Mana: 1, MaxMana: 2, MV: 3, MaxMV: 4}
	got := RenderPrompt("", v)
	// Default template substitutes the vital tokens and keeps its
	// semantic <hp>/<mana>/<mv> tags for the color renderer.
	for _, want := range []string{"7/10", "1/2", "3/4", "<hp>", "</mv>"} {
		if !strings.Contains(got, want) {
			t.Errorf("default prompt %q missing %q", got, want)
		}
	}
	if strings.ContainsRune(got, '{') {
		t.Errorf("default prompt still has an unsubstituted brace: %q", got)
	}
}

// The prompt's semantic tags must survive RenderPrompt so the color
// renderer can resolve them afterwards (the two stages compose).
func TestRenderPromptThenColor(t *testing.T) {
	theme := NewThemeRegistry()
	theme.Register("hp", ThemeEntry{FG: "green"})
	theme.Compile()
	cr := NewColorRenderer(theme)

	prompt := RenderPrompt("<hp>{hp}/{maxhp}</hp>", PromptVitals{HP: 9, MaxHP: 9})
	got := cr.RenderAnsi(prompt)
	want := "\x1b[32m9/9" + Reset
	if got != want {
		t.Errorf("composed = %q, want %q", got, want)
	}
}

func TestRenderPromptEdgeBraces(t *testing.T) {
	v := PromptVitals{HP: 1, MaxHP: 2}
	cases := map[string]string{
		"{123}":       "{123}", // non-letters: verbatim
		"{foo":        "{foo",  // unterminated: verbatim
		"no braces":   "no braces",
		"{hp}{maxhp}": "12",
	}
	for in, want := range cases {
		if got := RenderPrompt(in, v); got != want {
			t.Errorf("RenderPrompt(%q) = %q, want %q", in, got, want)
		}
	}
}
