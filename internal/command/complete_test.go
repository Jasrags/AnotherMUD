package command

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/keyword"
)

// --- test fixtures ---

func noopHandler(ctx context.Context, c *Context) error { return nil }

// fakeDoorEnum implements DoorScope AND the optional doorEnumerator so
// the door-completion path has something to enumerate.
type fakeDoorEnum struct{ doors []DoorRef }

func (f fakeDoorEnum) ResolveDoor(arg string) (DoorRef, bool, bool) {
	for _, d := range f.doors {
		if strings.EqualFold(d.Direction, arg) {
			return d, true, false
		}
	}
	return DoorRef{}, false, false
}
func (f fakeDoorEnum) EnumerateDoors() []DoorRef { return f.doors }

func completionRegistry(t *testing.T) *Registry {
	t.Helper()
	r := New()
	cmds := []Command{
		{Keyword: "look", Brief: "look around", Handler: noopHandler},
		{Keyword: "get", Brief: "pick up", Handler: noopHandler,
			Args: []ArgDefinition{{Name: "item", Type: ArgRoomItem, Bulk: true}}},
		{Keyword: "drop", Brief: "drop", Handler: noopHandler,
			Args: []ArgDefinition{{Name: "item", Type: ArgInventory, Bulk: true}}},
		{Keyword: "wear", Brief: "wear", Handler: noopHandler,
			Args: []ArgDefinition{{Name: "item", Type: ArgInventory}}}, // non-bulk

		{Keyword: "kill", Brief: "attack", Handler: noopHandler,
			Args: []ArgDefinition{{Name: "target", Type: ArgNPC}}},
		{Keyword: "greet", Brief: "greet", Handler: noopHandler,
			Args: []ArgDefinition{{Name: "who", Type: ArgPlayer}}},
		{Keyword: "put", Brief: "put", Handler: noopHandler, Args: []ArgDefinition{
			{Name: "item", Type: ArgInventory},
			{Name: "container", Type: ArgContainer, Prepositions: []string{"in"}},
		}},
		{Keyword: "open", Brief: "open", Handler: noopHandler,
			Args: []ArgDefinition{{Name: "door", Type: ArgDoor}}},
		{Keyword: "say", Brief: "say", Handler: noopHandler,
			Args: []ArgDefinition{{Name: "msg", Type: ArgText}}},
		{Keyword: "examine", Brief: "examine", Handler: noopHandler,
			Args: []ArgDefinition{{Name: "t", Type: ArgVisible}}},
		{Keyword: "find", Brief: "find", Handler: noopHandler,
			Args: []ArgDefinition{{Name: "t", Type: ArgFindable}}},
		{Keyword: "accept", Brief: "accept quest", Handler: noopHandler,
			HandParsed: true, Args: []ArgDefinition{{Name: "quest", Type: ArgQuest}}},
		{Keyword: "abandon", Brief: "abandon quest", Handler: noopHandler,
			HandParsed: true, Args: []ArgDefinition{{Name: "quest", Type: ArgActiveQuest}}},
		{Keyword: "unequip", Brief: "unequip", Handler: noopHandler,
			HandParsed: true, Args: []ArgDefinition{{Name: "item", Type: ArgEquipped}}},
		{Keyword: "talk", Brief: "talk", Handler: noopHandler,
			HandParsed: true, Args: []ArgDefinition{{Name: "npc", Type: ArgNPC}}},
		{Keyword: "sell", Brief: "sell", Handler: noopHandler,
			HandParsed: true, Args: []ArgDefinition{{Name: "item", Type: ArgInventory}}},
		{Keyword: "secret", Brief: "secret", Admin: true, Handler: noopHandler},
	}
	for _, c := range cmds {
		if err := r.RegisterCommand(c); err != nil {
			t.Fatalf("register %q: %v", c.Keyword, err)
		}
	}
	// Bare (metadata-less) registrations: an exact-vs-prefix pair.
	if err := r.Register("n", noopHandler); err != nil {
		t.Fatal(err)
	}
	if err := r.Register("north", noopHandler); err != nil {
		t.Fatal(err)
	}
	return r
}

func tokensOf(cands []Candidate) []string {
	out := make([]string, len(cands))
	for i, c := range cands {
		out[i] = c.Completion
	}
	return out
}

func has(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

// --- §2 target detection ---

func TestComplete_Target(t *testing.T) {
	r := completionRegistry(t)
	cases := []struct {
		partial string
		want    CompletionKind
		verb    string
	}{
		{"look", CompleteVerb, ""},
		{"loo", CompleteVerb, ""},
		{"", CompleteVerb, ""},
		{"get ", CompleteArgument, "get"},
		{"get sw", CompleteArgument, "get"},
		{"frobnicate x", CompleteNone, ""},
	}
	for _, tc := range cases {
		got := r.Complete(tc.partial, ResolveContext{}, CompletionOptions{IsAdmin: true})
		if got.Target != tc.want {
			t.Errorf("Complete(%q).Target = %v, want %v", tc.partial, got.Target, tc.want)
		}
		if tc.verb != "" && got.Verb != tc.verb {
			t.Errorf("Complete(%q).Verb = %q, want %q", tc.partial, got.Verb, tc.verb)
		}
	}
}

// --- §3 verb enumeration ---

func TestComplete_Verb_PrefixAndExactFirst(t *testing.T) {
	r := completionRegistry(t)

	got := tokensOf(r.Complete("n", ResolveContext{}, CompletionOptions{}).Candidates)
	if len(got) == 0 || got[0] != "n" {
		t.Errorf("partial %q: exact match should sort first, got %v", "n", got)
	}
	if !has(got, "north") {
		t.Errorf("partial %q: want prefix match %q in %v", "n", "north", got)
	}

	if loo := tokensOf(r.Complete("loo", ResolveContext{}, CompletionOptions{}).Candidates); !has(loo, "look") {
		t.Errorf("partial %q: want %q, got %v", "loo", "look", loo)
	}
	if none := r.Complete("xyzzy", ResolveContext{}, CompletionOptions{}); len(none.Candidates) != 0 {
		t.Errorf("partial %q: want no candidates, got %v", "xyzzy", tokensOf(none.Candidates))
	}
}

func TestComplete_Verb_AdminGate(t *testing.T) {
	r := completionRegistry(t)
	pleb := tokensOf(r.Complete("s", ResolveContext{}, CompletionOptions{IsAdmin: false}).Candidates)
	if has(pleb, "secret") {
		t.Errorf("non-admin must not see admin verb %q, got %v", "secret", pleb)
	}
	if !has(pleb, "say") {
		t.Errorf("non-admin should still see %q, got %v", "say", pleb)
	}
	admin := tokensOf(r.Complete("s", ResolveContext{}, CompletionOptions{IsAdmin: true}).Candidates)
	if !has(admin, "secret") {
		t.Errorf("admin should see %q, got %v", "secret", admin)
	}
}

func TestComplete_Verb_MetadatalessStillOffered(t *testing.T) {
	r := completionRegistry(t)
	got := tokensOf(r.Complete("nor", ResolveContext{}, CompletionOptions{}).Candidates)
	if !has(got, "north") {
		t.Errorf("a bare (metadata-less) verb should still complete, got %v", got)
	}
}

func TestComplete_Verb_EveryCandidateRoutes(t *testing.T) {
	r := completionRegistry(t)
	res := r.Complete("", ResolveContext{}, CompletionOptions{IsAdmin: true})
	if len(res.Candidates) == 0 {
		t.Fatal("empty partial should enumerate all verbs")
	}
	for _, c := range res.Candidates {
		if r.Resolve(c.Completion) == nil {
			t.Errorf("verb candidate %q does not route to a handler", c.Completion)
		}
	}
}

// --- §4 argument enumeration ---

func TestComplete_Arg_RoomItem(t *testing.T) {
	r := completionRegistry(t)
	rc := ResolveContext{RoomItems: []ItemCandidate{
		&fakeItem{id: "i1", name: "a sword", keywords: []string{"sword"}},
	}}
	if got := tokensOf(r.Complete("get sw", rc, CompletionOptions{}).Candidates); !has(got, "sword") {
		t.Errorf("get sw: want %q, got %v", "sword", got)
	}
	if got := r.Complete("get xq", rc, CompletionOptions{}); len(got.Candidates) != 0 {
		t.Errorf("get xq: want none, got %v", tokensOf(got.Candidates))
	}
}

func TestComplete_Arg_InventoryBulkAll(t *testing.T) {
	r := completionRegistry(t)
	rc := ResolveContext{Inventory: []ItemCandidate{
		&fakeItem{id: "i1", name: "a ruby", keywords: []string{"ruby"}},
		&fakeItem{id: "i2", name: "a torch", keywords: []string{"torch"}},
	}}
	got := tokensOf(r.Complete("drop ", rc, CompletionOptions{}).Candidates)
	if !has(got, "ruby") || !has(got, "torch") {
		t.Errorf("drop (empty partial) should list all carried items, got %v", got)
	}
	if !has(got, "all") {
		t.Errorf("bulk slot should offer %q, got %v", "all", got)
	}
}

func TestComplete_Arg_NPC_RoundTrips(t *testing.T) {
	r := completionRegistry(t)
	mob := &fakeEntity{id: "m1", name: "a grizzled bandit", keywords: []string{"grizzled", "bandit"}, kind: "mob"}
	rc := ResolveContext{RoomEntities: []EntityCandidate{mob}}
	res := r.Complete("kill ban", rc, CompletionOptions{})
	if len(res.Candidates) != 1 {
		t.Fatalf("kill ban: want 1 candidate, got %v", tokensOf(res.Candidates))
	}
	// §1 invariant: the completion token resolves back to the same mob.
	scope := entitiesAsNamed(rc.RoomEntities)
	if got := keyword.Resolve(scope, res.Candidates[0].Completion); got == nil || candidateID(got) != "m1" {
		t.Errorf("completion %q did not round-trip to the bandit", res.Candidates[0].Completion)
	}
}

func TestComplete_Arg_PlayerVsNpcScope(t *testing.T) {
	r := completionRegistry(t)
	rc := ResolveContext{RoomEntities: []EntityCandidate{
		&fakeEntity{id: "p1", name: "Alice", kind: "player"},
		&fakeEntity{id: "m1", name: "a bandit", keywords: []string{"bandit"}, kind: "mob"},
	}}
	// kill is npc-typed: no players.
	kill := tokensOf(r.Complete("kill ", rc, CompletionOptions{}).Candidates)
	if has(kill, "alice") || !has(kill, "bandit") {
		t.Errorf("npc slot should list mobs only, got %v", kill)
	}
	// greet is player-typed: no mobs.
	greet := tokensOf(r.Complete("greet ", rc, CompletionOptions{}).Candidates)
	if has(greet, "bandit") || !has(greet, "alice") {
		t.Errorf("player slot should list players only, got %v", greet)
	}
}

func TestComplete_Arg_Door(t *testing.T) {
	r := completionRegistry(t)
	rc := ResolveContext{Doors: fakeDoorEnum{doors: []DoorRef{
		{Direction: "n", Door: DoorInfo{Name: "iron gate", Keywords: []string{"iron", "gate"}}},
	}}}
	for _, partial := range []string{"open ", "open ga", "open iro", "open n"} {
		got := tokensOf(r.Complete(partial, rc, CompletionOptions{}).Candidates)
		if !has(got, "n") {
			t.Errorf("%q: door should complete to direction %q, got %v", partial, "n", got)
		}
	}
}

// The door filter matches the door's KEYWORDS, not its name words —
// content may set Keywords that diverge from the name. A door named
// "oak door" addressable only by "portal" is offered for "portal" and
// NOT for "oak" (parity with the resolver, which matches Keywords).
func TestComplete_Arg_Door_KeywordsNotNameWords(t *testing.T) {
	r := completionRegistry(t)
	rc := ResolveContext{Doors: fakeDoorEnum{doors: []DoorRef{
		{Direction: "e", Door: DoorInfo{Name: "oak door", Keywords: []string{"portal"}}},
	}}}
	if got := tokensOf(r.Complete("open portal", rc, CompletionOptions{}).Candidates); !has(got, "e") {
		t.Errorf("door should complete on its keyword %q, got %v", "portal", got)
	}
	if got := tokensOf(r.Complete("open oak", rc, CompletionOptions{}).Candidates); has(got, "e") {
		t.Errorf("door must NOT match a name word %q absent from its keywords, got %v", "oak", got)
	}
}

// fakeQuestScope is a deterministic QuestScope: refs feed ArgQuest
// (accept offers); active feeds ArgActiveQuest (abandon).
type fakeQuestScope struct {
	refs   []QuestRef
	active []QuestRef
}

func (f fakeQuestScope) EnumerateAcceptable() []QuestRef { return f.refs }
func (f fakeQuestScope) EnumerateActive() []QuestRef     { return f.active }

func TestComplete_Arg_Quest(t *testing.T) {
	r := completionRegistry(t)
	rc := ResolveContext{Quests: fakeQuestScope{refs: []QuestRef{
		{BareID: "gate-patrol", Name: "Gate Patrol"},
		{BareID: "forge-errand", Name: "Forge Errand"},
	}}}

	// Empty slot lists every offer; the completion token is the bare id
	// (round-trips through ResolveID), Display is the friendly name.
	all := r.Complete("accept ", rc, CompletionOptions{}).Candidates
	if got := tokensOf(all); !has(got, "gate-patrol") || !has(got, "forge-errand") {
		t.Fatalf("accept (empty): want both bare ids, got %v", got)
	}
	for _, c := range all {
		if c.Kind != CandQuest {
			t.Errorf("candidate %q kind = %q, want %q", c.Completion, c.Kind, CandQuest)
		}
	}

	// Prefix on the bare id.
	if got := tokensOf(r.Complete("accept ga", rc, CompletionOptions{}).Candidates); !has(got, "gate-patrol") || has(got, "forge-errand") {
		t.Errorf("accept ga: want only gate-patrol, got %v", got)
	}
	// Prefix on the display NAME also matches ("forge" → Forge Errand).
	if got := tokensOf(r.Complete("accept forge", rc, CompletionOptions{}).Candidates); !has(got, "forge-errand") {
		t.Errorf("accept forge: name prefix should match, got %v", got)
	}
}

// abandon completes the actor's ACTIVE quests (EnumerateActive), a
// distinct set from accept's room offers (EnumerateAcceptable).
func TestComplete_Arg_ActiveQuest(t *testing.T) {
	r := completionRegistry(t)
	rc := ResolveContext{Quests: fakeQuestScope{
		refs:   []QuestRef{{BareID: "forge-errand", Name: "Forge Errand"}}, // offered (accept)
		active: []QuestRef{{BareID: "gate-patrol", Name: "Gate Patrol"}},   // active (abandon)
	}}

	// abandon enumerates the ACTIVE set, not the offers.
	aband := tokensOf(r.Complete("abandon ", rc, CompletionOptions{}).Candidates)
	if !has(aband, "gate-patrol") || has(aband, "forge-errand") {
		t.Errorf("abandon: want only the active quest gate-patrol, got %v", aband)
	}
	// accept still enumerates the OFFER set — the two scopes are disjoint.
	acc := tokensOf(r.Complete("accept ", rc, CompletionOptions{}).Candidates)
	if !has(acc, "forge-errand") || has(acc, "gate-patrol") {
		t.Errorf("accept: want only the offered quest forge-errand, got %v", acc)
	}
	// Prefix + CandQuest kind on abandon.
	res := r.Complete("abandon ga", rc, CompletionOptions{})
	if got := tokensOf(res.Candidates); !has(got, "gate-patrol") {
		t.Errorf("abandon ga: want gate-patrol, got %v", got)
	}
	for _, c := range res.Candidates {
		if c.Kind != CandQuest {
			t.Errorf("abandon candidate %q kind = %q, want %q", c.Completion, c.Kind, CandQuest)
		}
	}
}

// A nil quest scope (quests unwired / no givers in the room) yields no
// candidates rather than crashing.
func TestComplete_Arg_Quest_NilScope(t *testing.T) {
	r := completionRegistry(t)
	res := r.Complete("accept ga", ResolveContext{}, CompletionOptions{})
	if res.Target != CompleteArgument || res.Verb != "accept" {
		t.Fatalf("want argument target for accept, got %+v", res)
	}
	if len(res.Candidates) != 0 {
		t.Errorf("nil quest scope: want no candidates, got %v", tokensOf(res.Candidates))
	}
}

// unequip completes the actor's EQUIPPED items (rc.Equipped), distinct
// from inventory; the token round-trips through the equipped scope.
func TestComplete_Arg_Equipped(t *testing.T) {
	r := completionRegistry(t)
	rc := ResolveContext{
		Inventory: []ItemCandidate{&fakeItem{id: "i1", name: "a torch", keywords: []string{"torch"}}},
		Equipped:  []ItemCandidate{&fakeItem{id: "e1", name: "a short sword", keywords: []string{"sword"}}},
	}
	// unequip enumerates the equipped set...
	if got := tokensOf(r.Complete("unequip sw", rc, CompletionOptions{}).Candidates); !has(got, "sword") {
		t.Errorf("unequip sw: want sword (equipped), got %v", got)
	}
	// ...and NOT inventory items.
	if got := tokensOf(r.Complete("unequip to", rc, CompletionOptions{}).Candidates); has(got, "torch") {
		t.Errorf("unequip must not complete inventory items, got %v", got)
	}
}

// talk completes room NPCs (ArgNPC) — mobs, not players or items.
func TestComplete_Arg_TalkNPC(t *testing.T) {
	r := completionRegistry(t)
	mob := &fakeEntity{id: "m1", name: "Maerys the Training Master", keywords: []string{"maerys", "master"}, kind: "mob"}
	rc := ResolveContext{RoomEntities: []EntityCandidate{mob}}
	if got := tokensOf(r.Complete("talk ma", rc, CompletionOptions{}).Candidates); !has(got, "maerys") && !has(got, "master") {
		t.Errorf("talk ma: want the Maerys NPC, got %v", got)
	}
}

func TestComplete_Arg_FreeTypesNoCandidates(t *testing.T) {
	r := completionRegistry(t)
	res := r.Complete("say hel", ResolveContext{}, CompletionOptions{})
	if res.Target != CompleteArgument {
		t.Errorf("say hel: want argument target, got %v", res.Target)
	}
	if len(res.Candidates) != 0 {
		t.Errorf("text arg should yield no candidates, got %v", tokensOf(res.Candidates))
	}
}

func TestComplete_Arg_VisibleSpansSelfItemsEntities(t *testing.T) {
	r := completionRegistry(t)
	rc := ResolveContext{
		ActorName:    "Hero",
		Inventory:    []ItemCandidate{&fakeItem{id: "i1", name: "a hat", keywords: []string{"hat"}}},
		RoomEntities: []EntityCandidate{&fakeEntity{id: "m1", name: "a hound", keywords: []string{"hound"}, kind: "mob"}},
	}
	// `visible` self-matches the actor's own name (spec §4).
	if got := tokensOf(r.Complete("examine he", rc, CompletionOptions{}).Candidates); !has(got, "Hero") {
		t.Errorf("visible slot should offer self %q, got %v", "Hero", got)
	}
	// `visible` spans both carried items and room entities.
	all := tokensOf(r.Complete("examine ", rc, CompletionOptions{}).Candidates)
	if !has(all, "hat") || !has(all, "hound") {
		t.Errorf("visible slot should span items + entities, got %v", all)
	}
}

func TestComplete_Arg_Findable(t *testing.T) {
	r := completionRegistry(t)
	rc := ResolveContext{
		Inventory: []ItemCandidate{&fakeItem{id: "i1", name: "a key", keywords: []string{"key"}}},
		RoomItems: []ItemCandidate{&fakeItem{id: "i2", name: "a lever", keywords: []string{"lever"}}},
	}
	got := tokensOf(r.Complete("find ", rc, CompletionOptions{}).Candidates)
	if !has(got, "key") || !has(got, "lever") {
		t.Errorf("findable slot should span inventory + room items, got %v", got)
	}
}

// --- §5 preposition slot mapping ---

func TestComplete_Prepositions(t *testing.T) {
	r := completionRegistry(t)
	rc := ResolveContext{
		Inventory: []ItemCandidate{&fakeItem{id: "g1", name: "a gem", keywords: []string{"gem"}}},
		RoomItems: []ItemCandidate{&fakeItem{id: "c1", name: "a chest", keywords: []string{"chest"}, container: true}},
	}
	cases := []struct {
		partial string
		want    string // expected completion token present
	}{
		{"put ", "gem"},            // first slot = inventory
		{"put gem in ch", "chest"}, // preposition consumed → container slot
		{"put gem ch", "chest"},    // preposition omitted → still container slot
		{"put gem in ", "chest"},   // trailing space after prep → container slot, unfiltered
	}
	for _, tc := range cases {
		got := tokensOf(r.Complete(tc.partial, rc, CompletionOptions{}).Candidates)
		if !has(got, tc.want) {
			t.Errorf("%q: want %q in candidates, got %v", tc.partial, tc.want, got)
		}
	}
}

// --- §6 disambiguation ---

func TestComplete_Disambiguation_DistinctNames(t *testing.T) {
	r := completionRegistry(t)
	gold := &fakeItem{id: "g", name: "a gold ring", keywords: []string{"ring", "gold"}}
	silver := &fakeItem{id: "s", name: "a silver ring", keywords: []string{"ring", "silver"}}
	rc := ResolveContext{Inventory: []ItemCandidate{gold, silver}}

	res := r.Complete("wear ri", rc, CompletionOptions{})
	if len(res.Candidates) != 2 {
		t.Fatalf("want 2 ring candidates, got %v", tokensOf(res.Candidates))
	}
	scope := itemsAsNamed(rc.Inventory)
	// Map each completion token back through the resolver and confirm it
	// lands on the ring whose name the candidate displayed (§1 round-trip
	// for the distinguishing-keyword mechanism specifically).
	byName := map[string]string{"a gold ring": "g", "a silver ring": "s"}
	for _, c := range res.Candidates {
		got := keyword.Resolve(scope, c.Completion)
		if got == nil {
			t.Errorf("completion %q resolves to nothing", c.Completion)
			continue
		}
		if c.Completion == "ring" {
			t.Errorf("ambiguous token %q offered for distinguishable rings", c.Completion)
		}
		if wantID := byName[c.Display]; candidateID(got) != wantID {
			t.Errorf("token %q (display %q) round-tripped to id %q, want %q",
				c.Completion, c.Display, candidateID(got), wantID)
		}
	}
	if dup := tokensOf(res.Candidates); dup[0] == dup[1] {
		t.Errorf("completion tokens must be unique, got %v", dup)
	}
}

func TestComplete_Disambiguation_OrdinalDupes(t *testing.T) {
	r := completionRegistry(t)
	rc := ResolveContext{Inventory: []ItemCandidate{
		&fakeItem{id: "r1", name: "a ring", keywords: []string{"ring"}},
		&fakeItem{id: "r2", name: "a ring", keywords: []string{"ring"}},
		&fakeItem{id: "r3", name: "a ring", keywords: []string{"ring"}},
	}}
	res := r.Complete("wear ring", rc, CompletionOptions{})
	got := tokensOf(res.Candidates)
	want := []string{"ring", "2.ring", "3.ring"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ordinal tokens = %v, want %v", got, want)
	}
	scope := itemsAsNamed(rc.Inventory)
	wantIDs := []string{"r1", "r2", "r3"}
	for i, tok := range got {
		if m := keyword.Resolve(scope, tok); m == nil || candidateID(m) != wantIDs[i] {
			t.Errorf("token %q should resolve to %q", tok, wantIDs[i])
		}
	}
}

func TestComplete_Disambiguation_Mixed(t *testing.T) {
	r := completionRegistry(t)
	gold := &fakeItem{id: "g", name: "a gold ring", keywords: []string{"ring", "gold"}}
	rc := ResolveContext{Inventory: []ItemCandidate{
		gold,
		&fakeItem{id: "r2", name: "a ring", keywords: []string{"ring"}},
		&fakeItem{id: "r3", name: "a ring", keywords: []string{"ring"}},
	}}
	res := r.Complete("wear ring", rc, CompletionOptions{})
	got := tokensOf(res.Candidates)
	if !has(got, "gold") {
		t.Errorf("gold ring should get its distinguishing token, got %v", got)
	}
	// No two completion tokens collide.
	seen := map[string]bool{}
	for _, tkn := range got {
		if seen[tkn] {
			t.Errorf("duplicate completion token %q in %v", tkn, got)
		}
		seen[tkn] = true
	}
	// Every token round-trips to a distinct entity.
	scope := itemsAsNamed(rc.Inventory)
	for _, tkn := range got {
		if keyword.Resolve(scope, tkn) == nil {
			t.Errorf("token %q resolves to nothing", tkn)
		}
	}
}

// --- §7 caps & determinism ---

func TestComplete_Cap(t *testing.T) {
	r := completionRegistry(t)
	rc := ResolveContext{Inventory: []ItemCandidate{
		&fakeItem{id: "1", name: "a", keywords: []string{"aa"}},
		&fakeItem{id: "2", name: "b", keywords: []string{"bb"}},
		&fakeItem{id: "3", name: "c", keywords: []string{"cc"}},
		&fakeItem{id: "4", name: "d", keywords: []string{"dd"}},
		&fakeItem{id: "5", name: "e", keywords: []string{"ee"}},
	}}
	res := r.Complete("drop ", rc, CompletionOptions{Cap: 3})
	if len(res.Candidates) != 3 || !res.Truncated {
		t.Errorf("cap 3 over 5 items: got %d candidates truncated=%v, want 3 truncated=true",
			len(res.Candidates), res.Truncated)
	}
	full := r.Complete("drop ", rc, CompletionOptions{Cap: 50})
	if full.Truncated {
		t.Errorf("cap 50 over 5 items should not truncate")
	}
}

func TestComplete_Deterministic(t *testing.T) {
	r := completionRegistry(t)
	rc := ResolveContext{RoomItems: []ItemCandidate{
		&fakeItem{id: "1", name: "a sword", keywords: []string{"sword"}},
		&fakeItem{id: "2", name: "a shield", keywords: []string{"shield"}},
	}}
	a := r.Complete("get s", rc, CompletionOptions{})
	b := r.Complete("get s", rc, CompletionOptions{})
	if !reflect.DeepEqual(a, b) {
		t.Errorf("identical queries diverged:\n a=%+v\n b=%+v", a, b)
	}
}

// An alias inherits its primary's declared args, so completing through
// the alias keyword behaves like the primary (here: close's `shut`).
func TestComplete_AliasCarriesArgs(t *testing.T) {
	r := New()
	if err := RegisterBuiltins(r); err != nil {
		t.Fatal(err)
	}
	rc := ResolveContext{Doors: fakeDoorEnum{doors: []DoorRef{
		{Direction: "n", Door: DoorInfo{Name: "oak door", Keywords: []string{"oak", "door"}}},
	}}}
	if got := tokensOf(r.Complete("shut o", rc, CompletionOptions{}).Candidates); !has(got, "n") {
		t.Errorf("alias `shut` should complete doors like `close`, got %v", got)
	}
}

// --- §8 degradation ---

func TestComplete_Degradation(t *testing.T) {
	r := completionRegistry(t)

	// Unknown verb under an argument slot: no candidates, no panic.
	if res := r.Complete("frobnicate xyz", ResolveContext{}, CompletionOptions{}); res.Target != CompleteNone || len(res.Candidates) != 0 {
		t.Errorf("unknown verb arg: got target=%v candidates=%v", res.Target, tokensOf(res.Candidates))
	}

	// A verb with no declared args: argument target, no candidates.
	res := r.Complete("look foo", ResolveContext{}, CompletionOptions{})
	if res.Target != CompleteArgument || len(res.Candidates) != 0 {
		t.Errorf("no-arg verb: got target=%v candidates=%v", res.Target, tokensOf(res.Candidates))
	}

	// Nil/empty resolve context: empty set, no panic.
	if res := r.Complete("get sw", ResolveContext{}, CompletionOptions{}); len(res.Candidates) != 0 {
		t.Errorf("empty context: want no candidates, got %v", tokensOf(res.Candidates))
	}
}
