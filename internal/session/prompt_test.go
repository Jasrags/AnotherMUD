package session

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/player"
)

// SetPromptTemplate writes the save field (so it persists across logout),
// marks the save dirty, and flags a prompt refresh; PromptTemplate reads
// it back. Clearing reverts to "" (→ default). Setting an unchanged value
// is a no-op. (ui-rendering-help §7.4.)
func TestSetPromptTemplate_PersistsAndFlagsRefresh(t *testing.T) {
	a := &connActor{save: &player.Save{Version: player.CurrentVersion, Name: "Tester"}}

	const tpl = "<hp>[HP {hp}/{maxhp}]</hp>> "
	a.SetPromptTemplate(tpl)

	if a.save.PromptTemplate != tpl {
		t.Errorf("save.PromptTemplate = %q, want %q", a.save.PromptTemplate, tpl)
	}
	if got := a.PromptTemplate(); got != tpl {
		t.Errorf("PromptTemplate() = %q, want %q", got, tpl)
	}
	if !a.dirty {
		t.Error("expected save marked dirty after set")
	}
	if !a.needsPromptRefresh {
		t.Error("expected needsPromptRefresh flagged after set")
	}

	// Clearing reverts to the default and re-marks dirty.
	a.dirty = false
	a.SetPromptTemplate("")
	if a.save.PromptTemplate != "" {
		t.Errorf("after clear, PromptTemplate = %q, want empty", a.save.PromptTemplate)
	}
	if !a.dirty {
		t.Error("expected dirty after clear")
	}

	// Re-setting the same (now empty) value is a no-op — no dirty churn.
	a.dirty = false
	a.SetPromptTemplate("")
	if a.dirty {
		t.Error("setting an unchanged value should not mark dirty")
	}
}

// PromptTemplate is nil-save safe (minimal test actors have no save).
func TestPromptTemplate_NilSaveSafe(t *testing.T) {
	a := &connActor{}
	if got := a.PromptTemplate(); got != "" {
		t.Errorf("PromptTemplate() on nil save = %q, want empty", got)
	}
	a.SetPromptTemplate("x") // must not panic
}
