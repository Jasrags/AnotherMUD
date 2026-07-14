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
	// The bare-help index groups commands under category headers (titles), with
	// each group a grid of verb keywords. The test service uses categories
	// outside categoryOrder ("commands"/"magic"), which render as leftover
	// groups titled from the key.
	a := dispatchHelp(t, helpSvc(t), "")
	out := a.lastLine()
	for _, want := range []string{"Command Categories", "Commands", "Magic", "look", "cast", "spells"} {
		if !strings.Contains(out, want) {
			t.Errorf("help index missing %q:\n%q", want, out)
		}
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

// TestGenerateHelpTopics_SynthesizesSyntax confirms §8: a typed command's
// help syntax is synthesized from its arg defs, while an untyped command
// keeps its hand-authored Syntax.
func TestGenerateHelpTopics_SynthesizesSyntax(t *testing.T) {
	r := command.New()
	noop := func(ctx context.Context, c *command.Context) error { return nil }

	if err := r.RegisterCommand(command.Command{
		Keyword: "zap", Brief: "Zap a target.", Handler: noop,
		Args: []command.ArgDefinition{{Name: "target", Type: command.ArgEntity}},
	}); err != nil {
		t.Fatalf("register zap: %v", err)
	}
	if err := r.RegisterCommand(command.Command{
		Keyword: "wiggle", Brief: "Wiggle.", Syntax: []string{"wiggle around"}, Handler: noop,
	}); err != nil {
		t.Fatalf("register wiggle: %v", err)
	}

	svc := help.NewService()
	command.GenerateHelpTopics(r, svc)

	if got := dispatchHelp(t, svc, "zap").lastLine(); !strings.Contains(got, "zap [target]") {
		t.Errorf("typed command help = %q, want synthesized 'zap [target]'", got)
	}
	if got := dispatchHelp(t, svc, "wiggle").lastLine(); !strings.Contains(got, "wiggle around") {
		t.Errorf("untyped command help = %q, want hand-authored syntax", got)
	}
}
