package session

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/render"
)

// buildTestRenderer compiles a tiny theme so the renderer resolves a
// known semantic tag.
func buildTestRenderer() *render.ColorRenderer {
	tr := render.NewThemeRegistry()
	tr.Register("highlight", render.ThemeEntry{FG: "bright-yellow"})
	tr.Compile()
	return render.NewColorRenderer(tr)
}

func TestConnActorRenderUsesThemeWhenColorEnabled(t *testing.T) {
	fc := &fakeConn{id: "c1"}
	a := &connActor{
		id:           "c1",
		conn:         fc,
		renderer:     buildTestRenderer(),
		colorEnabled: true,
		colorTier:    render.ColorTierBasic, // M16.6b: explicit ANSI-16
	}
	if err := a.Write(context.Background(), "<highlight>hi</highlight>"); err != nil {
		t.Fatal(err)
	}
	got := fc.writes()[0]
	want := "\x1b[93mhi" + render.Reset + "\r\n"
	if got != want {
		t.Errorf("colored write = %q, want %q", got, want)
	}
}

func TestConnActorRenderPlainWhenColorDisabled(t *testing.T) {
	fc := &fakeConn{id: "c1"}
	a := &connActor{
		id:           "c1",
		conn:         fc,
		renderer:     buildTestRenderer(),
		colorEnabled: false,
	}
	if err := a.Write(context.Background(), "<highlight>hi</highlight>"); err != nil {
		t.Fatal(err)
	}
	got := fc.writes()[0]
	if got != "hi\r\n" {
		t.Errorf("plain write = %q, want %q", got, "hi\\r\\n")
	}
	if strings.Contains(got, "\x1b") {
		t.Error("plain write must contain no ANSI escapes")
	}
}

// With no renderer wired, Write falls back to the minimal M2 ansi
// renderer (single-letter brace codes), preserving back-compat for
// tests and any caller that doesn't supply cfg.Render.
func TestConnActorRenderFallsBackToAnsi(t *testing.T) {
	fc := &fakeConn{id: "c1"}
	a := &connActor{id: "c1", conn: fc, colorEnabled: true}
	if err := a.Write(context.Background(), "{r}hi{x}"); err != nil {
		t.Fatal(err)
	}
	got := fc.writes()[0]
	if !strings.Contains(got, "\x1b[31m") || !strings.HasSuffix(got, "\r\n") {
		t.Errorf("fallback write = %q, want ROM red + CRLF", got)
	}
}
