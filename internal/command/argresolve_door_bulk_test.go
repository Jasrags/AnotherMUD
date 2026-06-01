package command

import (
	"errors"
	"testing"
)

// --- ordinal selection (§5.5), through the resolvers ---
//
// Ordinals are handled by keyword.Resolve (the resolvers call it
// directly), so these tests pin the §5.5 acceptance criterion
// "ordinal selectors work uniformly across selecting types" at the
// command-layer boundary rather than re-testing keyword internals.

func TestResolve_Inventory_OrdinalSelectsNth(t *testing.T) {
	r := resolveDefault()
	ctx := ResolveContext{
		Inventory: []ItemCandidate{
			&fakeItem{id: "sword-1", name: "a long sword", keywords: []string{"sword"}},
			&fakeItem{id: "sword-2", name: "a short sword", keywords: []string{"sword"}},
			&fakeItem{id: "sword-3", name: "a bent sword", keywords: []string{"sword"}},
		},
	}
	res, _, _, err := r.ResolveArgsWithContext(
		[]ArgDefinition{{Name: "what", Type: ArgInventory}},
		[]string{"2.sword"},
		ctx,
	)
	if err != nil {
		t.Fatalf("ResolveArgsWithContext: %v", err)
	}
	if got := res["what"].(ItemRef).ID; got != "sword-2" {
		t.Errorf("ordinal 2.sword = %q, want sword-2", got)
	}
}

func TestResolve_Inventory_OrdinalOutOfRange_NotFound(t *testing.T) {
	r := resolveDefault()
	ctx := ResolveContext{
		Inventory: []ItemCandidate{
			&fakeItem{id: "sword-1", name: "a long sword", keywords: []string{"sword"}},
		},
	}
	_, _, _, err := r.ResolveArgsWithContext(
		[]ArgDefinition{{Name: "what", Type: ArgInventory}},
		[]string{"5.sword"},
		ctx,
	)
	if !errors.Is(err, ErrItemNotInInventory) {
		t.Errorf("err = %v, want ErrItemNotInInventory (ordinal out of range)", err)
	}
}

func TestResolve_Entity_OrdinalSelectsNth(t *testing.T) {
	r := resolveDefault()
	ctx := ResolveContext{
		RoomEntities: []EntityCandidate{
			&fakeEntity{id: "rat-1", name: "a grey rat", keywords: []string{"rat"}, kind: "mob"},
			&fakeEntity{id: "rat-2", name: "a brown rat", keywords: []string{"rat"}, kind: "mob"},
		},
	}
	res, _, _, err := r.ResolveArgsWithContext(
		[]ArgDefinition{{Name: "t", Type: ArgEntity}},
		[]string{"2.rat"},
		ctx,
	)
	if err != nil {
		t.Fatalf("ResolveArgsWithContext: %v", err)
	}
	if got := res["t"].(EntityRef).ID; got != "rat-2" {
		t.Errorf("ordinal 2.rat = %q, want rat-2", got)
	}
}

// --- bulk (§5.5 / §5.6) ---

func TestResolve_InventoryBulk_All_ReturnsArray(t *testing.T) {
	r := resolveDefault()
	ctx := ResolveContext{
		Inventory: []ItemCandidate{
			&fakeItem{id: "it-1", name: "a torch", keywords: []string{"torch"}},
			&fakeItem{id: "it-2", name: "a coin", keywords: []string{"coin"}},
		},
	}
	res, _, _, err := r.ResolveArgsWithContext(
		[]ArgDefinition{{Name: "items", Type: ArgInventory, Bulk: true}},
		[]string{"all"},
		ctx,
	)
	if err != nil {
		t.Fatalf("ResolveArgsWithContext: %v", err)
	}
	refs, ok := res["items"].([]ItemRef)
	if !ok {
		t.Fatalf("items type = %T, want []ItemRef", res["items"])
	}
	if len(refs) != 2 {
		t.Errorf("len(refs) = %d, want 2", len(refs))
	}
}

func TestResolve_InventoryBulk_AllKeyword_FiltersMatches(t *testing.T) {
	r := resolveDefault()
	ctx := ResolveContext{
		Inventory: []ItemCandidate{
			&fakeItem{id: "coin-1", name: "a gold coin", keywords: []string{"coin"}},
			&fakeItem{id: "coin-2", name: "a silver coin", keywords: []string{"coin"}},
			&fakeItem{id: "torch", name: "a torch", keywords: []string{"torch"}},
		},
	}
	res, _, _, err := r.ResolveArgsWithContext(
		[]ArgDefinition{{Name: "items", Type: ArgInventory, Bulk: true}},
		[]string{"all.coin"},
		ctx,
	)
	if err != nil {
		t.Fatalf("ResolveArgsWithContext: %v", err)
	}
	refs := res["items"].([]ItemRef)
	if len(refs) != 2 {
		t.Fatalf("len(refs) = %d, want 2 coins", len(refs))
	}
	for _, ref := range refs {
		if ref.Keyword != "coin" {
			t.Errorf("ref %+v not a coin", ref)
		}
	}
}

func TestResolve_InventoryBulk_BareKeyword_ReturnsSingleRef(t *testing.T) {
	// A bulk-capable arg given a bare keyword (not `all`) still
	// resolves to a single ItemRef, NOT an array — the bulk variant
	// is specifically the `all` form (§5.5).
	r := resolveDefault()
	ctx := ResolveContext{
		Inventory: []ItemCandidate{
			&fakeItem{id: "it-1", name: "a torch", keywords: []string{"torch"}},
		},
	}
	res, _, _, err := r.ResolveArgsWithContext(
		[]ArgDefinition{{Name: "items", Type: ArgInventory, Bulk: true}},
		[]string{"torch"},
		ctx,
	)
	if err != nil {
		t.Fatalf("ResolveArgsWithContext: %v", err)
	}
	if _, ok := res["items"].(ItemRef); !ok {
		t.Errorf("items type = %T, want ItemRef (bare keyword, not bulk)", res["items"])
	}
}

func TestResolve_InventoryBulk_AllEmpty_NotFound(t *testing.T) {
	r := resolveDefault()
	_, _, _, err := r.ResolveArgsWithContext(
		[]ArgDefinition{{Name: "items", Type: ArgInventory, Bulk: true}},
		[]string{"all"},
		ResolveContext{},
	)
	if !errors.Is(err, ErrItemNotInInventory) {
		t.Errorf("err = %v, want ErrItemNotInInventory (empty bulk)", err)
	}
}

func TestResolve_Inventory_NonBulkArg_IgnoresAll(t *testing.T) {
	// Without Bulk:true, the token `all` is just a literal keyword;
	// no item has the keyword "all" so it fails to resolve.
	r := resolveDefault()
	ctx := ResolveContext{
		Inventory: []ItemCandidate{
			&fakeItem{id: "it-1", name: "a torch", keywords: []string{"torch"}},
		},
	}
	_, _, _, err := r.ResolveArgsWithContext(
		[]ArgDefinition{{Name: "what", Type: ArgInventory}}, // no Bulk
		[]string{"all"},
		ctx,
	)
	if !errors.Is(err, ErrItemNotInInventory) {
		t.Errorf("err = %v, want ErrItemNotInInventory (non-bulk ignores `all`)", err)
	}
}

func TestResolve_RoomItemBulk_All_ReturnsArray(t *testing.T) {
	r := resolveDefault()
	ctx := ResolveContext{
		RoomItems: []ItemCandidate{
			&fakeItem{id: "r-1", name: "a rock", keywords: []string{"rock"}},
		},
	}
	res, _, _, err := r.ResolveArgsWithContext(
		[]ArgDefinition{{Name: "items", Type: ArgRoomItem, Bulk: true}},
		[]string{"all"},
		ctx,
	)
	if err != nil {
		t.Fatalf("ResolveArgsWithContext: %v", err)
	}
	if refs, ok := res["items"].([]ItemRef); !ok || len(refs) != 1 {
		t.Errorf("items = %T %+v, want []ItemRef len 1", res["items"], res["items"])
	}
}

// --- door (§5.6) ---

// fakeDoorScope implements DoorScope for resolver tests.
type fakeDoorScope struct {
	ref       DoorRef
	ok        bool
	ambiguous bool
	gotArg    string
}

func (f *fakeDoorScope) ResolveDoor(arg string) (DoorRef, bool, bool) {
	f.gotArg = arg
	return f.ref, f.ok, f.ambiguous
}

func TestResolve_Door_UniqueMatch(t *testing.T) {
	r := resolveDefault()
	scope := &fakeDoorScope{
		ref: DoorRef{
			Direction: "n",
			Door:      DoorInfo{Name: "iron gate", Closed: true, Locked: true, KeyID: "key.iron"},
		},
		ok: true,
	}
	res, _, _, err := r.ResolveArgsWithContext(
		[]ArgDefinition{{Name: "d", Type: ArgDoor}},
		[]string{"gate"},
		ResolveContext{Doors: scope},
	)
	if err != nil {
		t.Fatalf("ResolveArgsWithContext: %v", err)
	}
	ref := res["d"].(DoorRef)
	if ref.Direction != "n" || !ref.Door.Locked || ref.Door.KeyID != "key.iron" {
		t.Errorf("ref = %+v", ref)
	}
	if scope.gotArg != "gate" {
		t.Errorf("scope received %q, want gate", scope.gotArg)
	}
}

func TestResolve_Door_Ambiguous_Errors(t *testing.T) {
	r := resolveDefault()
	scope := &fakeDoorScope{ambiguous: true}
	_, _, _, err := r.ResolveArgsWithContext(
		[]ArgDefinition{{Name: "d", Type: ArgDoor}},
		[]string{"door"},
		ResolveContext{Doors: scope},
	)
	if !errors.Is(err, ErrDoorAmbiguous) {
		t.Errorf("err = %v, want ErrDoorAmbiguous", err)
	}
}

func TestResolve_Door_NoMatch_Errors(t *testing.T) {
	r := resolveDefault()
	scope := &fakeDoorScope{} // ok=false, ambiguous=false
	_, _, _, err := r.ResolveArgsWithContext(
		[]ArgDefinition{{Name: "d", Type: ArgDoor}},
		[]string{"nothing"},
		ResolveContext{Doors: scope},
	)
	if !errors.Is(err, ErrNoSuchDoor) {
		t.Errorf("err = %v, want ErrNoSuchDoor", err)
	}
}

func TestResolve_Door_NilScope_Errors(t *testing.T) {
	r := resolveDefault()
	_, _, _, err := r.ResolveArgsWithContext(
		[]ArgDefinition{{Name: "d", Type: ArgDoor}},
		[]string{"gate"},
		ResolveContext{}, // Doors nil
	)
	if !errors.Is(err, ErrNoSuchDoor) {
		t.Errorf("err = %v, want ErrNoSuchDoor (nil scope)", err)
	}
}
