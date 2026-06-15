package command

import (
	"errors"
	"strings"
	"testing"
)

// fakeItem implements ItemCandidate (and conditionally
// ContainerCandidate) for resolver tests.
type fakeItem struct {
	id         string
	name       string
	keywords   []string
	templateID string
	container  bool
}

func (f *fakeItem) Name() string       { return f.name }
func (f *fakeItem) Keywords() []string { return f.keywords }
func (f *fakeItem) EntityID() string   { return f.id }
func (f *fakeItem) TemplateID() string { return f.templateID }
func (f *fakeItem) IsContainer() bool  { return f.container }

// fakeEntity implements EntityCandidate.
type fakeEntity struct {
	id       string
	name     string
	keywords []string
	kind     string // "player" or "mob"
}

func (f *fakeEntity) Name() string       { return f.name }
func (f *fakeEntity) Keywords() []string { return f.keywords }
func (f *fakeEntity) EntityID() string   { return f.id }
func (f *fakeEntity) EntityType() string { return f.kind }

// --- inventory ---

func TestResolve_Inventory_KeywordMatch(t *testing.T) {
	r := resolveDefault()
	ctx := ResolveContext{
		Inventory: []ItemCandidate{
			&fakeItem{id: "it-1", name: "a healing potion", keywords: []string{"potion", "healing"}, templateID: "potion.healing"},
		},
	}
	res, _, _, err := r.ResolveArgsWithContext(
		[]ArgDefinition{{Name: "what", Type: ArgInventory}},
		[]string{"potion"},
		ctx,
	)
	if err != nil {
		t.Fatalf("ResolveArgsWithContext: %v", err)
	}
	ref, ok := res["what"].(ItemRef)
	if !ok {
		t.Fatalf("what type = %T, want ItemRef", res["what"])
	}
	if ref.ID != "it-1" || ref.Name != "a healing potion" || ref.TemplateID != "potion.healing" {
		t.Errorf("ref = %+v", ref)
	}
	if ref.Keyword != "potion" {
		t.Errorf("Keyword = %q, want potion (first of Keywords())", ref.Keyword)
	}
}

func TestResolve_Inventory_NoMatch_Errors(t *testing.T) {
	r := resolveDefault()
	ctx := ResolveContext{
		Inventory: []ItemCandidate{
			&fakeItem{id: "it-1", name: "a sword", keywords: []string{"sword"}},
		},
	}
	_, _, _, err := r.ResolveArgsWithContext(
		[]ArgDefinition{{Name: "what", Type: ArgInventory}},
		[]string{"nonesuch"},
		ctx,
	)
	if err == nil {
		t.Fatal("expected ItemNotInInventory")
	}
	if !errors.Is(err, ErrItemNotInInventory) {
		t.Errorf("err = %v, want ErrItemNotInInventory", err)
	}
}

func TestResolve_Inventory_EmptyInventory_Errors(t *testing.T) {
	r := resolveDefault()
	_, _, _, err := r.ResolveArgsWithContext(
		[]ArgDefinition{{Name: "what", Type: ArgInventory}},
		[]string{"anything"},
		ResolveContext{},
	)
	if !errors.Is(err, ErrItemNotInInventory) {
		t.Errorf("err = %v, want ErrItemNotInInventory", err)
	}
}

// --- room_item ---

func TestResolve_RoomItem_KeywordMatch(t *testing.T) {
	r := resolveDefault()
	ctx := ResolveContext{
		RoomItems: []ItemCandidate{
			&fakeItem{id: "rit-1", name: "a stone bench", keywords: []string{"bench"}, templateID: "bench"},
		},
	}
	res, _, _, err := r.ResolveArgsWithContext(
		[]ArgDefinition{{Name: "fixture", Type: ArgRoomItem}},
		[]string{"bench"},
		ctx,
	)
	if err != nil {
		t.Fatalf("ResolveArgsWithContext: %v", err)
	}
	if res["fixture"].(ItemRef).ID != "rit-1" {
		t.Errorf("got %+v", res["fixture"])
	}
}

// --- entity / player / npc ---

func TestResolve_Entity_MatchesPlayerOrMob(t *testing.T) {
	r := resolveDefault()
	ctx := ResolveContext{
		RoomEntities: []EntityCandidate{
			&fakeEntity{id: "p-1", name: "Alice", keywords: []string{"alice"}, kind: "player"},
			&fakeEntity{id: "m-1", name: "a rat", keywords: []string{"rat"}, kind: "mob"},
		},
	}
	res, _, _, err := r.ResolveArgsWithContext(
		[]ArgDefinition{{Name: "target", Type: ArgEntity}},
		[]string{"alice"},
		ctx,
	)
	if err != nil {
		t.Fatalf("ResolveArgsWithContext: %v", err)
	}
	ref := res["target"].(EntityRef)
	if ref.ID != "p-1" || ref.Type != "player" {
		t.Errorf("got %+v", ref)
	}
}

func TestResolve_Player_FiltersOutMobs(t *testing.T) {
	r := resolveDefault()
	ctx := ResolveContext{
		RoomEntities: []EntityCandidate{
			&fakeEntity{id: "m-1", name: "a rat", keywords: []string{"rat"}, kind: "mob"},
		},
	}
	_, _, _, err := r.ResolveArgsWithContext(
		[]ArgDefinition{{Name: "p", Type: ArgPlayer}},
		[]string{"rat"},
		ctx,
	)
	if !errors.Is(err, ErrPlayerNotInRoom) {
		t.Errorf("err = %v, want ErrPlayerNotInRoom (mob excluded)", err)
	}
}

func TestResolve_NPC_FiltersOutPlayers(t *testing.T) {
	r := resolveDefault()
	ctx := ResolveContext{
		RoomEntities: []EntityCandidate{
			&fakeEntity{id: "p-1", name: "Alice", keywords: []string{"alice"}, kind: "player"},
		},
	}
	_, _, _, err := r.ResolveArgsWithContext(
		[]ArgDefinition{{Name: "n", Type: ArgNPC}},
		[]string{"alice"},
		ctx,
	)
	if !errors.Is(err, ErrNpcNotInRoom) {
		t.Errorf("err = %v, want ErrNpcNotInRoom (player excluded)", err)
	}
}

// --- container ---

func TestResolve_Container_InventoryFirstThenRoom(t *testing.T) {
	r := resolveDefault()
	invBox := &fakeItem{id: "inv-box", name: "a small box", keywords: []string{"box"}, container: true}
	roomChest := &fakeItem{id: "room-chest", name: "a wooden chest", keywords: []string{"chest"}, container: true}
	ctx := ResolveContext{
		Inventory: []ItemCandidate{invBox},
		RoomItems: []ItemCandidate{roomChest},
	}
	// `box` matches inventory; `chest` matches room.
	res, _, _, err := r.ResolveArgsWithContext(
		[]ArgDefinition{{Name: "c", Type: ArgContainer}},
		[]string{"box"},
		ctx,
	)
	if err != nil {
		t.Fatalf("box ResolveArgsWithContext: %v", err)
	}
	if res["c"].(ItemRef).ID != "inv-box" {
		t.Errorf("got %+v, want inv-box", res["c"])
	}
	res, _, _, err = r.ResolveArgsWithContext(
		[]ArgDefinition{{Name: "c", Type: ArgContainer}},
		[]string{"chest"},
		ctx,
	)
	if err != nil {
		t.Fatalf("chest ResolveArgsWithContext: %v", err)
	}
	if res["c"].(ItemRef).ID != "room-chest" {
		t.Errorf("got %+v, want room-chest", res["c"])
	}
}

func TestResolve_Container_NonContainerItemsExcluded(t *testing.T) {
	r := resolveDefault()
	// `box` is a non-container; only chest qualifies.
	ctx := ResolveContext{
		Inventory: []ItemCandidate{
			&fakeItem{id: "inv-box", name: "a small box", keywords: []string{"box"}, container: false},
		},
	}
	_, _, _, err := r.ResolveArgsWithContext(
		[]ArgDefinition{{Name: "c", Type: ArgContainer}},
		[]string{"box"},
		ctx,
	)
	if !errors.Is(err, ErrContainerNotFound) {
		t.Errorf("err = %v, want ErrContainerNotFound", err)
	}
}

// --- visible ---

func TestResolve_Visible_SelfMatchTaggedSelf(t *testing.T) {
	r := resolveDefault()
	ctx := ResolveContext{
		ActorName: "Alice",
		ActorID:   "p-self",
	}
	res, _, _, err := r.ResolveArgsWithContext(
		[]ArgDefinition{{Name: "t", Type: ArgVisible}},
		[]string{"alice"},
		ctx,
	)
	if err != nil {
		t.Fatalf("ResolveArgsWithContext: %v", err)
	}
	v := res["t"].(VisibleRef)
	if v.Source != "self" || v.ID != "p-self" {
		t.Errorf("got %+v, want self / p-self", v)
	}
}

func TestResolve_Visible_InventoryItemTaggedInventory(t *testing.T) {
	r := resolveDefault()
	ctx := ResolveContext{
		Inventory: []ItemCandidate{
			&fakeItem{id: "it-1", name: "a torch", keywords: []string{"torch"}},
		},
	}
	res, _, _, err := r.ResolveArgsWithContext(
		[]ArgDefinition{{Name: "t", Type: ArgVisible}},
		[]string{"torch"},
		ctx,
	)
	if err != nil {
		t.Fatalf("ResolveArgsWithContext: %v", err)
	}
	v := res["t"].(VisibleRef)
	if v.Source != "inventory" || v.Type != "item" {
		t.Errorf("got %+v, want source=inventory type=item", v)
	}
}

func TestResolve_Visible_RoomEntityTaggedRoom(t *testing.T) {
	r := resolveDefault()
	ctx := ResolveContext{
		RoomEntities: []EntityCandidate{
			&fakeEntity{id: "m-1", name: "a rat", keywords: []string{"rat"}, kind: "mob"},
		},
	}
	res, _, _, err := r.ResolveArgsWithContext(
		[]ArgDefinition{{Name: "t", Type: ArgVisible}},
		[]string{"rat"},
		ctx,
	)
	if err != nil {
		t.Fatalf("ResolveArgsWithContext: %v", err)
	}
	v := res["t"].(VisibleRef)
	if v.Source != "room" || v.Type != "mob" {
		t.Errorf("got %+v", v)
	}
}

func TestResolve_Visible_NoMatch_Errors(t *testing.T) {
	r := resolveDefault()
	_, _, _, err := r.ResolveArgsWithContext(
		[]ArgDefinition{{Name: "t", Type: ArgVisible}},
		[]string{"ghost"},
		ResolveContext{},
	)
	if !errors.Is(err, ErrNotVisible) {
		t.Errorf("err = %v, want ErrNotVisible", err)
	}
}

// --- findable ---

func TestResolve_Findable_InventoryFirstThenRoom(t *testing.T) {
	r := resolveDefault()
	ctx := ResolveContext{
		Inventory: []ItemCandidate{
			&fakeItem{id: "inv-key", name: "an iron key", keywords: []string{"key"}, templateID: "key.iron"},
		},
		RoomItems: []ItemCandidate{
			&fakeItem{id: "room-key", name: "a rusty key", keywords: []string{"key"}, templateID: "key.rusty"},
		},
	}
	res, _, _, err := r.ResolveArgsWithContext(
		[]ArgDefinition{{Name: "f", Type: ArgFindable}},
		[]string{"key"},
		ctx,
	)
	if err != nil {
		t.Fatalf("ResolveArgsWithContext: %v", err)
	}
	// Inventory wins on equal keyword match.
	if res["f"].(ItemRef).ID != "inv-key" {
		t.Errorf("got %+v, want inv-key", res["f"])
	}
}

func TestResolve_Findable_OnlyRoom(t *testing.T) {
	r := resolveDefault()
	ctx := ResolveContext{
		RoomItems: []ItemCandidate{
			&fakeItem{id: "room-rock", name: "a stone", keywords: []string{"stone"}},
		},
	}
	res, _, _, err := r.ResolveArgsWithContext(
		[]ArgDefinition{{Name: "f", Type: ArgFindable}},
		[]string{"stone"},
		ctx,
	)
	if err != nil {
		t.Fatalf("ResolveArgsWithContext: %v", err)
	}
	if res["f"].(ItemRef).ID != "room-rock" {
		t.Errorf("got %+v", res["f"])
	}
}

// --- error message smoke tests ---

func TestResolve_ItemNotFoundMessages_AreUserReadable(t *testing.T) {
	cases := []struct {
		t    ArgType
		want string
	}{
		{ArgInventory, "carrying"},
		{ArgRoomItem, "see"},
		{ArgEntity, "see"},
		{ArgPlayer, "player"},
		{ArgNPC, "mob"},
		{ArgContainer, "container"},
		{ArgVisible, "see"},
		{ArgFindable, "see"},
	}
	r := resolveDefault()
	for _, c := range cases {
		_, _, _, err := r.ResolveArgsWithContext(
			[]ArgDefinition{{Name: "x", Type: c.t}},
			[]string{"absent"},
			ResolveContext{},
		)
		if err == nil {
			t.Errorf("%s expected not-found error", c.t)
			continue
		}
		if !strings.Contains(strings.ToLower(err.Error()), c.want) {
			t.Errorf("%s err = %q, want substring %q", c.t, err.Error(), c.want)
		}
	}
}
