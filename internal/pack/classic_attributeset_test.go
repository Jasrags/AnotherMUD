package pack

import (
	"path/filepath"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/progression"
)

// The core pack's `classic` attribute set is the REGRESSION GATE for SR-M1
// (shadowrun-mvp.md Appendix A step 2): it must reproduce the engine's old
// hardcode exactly, so making the character seed / score sheet / train gate
// read from a content-declared set is a no-op for the existing worlds. If this
// drifts, WoT/starter-world characters would seed or train differently — the
// whole point of "content-declared, zero behavior change" would be violated.
func TestCorePack_ClassicSetMatchesEngineDefaults(t *testing.T) {
	content := repoContentDir(t)
	matches, err := filepath.Glob(filepath.Join(content, "core", "attributes", "*.yaml"))
	if err != nil {
		t.Fatalf("glob core attributes: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("core/attributes/*.yaml resolves to 0 files — the classic set is unauthored")
	}

	// Decode + Register through the real loader path (tests the content too).
	reg := progression.NewAttributeSetRegistry()
	for _, m := range matches {
		s, err := decodeAttributeSet(m, "tapestry-core")
		if err != nil {
			t.Fatalf("decode %s: %v", m, err)
		}
		if err := reg.Register(s); err != nil {
			t.Fatalf("register %s: %v", m, err)
		}
	}
	classic, ok := reg.Get("classic")
	if !ok {
		t.Fatal("core declares no `classic` attribute set")
	}

	// The six classic attribute keys DefaultPlayerBase seeds (excluding the
	// engine-vital keys hp_max/movement_max/hit_mod/ac, which are not attributes).
	sixKeys := []progression.StatType{
		progression.StatSTR, progression.StatINT, progression.StatWIS,
		progression.StatDEX, progression.StatCON, progression.StatLUCK,
	}

	// classic must be EXACTLY the six — no more, no fewer.
	if len(classic.Attributes) != len(sixKeys) {
		t.Fatalf("classic has %d attributes, want exactly %d (the classic six)", len(classic.Attributes), len(sixKeys))
	}

	// Defaults match DefaultPlayerBase for each of the six.
	engineBase := progression.DefaultPlayerBase()
	defs := classic.Defaults()
	for _, k := range sixKeys {
		if defs[k] != engineBase[k] {
			t.Errorf("classic default %s = %d, want %d (DefaultPlayerBase)", k, defs[k], engineBase[k])
		}
	}

	// Trainable set matches DefaultTrainingConfig exactly (same keys, both ways).
	tc := progression.DefaultTrainingConfig()
	tr := classic.TrainableSet()
	if len(tr) != len(tc.Trainable) {
		t.Errorf("classic trainable count = %d, want %d (DefaultTrainingConfig)", len(tr), len(tc.Trainable))
	}
	for k := range tc.Trainable {
		if !tr[k] {
			t.Errorf("classic set: %s not trainable, but DefaultTrainingConfig trains it", k)
		}
	}
	for k := range tr {
		if !tc.Trainable[k] {
			t.Errorf("classic set: %s trainable, but DefaultTrainingConfig does not train it", k)
		}
	}

	// Set-level cap equals the engine default race cap for every attribute.
	for _, a := range classic.Attributes {
		if a.Cap != tc.DefaultRaceCap {
			t.Errorf("classic %s cap = %d, want %d (DefaultTrainingConfig.DefaultRaceCap)", a.ID, a.Cap, tc.DefaultRaceCap)
		}
	}
}
