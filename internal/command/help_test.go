package command_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/help"
)

func helpSvc(t *testing.T) *help.Service {
	t.Helper()
	s := help.NewService()
	s.AddTopic(&help.Topic{PackName: "core", ID: "look", Title: "Look", Category: "commands", Brief: "Examine.", Keywords: []string{"examine"}}, 0)
	// cast + spells share the non-category keyword "arcana" (for true
	// disambiguation) and both live in the "magic" category (for category
	// drill-down).
	s.AddTopic(&help.Topic{PackName: "core", ID: "cast", Title: "Cast", Category: "magic", Brief: "Cast a spell.", Keywords: []string{"magic", "arcana"}}, 0)
	s.AddTopic(&help.Topic{PackName: "core", ID: "spells", Title: "Spells", Category: "magic", Brief: "Spell list.", Keywords: []string{"magic", "arcana"}}, 0)
	return s
}

func dispatchHelp(t *testing.T, svc *help.Service, args string) *testActor {
	t.Helper()
	a := newNamedTestActor("Tester", "p1", nil)
	c := &command.Context{Actor: a, Help: svc, Args: strings.Fields(args)}
	if err := command.HelpHandler(context.Background(), c); err != nil {
		t.Fatalf("HelpHandler: %v", err)
	}
	return a
}

func TestHelpHandlerTopic(t *testing.T) {
	a := dispatchHelp(t, helpSvc(t), "look")
	if !strings.Contains(a.lastLine(), "Look") || !strings.Contains(a.lastLine(), "Examine.") {
		t.Errorf("help look output = %q", a.lastLine())
	}
}

func TestHelpHandlerIndex(t *testing.T) {
	a := dispatchHelp(t, helpSvc(t), "")
	out := a.lastLine()
	if !strings.Contains(out, "Categories:") || !strings.Contains(out, "commands") || !strings.Contains(out, "magic") {
		t.Errorf("help index = %q", out)
	}
}

func TestHelpHandlerDisambiguation(t *testing.T) {
	// "arcana" is a shared keyword but not a category, so it falls through
	// to a fuzzy query that matches two topics.
	a := dispatchHelp(t, helpSvc(t), "arcana")
	if !strings.Contains(a.lastLine(), "Multiple matches") {
		t.Errorf("help arcana = %q", a.lastLine())
	}
}

func TestHelpHandlerCategoryListing(t *testing.T) {
	// A term naming a category drills into its topic list rather than
	// fuzzy-matching the keyword of the same name.
	a := dispatchHelp(t, helpSvc(t), "magic")
	out := a.lastLine()
	if strings.Contains(out, "Multiple matches") {
		t.Fatalf("category term fuzzy-matched instead of listing: %q", out)
	}
	if !strings.Contains(out, "cast") || !strings.Contains(out, "spells") {
		t.Errorf("category listing missing topics: %q", out)
	}
	if !strings.Contains(out, "Cast a spell.") {
		t.Errorf("category listing missing briefs: %q", out)
	}
}

func TestHelpHandlerNoMatch(t *testing.T) {
	a := dispatchHelp(t, helpSvc(t), "frobnitz")
	if !strings.Contains(a.lastLine(), "No help found for 'frobnitz'") {
		t.Errorf("help miss = %q", a.lastLine())
	}
}

func TestHelpHandlerNilService(t *testing.T) {
	a := newNamedTestActor("Tester", "p1", nil)
	c := &command.Context{Actor: a, Args: []string{"look"}}
	if err := command.HelpHandler(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(a.lastLine(), "not available") {
		t.Errorf("nil-service help = %q", a.lastLine())
	}
}
