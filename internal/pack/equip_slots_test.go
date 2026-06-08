package pack

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/slot"
)

// TestDecodeItem_EligibleAndCompanionSlots covers the §2.2/§3.3 slot
// declarations and the §3.2 legacy `properties.slot` bridge: an item
// with no explicit eligible_slots inherits its single legacy slot as a
// one-element eligible set, so existing content keeps working untouched.
func TestDecodeItem_EligibleAndCompanionSlots(t *testing.T) {
	tests := []struct {
		name          string
		body          string
		wantEligible  []string
		wantCompanion []string
	}{
		{
			name: "legacy slot property lifts to one-element eligible set",
			body: "id: x\nname: a thing\ntype: item\nproperties:\n  slot: wield\n",
			wantEligible:  []string{"wield"},
			wantCompanion: nil,
		},
		{
			name: "explicit eligible_slots",
			body: "id: x\nname: a thing\ntype: item\neligible_slots: [wield, offhand]\n",
			wantEligible:  []string{"wield", "offhand"},
			wantCompanion: nil,
		},
		{
			name: "explicit eligible_slots wins over legacy slot property",
			body: "id: x\nname: a thing\ntype: item\neligible_slots: [offhand]\nproperties:\n  slot: wield\n",
			wantEligible:  []string{"offhand"},
			wantCompanion: nil,
		},
		{
			name: "companion slots decode alongside eligible",
			body: "id: x\nname: a greatsword\ntype: item\neligible_slots: [wield]\ncompanion_slots: [offhand]\n",
			wantEligible:  []string{"wield"},
			wantCompanion: []string{"offhand"},
		},
		{
			name: "names are lowercased, trimmed, and deduped",
			body: "id: x\nname: a thing\ntype: item\neligible_slots: [\"  WIELD \", wield, Offhand]\n",
			wantEligible:  []string{"wield", "offhand"},
			wantCompanion: nil,
		},
		{
			name: "legacy slot is normalized too",
			body: "id: x\nname: a thing\ntype: item\nproperties:\n  slot: \"  Head \"\n",
			wantEligible:  []string{"head"},
			wantCompanion: nil,
		},
		{
			name: "no slot info means not equippable",
			body: "id: x\nname: a quest token\ntype: item\n",
			wantEligible:  nil,
			wantCompanion: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "item.yaml")
			writeFile(t, path, tt.body)
			tpl, err := decodeItem(path, "tapestry-core")
			if err != nil {
				t.Fatalf("decodeItem: %v", err)
			}
			if !slices.Equal(tpl.EligibleSlots, tt.wantEligible) {
				t.Errorf("EligibleSlots = %v, want %v", tpl.EligibleSlots, tt.wantEligible)
			}
			if !slices.Equal(tpl.CompanionSlots, tt.wantCompanion) {
				t.Errorf("CompanionSlots = %v, want %v", tpl.CompanionSlots, tt.wantCompanion)
			}
		})
	}
}

// TestLoad_RejectsUnknownItemSlot exercises the boot post-pass
// (validateItemSlots): an item naming a slot no pack registers fails the
// load with ErrItemUnknownSlot, naming the offending template — a typo
// surfaces at boot instead of as a silently never-equippable item.
func TestLoad_RejectsUnknownItemSlot(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  items: [items/*.yaml]
`)
	writeFile(t, filepath.Join(pack, "items/bad.yaml"),
		"id: bad\nname: a confused trinket\ntype: item\neligible_slots: [nonesuch]\n")

	regs := NewRegistries()
	if err := slot.RegisterEngineBaseline(regs.Slots); err != nil {
		t.Fatalf("baseline: %v", err)
	}
	err := Load(context.Background(), root, nil, regs, nil, nil, nil)
	if !errors.Is(err, ErrItemUnknownSlot) {
		t.Fatalf("Load err = %v, want ErrItemUnknownSlot", err)
	}
}

// TestLoad_AcceptsKnownItemSlots is the happy-path counterpart: an item
// whose eligible + companion slots all resolve loads cleanly, and the
// template carries the decoded slot sets.
func TestLoad_AcceptsKnownItemSlots(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  items: [items/*.yaml]
`)
	writeFile(t, filepath.Join(pack, "items/greatsword.yaml"),
		"id: greatsword\nname: a greatsword\ntype: item\neligible_slots: [wield]\ncompanion_slots: [offhand]\n")

	regs := NewRegistries()
	if err := slot.RegisterEngineBaseline(regs.Slots); err != nil {
		t.Fatalf("baseline: %v", err)
	}
	if err := Load(context.Background(), root, nil, regs, nil, nil, nil); err != nil {
		t.Fatalf("Load: %v", err)
	}
	tpl, err := regs.Items.Get("tapestry-core:greatsword")
	if err != nil {
		t.Fatalf("greatsword missing: %v", err)
	}
	if !slices.Equal(tpl.EligibleSlots, []string{"wield"}) {
		t.Errorf("EligibleSlots = %v, want [wield]", tpl.EligibleSlots)
	}
	if !slices.Equal(tpl.CompanionSlots, []string{"offhand"}) {
		t.Errorf("CompanionSlots = %v, want [offhand]", tpl.CompanionSlots)
	}
}
