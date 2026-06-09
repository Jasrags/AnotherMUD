package biome

import (
	"errors"
	"testing"
)

func TestRegistry_RegisterGetResolve(t *testing.T) {
	r := NewRegistry()
	forest := &Biome{ID: "Forest", DisplayName: "deep forest", Ambience: []string{"birdsong"}}
	if err := r.RegisterEngine(forest); err != nil {
		t.Fatalf("RegisterEngine: %v", err)
	}
	// id is lowercased on register.
	if forest.ID != "forest" {
		t.Errorf("id = %q, want lowercased 'forest'", forest.ID)
	}
	// Get is case-insensitive.
	if b, ok := r.Get("FOREST"); !ok || b != forest {
		t.Errorf("Get(FOREST) = (%v, %v), want the forest biome", b, ok)
	}
	if _, ok := r.Get("swamp"); ok {
		t.Error("Get(swamp) should miss")
	}
}

func TestRegistry_ResolveDefaultAndBackwardCompat(t *testing.T) {
	r := NewRegistry()
	if err := RegisterEngineBaseline(r); err != nil {
		t.Fatalf("baseline: %v", err)
	}
	// Empty terrain → default biome (outdoors).
	if b, ok := r.Resolve(""); !ok || b.ID != DefaultBiomeID {
		t.Errorf("Resolve(\"\") = (%v, %v), want the default outdoors biome", b, ok)
	}
	// Unregistered terrain → (nil, false): the §2.3 bare-string path.
	if b, ok := r.Resolve("forest"); ok {
		t.Errorf("Resolve(forest) = (%v, %v), want miss (unregistered → bare-string behavior)", b, ok)
	}
}

func TestRegistry_PackShadowRejected(t *testing.T) {
	r := NewRegistry()
	if err := RegisterEngineBaseline(r); err != nil {
		t.Fatalf("baseline: %v", err)
	}
	// A pack biome may add a new id...
	if err := r.RegisterPack("tapestry-core", &Biome{ID: "forest"}); err != nil {
		t.Errorf("RegisterPack(forest): %v, want success", err)
	}
	// ...but must NOT shadow an engine biome.
	err := r.RegisterPack("tapestry-core", &Biome{ID: "outdoors"})
	if !errors.Is(err, ErrShadow) {
		t.Errorf("RegisterPack(outdoors) err = %v, want ErrShadow", err)
	}
	// A second forest is a plain duplicate.
	err = r.RegisterPack("other", &Biome{ID: "forest"})
	if !errors.Is(err, ErrDuplicate) {
		t.Errorf("duplicate forest err = %v, want ErrDuplicate", err)
	}
}

func TestRegisterEngineBaseline_Shielding(t *testing.T) {
	r := NewRegistry()
	if err := RegisterEngineBaseline(r); err != nil {
		t.Fatalf("baseline: %v", err)
	}
	if r.Len() != 3 {
		t.Fatalf("baseline registered %d biomes, want 3", r.Len())
	}
	out, _ := r.Get(BaselineOutdoors)
	if out.WeatherShielded || out.TimeShielded {
		t.Error("outdoors must not be shielded")
	}
	for _, id := range []string{BaselineIndoors, BaselineUnderground} {
		b, _ := r.Get(id)
		if !b.WeatherShielded || !b.TimeShielded {
			t.Errorf("%s must be weather- and time-shielded (biomes §3 / PD-2)", id)
		}
	}
}

func TestDecode(t *testing.T) {
	b, err := Decode([]byte(`
id: swamp
name: fetid swamp
weather_shielded: false
time_shielded: false
ambience:
  - "Something plops into the muck."
  - "A cloud of midges drifts past."
forage_table: swamp-forage
node_spawn_table: swamp-nodes
`))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if b.ID != "swamp" || b.DisplayName != "fetid swamp" {
		t.Errorf("decoded = %+v", b)
	}
	if len(b.Ambience) != 2 || b.ForageTable != "swamp-forage" || b.NodeSpawnTable != "swamp-nodes" {
		t.Errorf("decoded fields = %+v", b)
	}
}

func TestDecode_EmptyIDRejected(t *testing.T) {
	if _, err := Decode([]byte(`name: nameless`)); err == nil {
		t.Error("Decode with no id should error")
	}
}
