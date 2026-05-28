package session

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/player"
)

func newPromptActor(tmpl string) (*connActor, *fakeConn) {
	fc := &fakeConn{id: "p1"}
	return &connActor{
		id:           "p1",
		conn:         fc,
		renderer:     buildTestRenderer(),
		colorEnabled: true,
		vitals:       combat.NewVitals(20),
		save:         &player.Save{PromptTemplate: tmpl},
	}, fc
}

// flushPrompt is a no-op until content has been sent (needsPromptRefresh
// is the gate); once content goes out, the flush renders the template on
// its own line and arms promptDisplayed.
func TestFlushPromptOnlyAfterContent(t *testing.T) {
	a, fc := newPromptActor("HP:{hp}/{maxhp}")
	ctx := context.Background()

	if err := a.flushPrompt(ctx); err != nil {
		t.Fatal(err)
	}
	if len(fc.writes()) != 0 {
		t.Fatalf("flush before content wrote %d times, want 0", len(fc.writes()))
	}

	if err := a.Write(ctx, "hello"); err != nil {
		t.Fatal(err)
	}
	if err := a.flushPrompt(ctx); err != nil {
		t.Fatal(err)
	}
	writes := fc.writes()
	last := writes[len(writes)-1]
	if last != "\r\nHP:20/20" {
		t.Errorf("prompt = %q, want %q", last, "\r\nHP:20/20")
	}

	// A second flush with nothing new is a no-op (refresh already cleared).
	before := len(fc.writes())
	if err := a.flushPrompt(ctx); err != nil {
		t.Fatal(err)
	}
	if len(fc.writes()) != before {
		t.Errorf("second flush wrote again; want no-op")
	}
}

// When a prompt is displayed and no input has arrived, the next content
// send breaks the line first so it doesn't run into the prompt.
func TestWriteBreaksDisplayedPrompt(t *testing.T) {
	a, fc := newPromptActor("p>")
	ctx := context.Background()

	a.mu.Lock()
	a.promptDisplayed = true
	a.receivedInput = false
	a.mu.Unlock()

	if err := a.Write(ctx, "async"); err != nil {
		t.Fatal(err)
	}
	got := fc.writes()[0]
	if got != "\r\nasync\r\n" {
		t.Errorf("broken write = %q, want %q", got, "\r\nasync\r\n")
	}
}

// After the player types (receivedInput set), the next content send does
// NOT prepend a CR-LF — the keystroke already moved off the prompt line.
func TestWriteNoBreakAfterInput(t *testing.T) {
	a, fc := newPromptActor("p>")
	ctx := context.Background()

	a.mu.Lock()
	a.promptDisplayed = true
	a.mu.Unlock()
	a.noteInput(time.Now()) // sets receivedInput

	if err := a.Write(ctx, "result"); err != nil {
		t.Fatal(err)
	}
	got := fc.writes()[0]
	if got != "result\r\n" {
		t.Errorf("write = %q, want %q (no leading CRLF)", got, "result\r\n")
	}
	if strings.HasPrefix(got, "\r\n") {
		t.Error("must not break line after input received")
	}
}
