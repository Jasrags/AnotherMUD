package command

import (
	"errors"
	"testing"
)

// The visibility seam (visibility §5.4): the room-scoped resolvers filter
// their candidates through ResolveContext.CanSee, so a command cannot
// target an entity/item the actor cannot see — unless the arg bypasses the
// filter or no predicate is set.

// hideOnly returns a CanSee predicate that conceals exactly the given id.
func hideOnly(hiddenID string) func(string) bool {
	return func(id string) bool { return id != hiddenID }
}

func TestResolve_Entity_VisibilityHidesUnseen(t *testing.T) {
	r := resolveDefault()
	ctx := ResolveContext{
		RoomEntities: []EntityCandidate{
			&fakeEntity{id: "m1", name: "a town guard", keywords: []string{"guard"}, kind: "mob"},
			&fakeEntity{id: "m2", name: "a sewer rat", keywords: []string{"rat"}, kind: "mob"},
		},
		CanSee: hideOnly("m2"), // the rat is concealed (e.g. darkness)
	}

	// The visible guard resolves.
	res, _, _, err := r.ResolveArgsWithContext(
		[]ArgDefinition{{Name: "target", Type: ArgEntity}}, []string{"guard"}, ctx)
	if err != nil {
		t.Fatalf("visible target should resolve: %v", err)
	}
	if ref := res["target"].(EntityRef); ref.ID != "m1" {
		t.Errorf("resolved %q, want m1", ref.ID)
	}

	// The concealed rat resolves as if it were not there.
	_, _, _, err = r.ResolveArgsWithContext(
		[]ArgDefinition{{Name: "target", Type: ArgEntity}}, []string{"rat"}, ctx)
	if !errors.Is(err, ErrEntityNotInRoom) {
		t.Errorf("concealed target err = %v, want ErrEntityNotInRoom", err)
	}
}

func TestResolve_Entity_BypassVisibilitySeesConcealed(t *testing.T) {
	r := resolveDefault()
	ctx := ResolveContext{
		RoomEntities: []EntityCandidate{
			&fakeEntity{id: "m2", name: "a sewer rat", keywords: []string{"rat"}, kind: "mob"},
		},
		CanSee: hideOnly("m2"),
	}
	// An arg flagged BypassVisibility reaches the concealed target (admin verbs).
	res, _, _, err := r.ResolveArgsWithContext(
		[]ArgDefinition{{Name: "target", Type: ArgEntity, BypassVisibility: true}},
		[]string{"rat"}, ctx)
	if err != nil {
		t.Fatalf("bypass should reach concealed target: %v", err)
	}
	if ref := res["target"].(EntityRef); ref.ID != "m2" {
		t.Errorf("resolved %q, want m2", ref.ID)
	}
}

func TestResolve_Entity_NilPredicateLegacyVisible(t *testing.T) {
	r := resolveDefault()
	ctx := ResolveContext{
		RoomEntities: []EntityCandidate{
			&fakeEntity{id: "m2", name: "a sewer rat", keywords: []string{"rat"}, kind: "mob"},
		},
		// CanSee nil — the legacy permissive path (tests / lit rooms).
	}
	res, _, _, err := r.ResolveArgsWithContext(
		[]ArgDefinition{{Name: "target", Type: ArgEntity}}, []string{"rat"}, ctx)
	if err != nil {
		t.Fatalf("nil predicate must not filter: %v", err)
	}
	if ref := res["target"].(EntityRef); ref.ID != "m2" {
		t.Errorf("resolved %q, want m2", ref.ID)
	}
}

func TestResolve_RoomItem_VisibilityHidesUnseen(t *testing.T) {
	r := resolveDefault()
	ctx := ResolveContext{
		RoomItems: []ItemCandidate{
			&fakeItem{id: "i1", name: "a glowing torch", keywords: []string{"torch"}},
			&fakeItem{id: "i2", name: "a dull coin", keywords: []string{"coin"}},
		},
		CanSee: hideOnly("i2"), // the coin is in shadow; the torch glows
	}

	if _, _, _, err := r.ResolveArgsWithContext(
		[]ArgDefinition{{Name: "it", Type: ArgRoomItem}}, []string{"torch"}, ctx); err != nil {
		t.Errorf("luminous torch should resolve: %v", err)
	}
	if _, _, _, err := r.ResolveArgsWithContext(
		[]ArgDefinition{{Name: "it", Type: ArgRoomItem}}, []string{"coin"}, ctx); !errors.Is(err, ErrItemNotInRoom) {
		t.Errorf("concealed coin err = %v, want ErrItemNotInRoom", err)
	}
}

// resolveContainer's room branch filters through the predicate (its
// inventory branch is never filtered — you see your own containers).
func TestResolve_Container_RoomBranchVisibility(t *testing.T) {
	r := resolveDefault()
	ctx := ResolveContext{
		RoomItems: []ItemCandidate{
			&fakeItem{id: "c1", name: "an iron chest", keywords: []string{"chest"}, container: true},
		},
		CanSee: hideOnly("c1"),
	}
	if _, _, _, err := r.ResolveArgsWithContext(
		[]ArgDefinition{{Name: "c", Type: ArgContainer}}, []string{"chest"}, ctx); !errors.Is(err, ErrContainerNotFound) {
		t.Errorf("concealed room container err = %v, want ErrContainerNotFound", err)
	}
	// Bypass reaches it.
	if _, _, _, err := r.ResolveArgsWithContext(
		[]ArgDefinition{{Name: "c", Type: ArgContainer, BypassVisibility: true}}, []string{"chest"}, ctx); err != nil {
		t.Errorf("bypass should reach concealed container: %v", err)
	}
}

// resolveFindable's room branch filters; its inventory branch does not.
func TestResolve_Findable_RoomBranchVisibility(t *testing.T) {
	r := resolveDefault()
	ctx := ResolveContext{
		Inventory: []ItemCandidate{
			&fakeItem{id: "inv", name: "a carried map", keywords: []string{"map"}},
		},
		RoomItems: []ItemCandidate{
			&fakeItem{id: "r1", name: "a stone tablet", keywords: []string{"tablet"}},
		},
		CanSee: func(id string) bool { return id == "inv" }, // room hidden, inventory not
	}
	// Carried item still findable (inventory never filtered).
	if _, _, _, err := r.ResolveArgsWithContext(
		[]ArgDefinition{{Name: "f", Type: ArgFindable}}, []string{"map"}, ctx); err != nil {
		t.Errorf("carried item should be findable: %v", err)
	}
	// Concealed room item is not.
	if _, _, _, err := r.ResolveArgsWithContext(
		[]ArgDefinition{{Name: "f", Type: ArgFindable}}, []string{"tablet"}, ctx); !errors.Is(err, ErrNotFindable) {
		t.Errorf("concealed room item err = %v, want ErrNotFindable", err)
	}
}

// resolveVisible matches the actor's own name as "self" BEFORE consulting
// the (filtered) candidate lists — self is always visible (§2.1) even with
// a deny-all predicate.
func TestResolve_Visible_SelfAlwaysResolvesUnderConcealment(t *testing.T) {
	r := resolveDefault()
	ctx := ResolveContext{
		ActorName: "Aiel",
		ActorID:   "p1",
		CanSee:    func(string) bool { return false }, // pitch black
	}
	res, _, _, err := r.ResolveArgsWithContext(
		[]ArgDefinition{{Name: "t", Type: ArgVisible}}, []string{"Aiel"}, ctx)
	if err != nil {
		t.Fatalf("self should resolve even in total darkness: %v", err)
	}
	if ref := res["t"].(VisibleRef); ref.Source != "self" || ref.ID != "p1" {
		t.Errorf("resolved %+v, want self/p1", ref)
	}
}

// The NPC and player resolvers filter through the same predicate.
func TestResolve_NPCAndPlayer_VisibilityHidesUnseen(t *testing.T) {
	r := resolveDefault()
	ctx := ResolveContext{
		RoomEntities: []EntityCandidate{
			&fakeEntity{id: "m2", name: "a sewer rat", keywords: []string{"rat"}, kind: "mob"},
			&fakeEntity{id: "p2", name: "Borin", keywords: []string{"borin"}, kind: "player"},
		},
		CanSee: func(string) bool { return false }, // pitch black: nothing visible
	}
	if _, _, _, err := r.ResolveArgsWithContext(
		[]ArgDefinition{{Name: "t", Type: ArgNPC}}, []string{"rat"}, ctx); !errors.Is(err, ErrNpcNotInRoom) {
		t.Errorf("npc err = %v, want ErrNpcNotInRoom", err)
	}
	if _, _, _, err := r.ResolveArgsWithContext(
		[]ArgDefinition{{Name: "t", Type: ArgPlayer}}, []string{"borin"}, ctx); !errors.Is(err, ErrPlayerNotInRoom) {
		t.Errorf("player err = %v, want ErrPlayerNotInRoom", err)
	}
}
