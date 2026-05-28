package render

import "testing"

func TestRenderAnsi(t *testing.T) {
	r := NewColorRenderer(newTestTheme())
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"semantic open+close", "<highlight>hi</highlight>", "\x1b[93mhi" + Reset},
		{"semantic bg", "<danger>x</danger>", "\x1b[31m\x1b[40mx" + Reset},
		{"declared color-less emits plain", "<note>quiet</note>", "quiet"},
		{"literal color fg", `<color fg="red">danger</color>`, "\x1b[31mdanger" + Reset},
		{"literal color fg+bg", `<color fg="yellow" bg="black">w</color>`, "\x1b[33m\x1b[40mw" + Reset},
		{"brace full name", "{yellow}hi{/}", "\x1b[33mhi" + Reset},
		{"brace ROM back-compat", "{r}hi{x}", "\x1b[31mhi" + Reset},
		{"brace no auto close adds trailing reset", "{r}hi", "\x1b[31mhi" + Reset},
		{"unknown opening tag passes through", "<bogus>x", "<bogus>x"},
		{"unknown closing tag passes through", "x</bogus>", "x</bogus>"},
		{"unknown brace passes through", "{frobnitz}", "{frobnitz}"},
		{"literal brace", "{{x}", "{x}"},
		{"unmatched angle", "a < b", "a < b"},
		{"raw ESC dropped", "a\x1bb", "ab"},
		{"case insensitive tag", "<HIGHLIGHT>h</HIGHLIGHT>", "\x1b[93mh" + Reset},
		{"plain text", "hello world", "hello world"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := r.RenderAnsi(tt.in); got != tt.want {
				t.Errorf("RenderAnsi(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestRenderPlain(t *testing.T) {
	r := NewColorRenderer(newTestTheme())
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"semantic", "<highlight>hi</highlight>", "hi"},
		{"literal color", `<color fg="red">danger</color>`, "danger"},
		{"brace", "{yellow}hi{/}", "hi"},
		{"rom brace", "{r}hi{x}", "hi"},
		{"unknown opening passes through", "<bogus>x", "<bogus>x"},
		{"unknown brace passes through", "{frobnitz}", "{frobnitz}"},
		{"literal brace", "{{x}", "{x}"},
		{"raw ESC dropped", "a\x1bb", "ab"},
		{"plain", "hello", "hello"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := r.RenderPlain(tt.in); got != tt.want {
				t.Errorf("RenderPlain(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// Plain and ANSI must recognize the same constructs: stripping the SGR
// codes from RenderAnsi yields RenderPlain's visible text. We verify the
// shared-structure property by checking that visible characters match.
func TestRenderPlainMatchesAnsiVisible(t *testing.T) {
	r := NewColorRenderer(newTestTheme())
	inputs := []string{
		"<highlight>a</highlight>b<danger>c</danger>",
		`mix <color fg="red">x</color> {yellow}y{/} z`,
		"<note>colorless</note> tail",
	}
	for _, in := range inputs {
		ansi := r.RenderAnsi(in)
		plain := r.RenderPlain(in)
		stripped := stripSGR(ansi)
		if stripped != plain {
			t.Errorf("for %q: ansi-stripped %q != plain %q", in, stripped, plain)
		}
	}
}

func TestRenderCache(t *testing.T) {
	r := NewColorRenderer(newTestTheme())
	in := "<highlight>cached</highlight>"
	a := r.RenderAnsi(in)
	b := r.RenderAnsi(in)
	if a != b {
		t.Fatal("cached ansi differs")
	}
	if _, ok := r.ansiCache.Load(in); !ok {
		t.Error("expected cache entry after RenderAnsi")
	}
}

func TestAttrValue(t *testing.T) {
	tests := []struct {
		inner string
		key   string
		want  string
	}{
		{`color fg="red"`, "fg", "red"},
		{`color fg="yellow" bg="black"`, "bg", "black"},
		{`color fg='cyan'`, "fg", "cyan"},      // single quotes
		{`color nofg="evil"`, "fg", ""},        // H1: no substring match
		{`color bg="x" nofg="evil"`, "fg", ""}, // anchored, not found
		{`color fg="a" nofg="b"`, "fg", "a"},   // real fg still found
		{`color`, "fg", ""},                    // absent
	}
	for _, tt := range tests {
		if got := attrValue(tt.inner, tt.key); got != tt.want {
			t.Errorf("attrValue(%q,%q) = %q, want %q", tt.inner, tt.key, got, tt.want)
		}
	}
}

func TestRenderConcurrent(t *testing.T) {
	r := NewColorRenderer(newTestTheme())
	const n = 50
	done := make(chan struct{})
	for i := 0; i < n; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			r.RenderAnsi("<highlight>x</highlight>")
			r.RenderPlain(`<color fg="red">y</color>`)
		}()
	}
	for i := 0; i < n; i++ {
		<-done
	}
}

// stripSGR removes ESC[...m sequences (test helper).
func stripSGR(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1B && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && s[j] != 'm' {
				j++
			}
			i = j
			continue
		}
		out = append(out, s[i])
	}
	return string(out)
}

func TestRenderQuotedAngleNotTruncated(t *testing.T) {
	r := NewColorRenderer(newTestTheme())
	// A '>' inside a quoted attribute value must not truncate the tag.
	// fg=">" is not a real color, so the open produces no SGR; the
	// <color> close emits a (harmless) reset. The key property: "x"
	// renders with no garbled trailing text (the m10-1 #2 fix).
	if got := r.RenderAnsi(`<color fg=">">x</color>`); got != "x"+Reset {
		t.Errorf("quoted '>' truncated the tag: %q", got)
	}
	// bg after a quoted '>' is still parsed (boundary not lost early).
	if got := r.RenderAnsi(`<color fg="red" bg=">">y</color>`); got != "\x1b[31my"+Reset {
		t.Errorf("attr after quoted '>' lost: %q", got)
	}
}
