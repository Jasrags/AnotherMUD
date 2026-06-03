package command

import "testing"

// get/take/kill declare their arg shape for completion (Command.HandParsed)
// even though their handlers parse the args themselves. This is what makes
// the most common targeting verbs completable.
func TestComplete_HandParsedVerbs(t *testing.T) {
	r := New()
	if err := RegisterBuiltins(r); err != nil {
		t.Fatal(err)
	}
	rc := ResolveContext{
		RoomItems: []ItemCandidate{
			&fakeItem{id: "i1", name: "a short sword", keywords: []string{"sword", "short"}},
		},
		RoomEntities: []EntityCandidate{
			&fakeEntity{id: "m1", name: "a road bandit", keywords: []string{"bandit", "rogue"}, kind: "mob"},
		},
	}
	// get → room-item scope.
	if got := tokensOf(r.Complete("get sw", rc, CompletionOptions{}).Candidates); !has(got, "sword") {
		t.Errorf("get sw: want sword, got %v", got)
	}
	// take is an alias of get and must complete identically (alias args).
	if got := tokensOf(r.Complete("take sw", rc, CompletionOptions{}).Candidates); !has(got, "sword") {
		t.Errorf("take sw (get alias): want sword, got %v", got)
	}
	// kill → entity scope, matched on a keyword absent from the name.
	if got := tokensOf(r.Complete("kill rog", rc, CompletionOptions{}).Candidates); !has(got, "rogue") {
		t.Errorf("kill rog: want rogue (keyword match), got %v", got)
	}
	// get ... from <container>: the `from` preposition maps the cursor to
	// the container slot (proves the second declared arg + preposition).
	rc.RoomItems = append(rc.RoomItems, &fakeItem{id: "c1", name: "a wooden crate", keywords: []string{"crate"}, container: true})
	if got := tokensOf(r.Complete("get sword from cr", rc, CompletionOptions{}).Candidates); !has(got, "crate") {
		t.Errorf("get sword from cr: want crate (container slot), got %v", got)
	}
}

// look and consider are hand-parsed but declare their target scope for
// completion: look → visible (incl. at/in prepositions), consider → entity.
func TestComplete_LookAndConsider(t *testing.T) {
	r := New()
	if err := RegisterBuiltins(r); err != nil {
		t.Fatal(err)
	}
	rc := ResolveContext{
		Inventory:    []ItemCandidate{&fakeItem{id: "i1", name: "a short sword", keywords: []string{"sword"}}},
		RoomEntities: []EntityCandidate{&fakeEntity{id: "m1", name: "a road bandit", keywords: []string{"bandit"}, kind: "mob"}},
	}
	// look → visible scope (carried sword + room bandit).
	if got := tokensOf(r.Complete("look sw", rc, CompletionOptions{}).Candidates); !has(got, "sword") {
		t.Errorf("look sw: want sword (visible), got %v", got)
	}
	// look at <target>: the `at` preposition maps to the visible slot.
	if got := tokensOf(r.Complete("look at ban", rc, CompletionOptions{}).Candidates); !has(got, "bandit") {
		t.Errorf("look at ban: want bandit (visible via `at`), got %v", got)
	}
	// consider → entity scope (room mobs/players).
	if got := tokensOf(r.Complete("consider ban", rc, CompletionOptions{}).Candidates); !has(got, "bandit") {
		t.Errorf("consider ban: want bandit (entity), got %v", got)
	}
	// con is consider's alias and inherits its args.
	if got := tokensOf(r.Complete("con ban", rc, CompletionOptions{}).Candidates); !has(got, "bandit") {
		t.Errorf("con ban (consider alias): want bandit, got %v", got)
	}
}
