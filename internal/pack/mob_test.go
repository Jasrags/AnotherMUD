package pack

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/mob"
)

// mobPack writes a minimal pack carrying one mob YAML. Content is
// substituted in so tests can vary required/optional fields. The
// pack manifest declares `areas` and `mobs` globs but intentionally
// omits `rooms`: resolveGlobs errors when a declared pattern matches
// zero files, so adding `rooms: []` here would only work if the
// glob expander accepts an empty list AND no files match — easier
// to keep the key absent. If a future test wants to combine rooms
// with mob loading, declare `rooms` AND write at least one room file.
func mobPack(t *testing.T, mobBody string) string {
	t.Helper()
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  areas: [areas/*.yaml]
  mobs: [mobs/*.yaml]
`)
	writeFile(t, filepath.Join(pack, "areas/town.yaml"), "id: town\nname: Town\n")
	writeFile(t, filepath.Join(pack, "mobs/guard.yaml"), mobBody)
	return root
}

func TestLoad_DecodesMobTemplate(t *testing.T) {
	body := `
id: village-guard
name: a village guard
behavior: stationary
disposition: 0
tags: [humanoid, guard]
keywords: [guard, villager]
properties:
  patrol_speed: 2
stats:
  str: 12
  hp_max: 40
equipment:
  - tapestry-core:short-sword
`
	root := mobPack(t, body)
	regs := NewRegistries()
	if err := Load(context.Background(), root, nil, regs, nil, nil, nil); err != nil {
		t.Fatalf("Load: %v", err)
	}
	got, err := regs.Mobs.Get("tapestry-core:village-guard")
	if err != nil {
		t.Fatalf("Mobs.Get: %v", err)
	}
	if got.Name != "a village guard" {
		t.Errorf("Name = %q", got.Name)
	}
	if got.Type != defaultMobType {
		t.Errorf("Type = %q, want default %q", got.Type, defaultMobType)
	}
	if got.Behavior != "stationary" {
		t.Errorf("Behavior = %q", got.Behavior)
	}
	if got.Stats["hp_max"] != 40 {
		t.Errorf("Stats[hp_max] = %d", got.Stats["hp_max"])
	}
	if len(got.Equipment) != 1 || got.Equipment[0] != "tapestry-core:short-sword" {
		t.Errorf("Equipment = %v", got.Equipment)
	}
}

func TestLoad_MobMissingBehavior(t *testing.T) {
	root := mobPack(t, `
id: bad
name: a thing
`)
	err := Load(context.Background(), root, nil, NewRegistries(), nil, nil, nil)
	if !errors.Is(err, ErrInvalidContent) {
		t.Fatalf("err = %v, want ErrInvalidContent", err)
	}
}

func TestLoad_MobMissingID(t *testing.T) {
	root := mobPack(t, `
name: nameless
behavior: idle
`)
	err := Load(context.Background(), root, nil, NewRegistries(), nil, nil, nil)
	if !errors.Is(err, ErrInvalidContent) {
		t.Fatalf("err = %v, want ErrInvalidContent", err)
	}
}

func TestLoad_MobTypeDefaultsToNpc(t *testing.T) {
	root := mobPack(t, `
id: guard
name: a guard
behavior: idle
`)
	regs := NewRegistries()
	if err := Load(context.Background(), root, nil, regs, nil, nil, nil); err != nil {
		t.Fatalf("Load: %v", err)
	}
	got, _ := regs.Mobs.Get("tapestry-core:guard")
	if got.Type != "npc" {
		t.Errorf("Type = %q, want npc (spec default)", got.Type)
	}
}

func TestLoad_MobTypeExplicit(t *testing.T) {
	root := mobPack(t, `
id: dragon
name: an old dragon
type: monster
behavior: territorial
`)
	regs := NewRegistries()
	if err := Load(context.Background(), root, nil, regs, nil, nil, nil); err != nil {
		t.Fatalf("Load: %v", err)
	}
	got, _ := regs.Mobs.Get("tapestry-core:dragon")
	if got.Type != "monster" {
		t.Errorf("Type = %q, want monster", got.Type)
	}
}

// TestLoad_MobDuplicateExplicitQualifiedID covers two packs both
// trying to claim the same explicitly-qualified id ("shared:dup").
// Mirrors the existing TestLoadCrossPackDuplicateIDs / item test:
// the second pack to register must surface ErrDuplicateID rather
// than silently overwrite.
func TestLoad_MobDuplicateExplicitQualifiedID(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"a", "b"} {
		pack := filepath.Join(root, name)
		writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: `+name+`
content:
  mobs: [mobs/*.yaml]
`)
		writeFile(t, filepath.Join(pack, "mobs/dup.yaml"), `
id: shared:dup
name: dup
behavior: idle
`)
	}
	err := Load(context.Background(), root, nil, NewRegistries(), nil, nil, nil)
	if !errors.Is(err, mob.ErrDuplicateID) {
		t.Fatalf("err = %v, want mob.ErrDuplicateID", err)
	}
}

// TestLoad_MobBareIDsInDifferentPacksDoNotCollide pins the namespace
// isolation invariant: two packs each declaring `id: guard` (bare)
// must register as distinct namespace-qualified ids and BOTH succeed.
// Without this, the duplicate-id test above would falsely also pass
// against a buggy loader that ignored the namespace.
func TestLoad_MobBareIDsInDifferentPacksDoNotCollide(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"a", "b"} {
		pack := filepath.Join(root, name)
		writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: `+name+`
content:
  mobs: [mobs/*.yaml]
`)
		writeFile(t, filepath.Join(pack, "mobs/guard.yaml"), `
id: guard
name: a guard
behavior: idle
`)
	}
	regs := NewRegistries()
	if err := Load(context.Background(), root, nil, regs, nil, nil, nil); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !regs.Mobs.Has("a:guard") {
		t.Errorf("missing a:guard after namespace qualification")
	}
	if !regs.Mobs.Has("b:guard") {
		t.Errorf("missing b:guard after namespace qualification")
	}
	if got := regs.Mobs.Count(); got != 2 {
		t.Errorf("Mobs.Count = %d, want 2 (distinct ns-qualified ids)", got)
	}
}

func TestLoad_EquipmentNotValidatedAtLoad(t *testing.T) {
	// Spec §3.1: missing equipment templates fail silently at spawn
	// time. The loader MUST NOT reject a mob that names an item
	// template the pack doesn't (yet) carry.
	root := mobPack(t, `
id: phantom
name: a phantom
behavior: idle
equipment:
  - tapestry-core:nonexistent-blade
`)
	regs := NewRegistries()
	if err := Load(context.Background(), root, nil, regs, nil, nil, nil); err != nil {
		t.Fatalf("Load: %v (equipment validity is spawn-time, not load-time)", err)
	}
	got, _ := regs.Mobs.Get("tapestry-core:phantom")
	if len(got.Equipment) != 1 {
		t.Errorf("Equipment lost: %v", got.Equipment)
	}
}

// TestLoad_NilRegistryFieldRejected pins the contract that all five
// registry fields (now including Mobs) must be non-nil.
func TestLoad_NilMobsRegistryRejected(t *testing.T) {
	regs := NewRegistries()
	regs.Mobs = nil
	err := Load(context.Background(), t.TempDir(), nil, regs, nil, nil, nil)
	if err == nil {
		t.Fatal("Load with nil Mobs registry = nil, want error")
	}
}

func TestLoad_DecodesDispositionRules(t *testing.T) {
	body := `
id: guard
name: a guard
behavior: stationary
base_disposition: friendly
disposition_rules:
  default: friendly
  rules:
    - has_tag: outlaw
      reaction: hostile
    - min_alignment: -100
      max_alignment: -50
      reaction: hostile
    - reaction: wary
`
	root := mobPack(t, body)
	regs := NewRegistries()
	if err := Load(context.Background(), root, nil, regs, nil, nil, nil); err != nil {
		t.Fatalf("Load: %v", err)
	}
	got, err := regs.Mobs.Get("tapestry-core:guard")
	if err != nil {
		t.Fatalf("Mobs.Get: %v", err)
	}
	if got.BaseDisposition != mob.ReactionFriendly {
		t.Errorf("BaseDisposition = %q, want %q", got.BaseDisposition, mob.ReactionFriendly)
	}
	if got.DispositionRules == nil {
		t.Fatal("DispositionRules = nil")
	}
	if got.DispositionRules.Default != mob.ReactionFriendly {
		t.Errorf("Default = %q", got.DispositionRules.Default)
	}
	if n := len(got.DispositionRules.Rules); n != 3 {
		t.Fatalf("rules = %d, want 3", n)
	}
	r0 := got.DispositionRules.Rules[0]
	if r0.HasTag != "outlaw" || r0.Reaction != mob.ReactionHostile {
		t.Errorf("rule[0] = %+v", r0)
	}
	r1 := got.DispositionRules.Rules[1]
	if !r1.HasMinAlignment || r1.MinAlignment != -100 || !r1.HasMaxAlignment || r1.MaxAlignment != -50 {
		t.Errorf("rule[1] alignment bounds = %+v", r1)
	}
	r2 := got.DispositionRules.Rules[2]
	if r2.HasConditions() {
		t.Errorf("rule[2] should be unconditional (fallback): %+v", r2)
	}
}

func TestLoad_DispositionRuleMissingReactionIsRejected(t *testing.T) {
	body := `
id: guard
name: a guard
behavior: stationary
disposition_rules:
  default: friendly
  rules:
    - has_tag: outlaw
`
	root := mobPack(t, body)
	regs := NewRegistries()
	err := Load(context.Background(), root, nil, regs, nil, nil, nil)
	if err == nil {
		t.Fatal("Load with rule missing 'reaction' = nil, want error")
	}
	if !errors.Is(err, ErrInvalidContent) {
		t.Errorf("err = %v, want %v", err, ErrInvalidContent)
	}
}

func TestLoad_MobTrainerOK(t *testing.T) {
	body := `
id: trainer
name: a trainer
behavior: stationary
tags: [skill_trainer]
trainer:
  tier: novice
  teach: [slash, parry]
`
	root := mobPack(t, body)
	regs := NewRegistries()
	if err := Load(context.Background(), root, nil, regs, nil, nil, nil); err != nil {
		t.Fatalf("Load: %v", err)
	}
	got, _ := regs.Mobs.Get("tapestry-core:trainer")
	if got.TrainerTier != 25 {
		t.Errorf("TrainerTier = %d, want 25 (Novice)", got.TrainerTier)
	}
	if len(got.TrainerTeach) != 2 || got.TrainerTeach[0] != "slash" {
		t.Errorf("TrainerTeach = %v", got.TrainerTeach)
	}
}

func TestLoad_MobTrainerBlockWithoutTagRejected(t *testing.T) {
	body := `
id: trainer
name: a trainer
behavior: stationary
trainer:
  tier: novice
  teach: [slash]
`
	root := mobPack(t, body)
	err := Load(context.Background(), root, nil, NewRegistries(), nil, nil, nil)
	if !errors.Is(err, ErrInvalidContent) {
		t.Fatalf("err = %v, want ErrInvalidContent", err)
	}
}

func TestLoad_MobSkillTrainerTagWithoutBlockRejected(t *testing.T) {
	body := `
id: trainer
name: a trainer
behavior: stationary
tags: [skill_trainer]
`
	root := mobPack(t, body)
	err := Load(context.Background(), root, nil, NewRegistries(), nil, nil, nil)
	if !errors.Is(err, ErrInvalidContent) {
		t.Fatalf("err = %v, want ErrInvalidContent", err)
	}
}

func TestLoad_MobTrainerInvalidTierRejected(t *testing.T) {
	body := `
id: trainer
name: a trainer
behavior: stationary
tags: [skill_trainer]
trainer:
  tier: archmage
  teach: [slash]
`
	root := mobPack(t, body)
	err := Load(context.Background(), root, nil, NewRegistries(), nil, nil, nil)
	if !errors.Is(err, ErrInvalidContent) {
		t.Fatalf("err = %v, want ErrInvalidContent", err)
	}
}
