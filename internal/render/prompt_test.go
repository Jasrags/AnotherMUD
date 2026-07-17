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

// The default template is pool-adaptive: the mana segment appears only
// when the character has a mana pool (MaxMana > 0). A mana-less archetype
// (a Shadowrun street samurai) gets no dead [MA 0/0].
func TestDefaultPromptTemplateAdaptive(t *testing.T) {
	caster := DefaultPromptTemplate(PromptVitals{MaxMana: 8})
	if !strings.Contains(caster, "[MA {mana}/{maxmana}]") {
		t.Errorf("caster default %q should include the mana segment", caster)
	}

	sam := DefaultPromptTemplate(PromptVitals{MaxMana: 0})
	if strings.Contains(sam, "MA") || strings.Contains(sam, "mana") {
		t.Errorf("mana-less default %q should omit the mana segment", sam)
	}
	// HP and MV always show, for everyone.
	for _, want := range []string{"[HP {hp}/{maxhp}]", "[MV {mv}/{maxmv}]"} {
		if !strings.Contains(sam, want) {
			t.Errorf("default %q missing always-on segment %q", sam, want)
		}
	}

	// Stun is the same adaptive rule: shown only when the character has a
	// stun pool (MaxStun > 0), and it sits right after HP.
	stunless := DefaultPromptTemplate(PromptVitals{MaxHP: 26})
	if strings.Contains(stunless, "ST") || strings.Contains(stunless, "stun") {
		t.Errorf("stun-less default %q should omit the stun segment", stunless)
	}
	runner := DefaultPromptTemplate(PromptVitals{MaxStun: 11})
	if !strings.Contains(runner, "[ST {stun}/{maxstun}]") {
		t.Errorf("stun-bearing default %q should include the stun segment", runner)
	}
	// Order: HP, then ST, then MV (no mana here).
	if hp, st := strings.Index(runner, "[HP"), strings.Index(runner, "[ST"); hp < 0 || st < hp {
		t.Errorf("stun segment should follow HP in %q", runner)
	}
	if got := RenderPrompt("", PromptVitals{HP: 7, MaxHP: 26, Stun: 10, MaxStun: 11, MV: 28, MaxMV: 30}); !strings.Contains(got, "ST 10/11") {
		t.Errorf("rendered runner prompt %q should show ST 10/11", got)
	}

	// A rendered mana-less prompt has no unsubstituted braces and no MA.
	got := RenderPrompt("", PromptVitals{HP: 7, MaxHP: 26, MV: 28, MaxMV: 30})
	if strings.Contains(got, "MA") {
		t.Errorf("rendered mana-less prompt %q should not contain MA", got)
	}
	if strings.ContainsRune(got, '{') {
		t.Errorf("rendered prompt %q still has an unsubstituted brace", got)
	}
}

// Conditional segments {?name}…{/name} render their body only when the
// character has the named pool (max > 0), letting a custom template adapt
// the way the default does (ui-rendering-help §7.5).
func TestRenderPromptConditionalSegments(t *testing.T) {
	sam := PromptVitals{HP: 7, MaxHP: 26, MaxStun: 11, Stun: 10, MV: 28, MaxMV: 30} // no mana
	mage := PromptVitals{HP: 12, MaxHP: 20, MaxMana: 8, Mana: 5, MV: 30, MaxMV: 40} // no stun
	tests := []struct {
		name string
		tmpl string
		v    PromptVitals
		want string
	}{
		{"present pool renders body", "{?stun}ST {stun}/{maxstun}{/stun}", sam, "ST 10/11"},
		{"absent pool hides body", "{?mana}MA {mana}/{maxmana}{/mana}", sam, ""},
		{"absent stun hides for mage", "{?stun}ST {stun}{/stun}", mage, ""},
		{"present mana renders for mage", "{?mana}MA {mana}/{maxmana}{/mana}", mage, "MA 5/8"},
		{"surrounding text kept", "a{?mana}X{/mana}b", sam, "ab"},
		{"surrounding text kept when shown", "a{?stun}X{/stun}b", sam, "aXb"},
		// Different-name nesting composes via recursion.
		{"nested different names both present", "{?stun}S{?mana}M{/mana}{/stun}", mage, ""},
		{"nested outer present inner absent", "{?stun}S{?mana}M{/mana}E{/stun}", sam, "SE"},
		{"nested both present", "{?stun}S{?mv}M{/mv}{/stun}", sam, "SM"},
		// Unknown condition hides its body (no such pool).
		{"unknown condition hides", "x{?money}rich{/money}y", sam, "xy"},
		// Case-insensitive pairing: {?Stun} pairs with {/stun}, and the
		// segment stays gated (a mismatch must NOT render unconditionally).
		{"case-mismatched pair hides when absent", "a{?Stun}X{/stun}b", mage, "ab"},
		{"case-mismatched pair shows when present", "a{?Stun}X{/STUN}b", sam, "aXb"},
		// Lenient malformed handling.
		{"missing close drops marker", "a{?stun}b", sam, "ab"},
		{"stray close dropped", "a{/stun}b", sam, "ab"},
		{"malformed open left literal", "{?}", sam, "{?}"},
		{"non-letter close left literal", "a{/1}b", sam, "a{/1}b"},
		// gold condition keys on Gold > 0.
		{"gold present", "{?gold}{gold}nY{/gold}", PromptVitals{Gold: 725}, "725nY"},
		{"gold absent", "{?gold}{gold}nY{/gold}", PromptVitals{Gold: 0}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RenderPrompt(tt.tmpl, tt.v); got != tt.want {
				t.Errorf("RenderPrompt(%q) = %q, want %q", tt.tmpl, got, tt.want)
			}
		})
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
