package session

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/progression"
)

// seedBaseFor is the world-aware seed resolver (SR-M1 step 3). These tests
// cover the resolution + every fallback without building a live connActor.
func TestSeedBaseFor(t *testing.T) {
	reg := progression.NewAttributeSetRegistry()
	// A stand-in classic set for resolution/fallback wiring only — the real
	// contract that classic == the engine hardcode is
	// TestCorePack_ClassicSetMatchesEngineDefaults (which loads classic.yaml).
	if err := reg.Register(&progression.AttributeSet{
		ID: progression.ClassicAttributeSetID,
		Attributes: []progression.Attribute{
			{ID: "str", Default: 10}, {ID: "int", Default: 10}, {ID: "wis", Default: 10},
			{ID: "dex", Default: 10}, {ID: "con", Default: 10}, {ID: "luck", Default: 10},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := reg.Register(&progression.AttributeSet{
		ID:         "shadowrun5",
		Attributes: []progression.Attribute{{ID: "body", Default: 3}, {ID: "agility", Default: 4}},
	}); err != nil {
		t.Fatal(err)
	}
	selection := map[string]string{"shadowrun": "shadowrun5"} // wot/starter-world select nothing

	t.Run("world selecting a set uses it", func(t *testing.T) {
		got := seedBaseFor(reg, selection, "shadowrun")
		if got["body"] != 3 || got["agility"] != 4 {
			t.Errorf("shadowrun seed = %v, want body/agility from shadowrun5", got)
		}
		if _, ok := got[progression.StatSTR]; ok {
			t.Error("shadowrun seed leaked the classic 'str' key")
		}
	})

	t.Run("world selecting nothing falls back to classic", func(t *testing.T) {
		got := seedBaseFor(reg, selection, "wot") // not in selection map
		if got[progression.StatSTR] != 10 {
			t.Errorf("wot seed = %v, want classic six", got)
		}
		if _, ok := got["body"]; ok {
			t.Error("wot seed leaked a shadowrun key")
		}
	})

	t.Run("empty worldID falls back to classic", func(t *testing.T) {
		got := seedBaseFor(reg, selection, "")
		if got[progression.StatSTR] != 10 {
			t.Errorf("empty-world seed = %v, want classic six", got)
		}
	})

	t.Run("nil registry falls back to DefaultPlayerBase", func(t *testing.T) {
		got := seedBaseFor(nil, selection, "shadowrun")
		if got[progression.StatSTR] != 10 || got[progression.StatHPMax] != 20 {
			t.Errorf("nil-registry seed = %v, want DefaultPlayerBase", got)
		}
	})

	t.Run("registry present but classic unregistered falls through", func(t *testing.T) {
		empty := progression.NewAttributeSetRegistry()
		got := seedBaseFor(empty, nil, "wot") // resolves to classic, which is absent
		if got[progression.StatSTR] != 10 {
			t.Errorf("seed = %v, want DefaultPlayerBase fallback when classic unregistered", got)
		}
	})
}
