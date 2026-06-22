package command_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// fakeGroup is a minimal GroupService for the `lootmode` verb tests: it tracks a
// single party (leader + members) and its loot policy. Only the methods the verb
// touches carry real behavior; the rest are no-ops.
type fakeGroup struct {
	leader  string
	members []string // includes the leader
	mode    command.LootMode
	master  string
	setErr  error // forced error from SetLootMode (leader/membership checks)
}

func (g *fakeGroup) Invite(string, string) error                 { return nil }
func (g *fakeGroup) Accept(string, string) error                 { return nil }
func (g *fakeGroup) Leave(string) (bool, string, []string, bool) { return false, "", nil, false }
func (g *fakeGroup) Disband(string) ([]string, bool)             { return nil, false }
func (g *fakeGroup) Members(string) []string                     { return g.members }
func (g *fakeGroup) LeaderOf(string) (string, bool)              { return g.leader, g.leader != "" }

func (g *fakeGroup) LootPolicy(string) (command.LootMode, string, bool) {
	if g.leader == "" {
		return command.LootFFA, "", false
	}
	if g.mode == command.LootMaster {
		master := g.master
		if master == "" {
			master = g.leader
		}
		return command.LootMaster, master, true
	}
	return command.LootFFA, "", true
}

func (g *fakeGroup) SetLootMode(_ string, mode command.LootMode, master string) (command.LootMode, string, error) {
	if g.setErr != nil {
		return command.LootFFA, "", g.setErr
	}
	if mode == command.LootMaster && master == "" {
		master = g.leader
	}
	g.mode, g.master = mode, master
	return g.mode, g.master, nil
}

// lootModeCtx wires a Context whose actor leads a two-person party (L + Alice),
// with ActorByID resolving both so announcements + name lookups work.
func lootModeCtx(g *fakeGroup, args ...string) (*command.Context, *testActor) {
	room := &world.Room{ID: "z:a"}
	leader := newNamedTestActor("Lead", "L", room)
	alice := newNamedTestActor("Alice", "A", room)
	byID := map[string]command.Actor{"L": leader, "A": alice}
	return &command.Context{
		Actor:     leader,
		Args:      args,
		Group:     g,
		ActorByID: func(id string) (command.Actor, bool) { a, ok := byID[id]; return a, ok },
	}, leader
}

func TestLootMode_StatusFreeForAll(t *testing.T) {
	g := &fakeGroup{leader: "L", members: []string{"L", "A"}}
	c, a := lootModeCtx(g)
	if err := command.LootModeHandler(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if got := a.lastLine(); !strings.Contains(got, "free-for-all") {
		t.Errorf("status = %q, want free-for-all", got)
	}
}

func TestLootMode_StatusMasterNamesMaster(t *testing.T) {
	g := &fakeGroup{leader: "L", members: []string{"L", "A"}, mode: command.LootMaster, master: "A"}
	c, a := lootModeCtx(g)
	if err := command.LootModeHandler(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	got := a.lastLine()
	if !strings.Contains(got, "master-looter") || !strings.Contains(got, "Alice") {
		t.Errorf("status = %q, want master-looter naming Alice", got)
	}
}

func TestLootMode_NotInParty(t *testing.T) {
	g := &fakeGroup{} // no party
	c, a := lootModeCtx(g, "ffa")
	if err := command.LootModeHandler(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if got := a.lastLine(); !strings.Contains(got, "aren't in a party") {
		t.Errorf("msg = %q, want not-in-a-party", got)
	}
}

func TestLootMode_SetMasterByName(t *testing.T) {
	g := &fakeGroup{leader: "L", members: []string{"L", "A"}}
	c, a := lootModeCtx(g, "master", "Alice")
	if err := command.LootModeHandler(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if g.mode != command.LootMaster || g.master != "A" {
		t.Fatalf("policy = (%v,%q), want (master, A)", g.mode, g.master)
	}
	if got := a.lastLine(); !strings.Contains(got, "Alice") || !strings.Contains(got, "master-looter") {
		t.Errorf("announce = %q, want it to name Alice as master-looter", got)
	}
}

func TestLootMode_SetFFA(t *testing.T) {
	g := &fakeGroup{leader: "L", members: []string{"L", "A"}, mode: command.LootMaster, master: "A"}
	c, a := lootModeCtx(g, "ffa")
	if err := command.LootModeHandler(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if g.mode != command.LootFFA {
		t.Fatalf("mode = %v, want free-for-all", g.mode)
	}
	if got := a.lastLine(); !strings.Contains(got, "free-for-all") {
		t.Errorf("announce = %q, want free-for-all", got)
	}
}

func TestLootMode_MasterUnknownMember(t *testing.T) {
	g := &fakeGroup{leader: "L", members: []string{"L", "A"}}
	c, a := lootModeCtx(g, "master", "Stranger")
	if err := command.LootModeHandler(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if got := a.lastLine(); !strings.Contains(got, "isn't in your party") {
		t.Errorf("msg = %q, want isn't-in-your-party", got)
	}
	if g.mode == command.LootMaster {
		t.Error("an unknown master should not have changed the policy")
	}
}

func TestLootMode_NotLeaderRefused(t *testing.T) {
	g := &fakeGroup{leader: "L", members: []string{"L", "A"}, setErr: command.ErrGroupNotLeader}
	c, a := lootModeCtx(g, "ffa")
	if err := command.LootModeHandler(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if got := a.lastLine(); !strings.Contains(got, "leader") {
		t.Errorf("msg = %q, want a leader-only refusal", got)
	}
}

func TestLootMode_JunkSubcommand(t *testing.T) {
	g := &fakeGroup{leader: "L", members: []string{"L", "A"}}
	c, a := lootModeCtx(g, "banana")
	if err := command.LootModeHandler(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if got := a.lastLine(); !strings.Contains(got, "lootmode") {
		t.Errorf("msg = %q, want usage hint", got)
	}
}
