package entities

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/item"
)

func TestSpawnAssignsFreshIDAndCopiesFields(t *testing.T) {
	s := NewStore()
	tpl := &item.Template{
		ID:       "tapestry-core:short-sword",
		Name:     "a short sword",
		Type:     "item",
		Tags:     []string{"weapon", "metal"},
		Keywords: []string{"sword", "short"},
		Properties: map[string]any{
			"damage": 4,
		},
		Modifiers: []item.Modifier{{Stat: "str", Value: 1}},
	}

	a, err := s.Spawn(tpl)
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	b, err := s.Spawn(tpl)
	if err != nil {
		t.Fatalf("Spawn second: %v", err)
	}

	if a.ID() == b.ID() {
		t.Fatalf("ids collided: %q == %q", a.ID(), b.ID())
	}
	if !strings.HasPrefix(string(a.ID()), "entity-") {
		t.Errorf("id prefix: %q", a.ID())
	}
	if a.Name() != tpl.Name {
		t.Errorf("Name = %q", a.Name())
	}
	if a.Type() != tpl.Type {
		t.Errorf("Type = %q", a.Type())
	}
	if a.TemplateID() != tpl.ID {
		t.Errorf("TemplateID = %q", a.TemplateID())
	}
	if got := a.Properties()[PropTemplateID]; got != string(tpl.ID) {
		t.Errorf("Properties[%s] = %v, want %q", PropTemplateID, got, tpl.ID)
	}
}

func TestSpawnFiltersRoomIDFromProperties(t *testing.T) {
	s := NewStore()
	tpl := &item.Template{
		ID:   "tapestry-core:foo",
		Name: "foo",
		Type: "item",
		Properties: map[string]any{
			"room_id": "tapestry-core:somewhere",
			"keep":    "ok",
		},
	}
	a, err := s.Spawn(tpl)
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if _, present := a.Properties()[PropRoomID]; present {
		t.Errorf("room_id leaked into instance properties: %+v", a.Properties())
	}
	if a.Properties()["keep"] != "ok" {
		t.Errorf("non-reserved property dropped: %+v", a.Properties())
	}
}

func TestSpawnDropsImplicitTypeTag(t *testing.T) {
	// §2.3 step 2: tag matching the entity's own type is implicit.
	s := NewStore()
	tpl := &item.Template{
		ID:   "tapestry-core:foo",
		Name: "foo",
		Type: "item",
		Tags: []string{"item", "weapon"},
	}
	a, err := s.Spawn(tpl)
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if got := a.Tags(); !reflect.DeepEqual(got, []string{"weapon"}) {
		t.Errorf("Tags = %v, want [weapon]", got)
	}
}

func TestSpawnNormalizesNestedUntypedMaps(t *testing.T) {
	// yaml.v3 can produce map[any]any for nested maps in some shapes.
	// §2.3 step 4 requires recursive normalization.
	s := NewStore()
	tpl := &item.Template{
		ID:   "tapestry-core:foo",
		Name: "foo",
		Type: "item",
		Properties: map[string]any{
			"nested": map[any]any{
				"inner": map[any]any{"k": 1},
				42:      "dropped non-string key",
			},
		},
	}
	a, err := s.Spawn(tpl)
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	nested, ok := a.Properties()["nested"].(map[string]any)
	if !ok {
		t.Fatalf("nested not normalized: %T", a.Properties()["nested"])
	}
	inner, ok := nested["inner"].(map[string]any)
	if !ok {
		t.Fatalf("inner not normalized: %T", nested["inner"])
	}
	if inner["k"] != 1 {
		t.Errorf("inner[k] = %v", inner["k"])
	}
	if _, present := nested["42"]; present {
		t.Errorf("non-string key was promoted to string: %+v", nested)
	}
}

func TestSpawnModifiersTaggedByEntityID(t *testing.T) {
	s := NewStore()
	tpl := &item.Template{
		ID:        "tapestry-core:sword",
		Name:      "sword",
		Type:      "item",
		Modifiers: []item.Modifier{{Stat: "str", Value: 2}, {Stat: "dex", Value: 1}},
	}
	a, _ := s.Spawn(tpl)
	b, _ := s.Spawn(tpl)

	for _, m := range a.Modifiers() {
		want := SourceKey("entity:" + string(a.ID()))
		if m.Source != want {
			t.Errorf("a modifier source = %q, want %q", m.Source, want)
		}
	}
	if a.Modifiers()[0].Source == b.Modifiers()[0].Source {
		t.Errorf("two instances share a source key: %q", a.Modifiers()[0].Source)
	}
}

func TestSpawnNilTemplateReturnsError(t *testing.T) {
	s := NewStore()
	_, err := s.Spawn(nil)
	if !errors.Is(err, ErrUnknownTemplate) {
		t.Errorf("err = %v, want ErrUnknownTemplate", err)
	}
}
