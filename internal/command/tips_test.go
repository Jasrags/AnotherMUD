package command_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/light"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

func tipsDispatch(t *testing.T, a *testActor, args string) {
	t.Helper()
	c := &command.Context{Actor: a, Args: strings.Fields(args)}
	if err := command.TipsHandler(context.Background(), c); err != nil {
		t.Fatalf("TipsHandler: %v", err)
	}
}

func TestTipsHandler_StatusToggleReset(t *testing.T) {
	a := newNamedTestActor("Tester", "p1", nil)

	tipsDispatch(t, a, "") // bare: status, must NOT flip
	if !strings.Contains(a.lastLine(), "on") {
		t.Errorf("bare tips status = %q, want on", a.lastLine())
	}
	if !a.TipsEnabled() {
		t.Error("bare tips flipped the preference")
	}

	tipsDispatch(t, a, "off")
	if a.TipsEnabled() {
		t.Error("tips off did not disable")
	}
	tipsDispatch(t, a, "on")
	if !a.TipsEnabled() {
		t.Error("tips on did not enable")
	}

	// After seeing a tip, reset re-arms it.
	a.tipsActive = true
	a.ShowTipOnce(context.Background(), "x", "hint")
	if _, seen := a.tipsSeen["x"]; !seen {
		t.Fatal("tip not recorded")
	}
	tipsDispatch(t, a, "reset")
	if len(a.tipsSeen) != 0 {
		t.Error("reset did not clear the seen set")
	}

	tipsDispatch(t, a, "sideways")
	if !strings.Contains(a.lastLine(), "Usage: tips") {
		t.Errorf("bad arg = %q, want usage", a.lastLine())
	}
}

func TestMaybeShowRoomTips_HelpFirstOnceThenSilent(t *testing.T) {
	room := &world.Room{ID: "r1", Name: "Nowhere"}
	a := newNamedTestActor("Tester", "p1", room)
	a.tipsActive = true
	c := &command.Context{Actor: a}

	// First room view (lit): the general help tip fires (unconditional, first).
	c.ShowRoomTipsForTest(context.Background(), room, light.Dim)
	if !strings.Contains(a.lastLine(), "help getting-started") {
		t.Fatalf("first room view tip = %q, want the help tip", a.lastLine())
	}
	// Second view: help tip already seen; a lit, empty room has no other
	// candidate, so nothing new is shown.
	before := len(a.lines)
	c.ShowRoomTipsForTest(context.Background(), room, light.Dim)
	if len(a.lines) != before {
		t.Errorf("a second room view re-emitted a tip: %q", a.lastLine())
	}
}

func TestMaybeShowRoomTips_DarkTip(t *testing.T) {
	room := &world.Room{ID: "r1", Name: "Nowhere"}
	a := newNamedTestActor("Tester", "p1", room)
	a.tipsActive = true
	// Pre-mark the help tip so the situational dark tip is the live candidate.
	a.ShowTipOnce(context.Background(), "help", "seen")
	c := &command.Context{Actor: a}

	c.ShowRoomTipsForTest(context.Background(), room, light.Black)
	if !strings.Contains(a.lastLine(), "too dark") {
		t.Errorf("dark room tip = %q, want the darkness tip", a.lastLine())
	}
}

func TestMaybeShowRoomTips_OptOutSilent(t *testing.T) {
	room := &world.Room{ID: "r1", Name: "Nowhere"}
	a := newNamedTestActor("Tester", "p1", room)
	a.SetTipsEnabled(false)
	c := &command.Context{Actor: a}

	c.ShowRoomTipsForTest(context.Background(), room, 0)
	if got := a.lines; len(got) != 0 {
		t.Errorf("disabled tips still emitted %v", got)
	}
}
