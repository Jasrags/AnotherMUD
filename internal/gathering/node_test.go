package gathering

import "testing"

func TestDecodeNodeTemplate_ParsesAndNormalizes(t *testing.T) {
	n, err := DecodeNodeTemplate([]byte(`
id: iron-vein
name: an iron ore vein
keywords: [vein, ore, iron]
yield_table: iron-vein-yield
charges: 3
required_tool: pick
`))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if n.ID != "iron-vein" || n.YieldTable != "iron-vein-yield" || n.Charges != 3 || n.RequiredTool != "pick" {
		t.Errorf("node = %+v", n)
	}
	// charges<1 normalizes to 1.
	n2, _ := DecodeNodeTemplate([]byte("id: x\nyield_table: y\ncharges: 0"))
	if n2.Charges != 1 {
		t.Errorf("charges = %d, want 1 (normalized)", n2.Charges)
	}
}

func TestDecodeNodeTemplate_RejectsMissingFields(t *testing.T) {
	if _, err := DecodeNodeTemplate([]byte(`yield_table: y`)); err == nil {
		t.Error("missing id should error")
	}
	if _, err := DecodeNodeTemplate([]byte(`id: x`)); err == nil {
		t.Error("missing yield_table should error")
	}
}

func TestDecodeNodeSpawnTable_ParsesAndNormalizes(t *testing.T) {
	st, err := DecodeNodeSpawnTable([]byte(`
id: cave-nodes
entries:
  - {node: iron-vein, count: 2, reset_interval: 600}
  - {node: copper-vein}
`))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(st.Entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(st.Entries))
	}
	if st.Entries[0].Node != "iron-vein" || st.Entries[0].Count != 2 || st.Entries[0].ResetInterval != 600 {
		t.Errorf("entry0 = %+v", st.Entries[0])
	}
	if st.Entries[1].Count != 1 { // default
		t.Errorf("entry1 count = %d, want 1 (default)", st.Entries[1].Count)
	}
}

func TestDecodeNodeSpawnTable_RejectsEmpty(t *testing.T) {
	if _, err := DecodeNodeSpawnTable([]byte(`id: empty`)); err == nil {
		t.Error("no entries should error")
	}
	if _, err := DecodeNodeSpawnTable([]byte("id: blank\nentries:\n  - {node: \"\"}")); err == nil {
		t.Error("blank node id should error")
	}
}

func TestNodeRegistry_RegisterGet(t *testing.T) {
	r := NewNodeRegistry()
	if err := r.RegisterNode(&NodeTemplate{ID: "Iron-Vein", YieldTable: "y"}); err != nil {
		t.Fatalf("register node: %v", err)
	}
	if _, ok := r.Node("iron-vein"); !ok {
		t.Error("Node lookup should find the lowercased id")
	}
	if err := r.RegisterNode(&NodeTemplate{ID: "iron-vein", YieldTable: "y"}); err == nil {
		t.Error("duplicate node should error")
	}
	if err := r.RegisterSpawnTable(&NodeSpawnTable{ID: "cave", Entries: []NodeSpawnEntry{{Node: "iron-vein", Count: 1}}}); err != nil {
		t.Fatalf("register table: %v", err)
	}
	if _, ok := r.SpawnTable("cave"); !ok {
		t.Error("SpawnTable lookup should find it")
	}
}
