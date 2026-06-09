package gathering

import "testing"

func TestDecodeForageTable_ParsesAndNormalizes(t *testing.T) {
	tbl, err := DecodeForageTable([]byte(`
id: forest-forage
richness: 150
ceiling: uncommon
entries:
  - {item: herb, weight: 3}
  - {item: berries, weight: 1, qty: 2}
  - {item: bark, weight: 0}
`))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if tbl.ID != "forest-forage" || tbl.Ceiling != "uncommon" {
		t.Errorf("table = %+v", tbl)
	}
	if tbl.Richness != 100 {
		t.Errorf("richness = %d, want clamped to 100", tbl.Richness)
	}
	if len(tbl.Entries) != 3 || tbl.Entries[0].Qty != 1 || tbl.Entries[1].Qty != 2 {
		t.Errorf("entries = %+v (qty default/parse)", tbl.Entries)
	}
}

func TestDecodeForageTable_RejectsEmptyAndUnselectable(t *testing.T) {
	if _, err := DecodeForageTable([]byte(`richness: 10`)); err == nil {
		t.Error("decode with no id should error")
	}
	if _, err := DecodeForageTable([]byte(`id: empty`)); err == nil {
		t.Error("decode with no entries should error")
	}
	// All entries zero-weight → unselectable → error (can never yield).
	if _, err := DecodeForageTable([]byte(`
id: dead
entries:
  - {item: rock, weight: 0}
`)); err == nil {
		t.Error("decode with no positive-weight entry should error")
	}
	if _, err := DecodeForageTable([]byte(`
id: blankitem
entries:
  - {item: "", weight: 3}
`)); err == nil {
		t.Error("decode with a blank item id should error")
	}
}

func TestForageRegistry_RegisterGet(t *testing.T) {
	r := NewForageRegistry()
	if err := r.Register(&ForageTable{ID: "Forest-Forage", Entries: []ForageEntry{{Item: "herb", Weight: 1, Qty: 1}}}); err != nil {
		t.Fatalf("register: %v", err)
	}
	if _, ok := r.Get("forest-forage"); !ok {
		t.Error("Get should find the lowercased id")
	}
	if err := r.Register(&ForageTable{ID: "forest-forage"}); err == nil {
		t.Error("duplicate id should error")
	}
}
