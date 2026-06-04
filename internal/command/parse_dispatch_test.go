package command_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/command"
)

// TestParseInput_DispatchPipeline drives ParseInput → Dispatch the way the
// session pump does, proving a chained/repeat line runs each command in
// order with the right args.
func TestParseInput_DispatchPipeline(t *testing.T) {
	r := command.New()
	var calls []string
	rec := func(verb string) command.Handler {
		return func(ctx context.Context, c *command.Context) error {
			calls = append(calls, verb+":"+strings.Join(c.Args, ","))
			return nil
		}
	}
	for _, kw := range []string{"n", "e", "pick"} {
		if err := r.RegisterCommand(command.Command{Keyword: kw, Brief: kw, Handler: rec(kw)}); err != nil {
			t.Fatalf("register %s: %v", kw, err)
		}
	}

	a := newNamedTestActor("Alice", "p1", nil)
	env := command.Env{}
	for _, seg := range command.ParseInput("n;e;2pick gem", 10) {
		if err := r.Dispatch(context.Background(), env, a, seg); err != nil {
			t.Fatalf("dispatch %q: %v", seg, err)
		}
	}

	want := []string{"n:", "e:", "pick:gem", "pick:gem"}
	if !slices.Equal(calls, want) {
		t.Errorf("dispatch order = %v, want %v", calls, want)
	}
}
