package command_test

import (
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/mob"
)

// dougTpl is a bartender NPC carrying a `dialogue` property: the content shape
// the `ask <npc> about <topic>` verb reads. `laws` is a LIST (rotates on
// repeat), `brian` a single line, and `default` the catch-all fallback.
func dougTpl() *mob.Template {
	return &mob.Template{
		ID:       "tapestry-core:doug",
		Name:     "Doug Coughlin",
		Type:     "npc",
		Behavior: "stationary",
		Keywords: []string{"doug", "bartender"},
		Properties: map[string]any{
			"dialogue": map[string]any{
				"laws": []any{
					"Coughlin's Law: bury the dead.",
					"Coughlin's Law: use your fork.",
				},
				"brian":   "The kid is all flash and ambition.",
				"default": "Buy a drink and think on it.",
			},
		},
	}
}

// muteTpl carries dialogue but no `default` — used to exercise the
// unknown-topic-with-no-fallback branch.
func muteTpl() *mob.Template {
	return &mob.Template{
		ID:       "tapestry-core:mute",
		Name:     "a quiet ork",
		Type:     "npc",
		Behavior: "stationary",
		Keywords: []string{"ork", "quiet"},
		Properties: map[string]any{
			"dialogue": map[string]any{"laws": "just the one law"},
		},
	}
}

// spawnMobInRoom spawns a mob template and places it in the fixture room.
func spawnMobInRoom(t *testing.T, f *invFixture, tpl *mob.Template) *entities.MobInstance {
	t.Helper()
	m, err := f.store.SpawnMob(tpl)
	if err != nil {
		t.Fatalf("SpawnMob: %v", err)
	}
	f.place.Place(m.ID(), f.room.ID)
	return m
}

func TestAsk_TopicReturnsDialogueLine(t *testing.T) {
	// `ask doug about brian` speaks the single authored line, formatted as
	// NPC speech (name + "says").
	f := newInvFixture(t)
	spawnMobInRoom(t, f, dougTpl())
	a := newTestActor(f.room)
	r := newRegistry(t)

	dispatchActor(t, r, f.env(), a, "ask doug about brian")

	out := a.lastLine()
	// The raw line still carries render markup tags (stripped at render time),
	// so assert on the name and the speech verb separately.
	if !strings.Contains(out, "Doug Coughlin") || !strings.Contains(out, "says") {
		t.Errorf("ask output = %q, want it spoken by Doug", out)
	}
	if !strings.Contains(out, "all flash and ambition") {
		t.Errorf("ask output = %q, want the brian dialogue line", out)
	}
}

func TestAsk_ListTopicRotatesDeterministically(t *testing.T) {
	// A list topic (Coughlin's Laws) resolves to a line. With no clock wired
	// in the test env, pickFrom selects index 0 deterministically.
	f := newInvFixture(t)
	spawnMobInRoom(t, f, dougTpl())
	a := newTestActor(f.room)
	r := newRegistry(t)

	dispatchActor(t, r, f.env(), a, "ask doug about laws")

	if got := a.lastLine(); !strings.Contains(got, "bury the dead") {
		t.Errorf("ask about laws = %q, want the first law", got)
	}
}

func TestAsk_UnknownTopicFallsBackToDefault(t *testing.T) {
	f := newInvFixture(t)
	spawnMobInRoom(t, f, dougTpl())
	a := newTestActor(f.room)
	r := newRegistry(t)

	dispatchActor(t, r, f.env(), a, "ask doug about the weather")

	if got := a.lastLine(); !strings.Contains(got, "Buy a drink and think on it") {
		t.Errorf("unknown topic = %q, want the default line", got)
	}
}

func TestAsk_UnknownTopicNoDefaultSaysNothing(t *testing.T) {
	f := newInvFixture(t)
	spawnMobInRoom(t, f, muteTpl())
	a := newTestActor(f.room)
	r := newRegistry(t)

	dispatchActor(t, r, f.env(), a, "ask ork about brian")

	if got := a.lastLine(); !strings.Contains(got, "nothing to say") {
		t.Errorf("unknown topic, no default = %q, want the nothing-to-say line", got)
	}
}

func TestAsk_TopicMatchIsCaseInsensitive(t *testing.T) {
	f := newInvFixture(t)
	spawnMobInRoom(t, f, dougTpl())
	a := newTestActor(f.room)
	r := newRegistry(t)

	dispatchActor(t, r, f.env(), a, "ask doug about BRIAN")

	if got := a.lastLine(); !strings.Contains(got, "all flash and ambition") {
		t.Errorf("case-insensitive topic = %q, want the brian line", got)
	}
}

func TestAsk_NoAboutDelegatesToTalk(t *testing.T) {
	// `ask <npc>` with no topic falls through to the quest-giver `talk`
	// behavior. With no quest service wired, TalkHandler reports there's no
	// one to talk to — proving the delegation path, not a dialogue lookup.
	f := newInvFixture(t)
	spawnMobInRoom(t, f, dougTpl())
	a := newTestActor(f.room)
	r := newRegistry(t)

	dispatchActor(t, r, f.env(), a, "ask doug")

	if got := a.lastLine(); !strings.Contains(got, "talk") {
		t.Errorf("ask with no topic = %q, want delegation to talk", got)
	}
}

func TestAsk_EmptyTopicPrompts(t *testing.T) {
	f := newInvFixture(t)
	spawnMobInRoom(t, f, dougTpl())
	a := newTestActor(f.room)
	r := newRegistry(t)

	dispatchActor(t, r, f.env(), a, "ask doug about")

	if got := a.lastLine(); !strings.Contains(got, "about what") {
		t.Errorf("empty topic = %q, want a prompt", got)
	}
}
