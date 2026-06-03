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
