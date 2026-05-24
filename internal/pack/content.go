package pack

// Content YAML schemas — minimal subset of the spec that M2 wires.
// Spec: scripting-and-packs §3.3, world-rooms-movement §2.

// AreaFile is the YAML shape for an area-definition file. One file
// may declare one area today. Multi-area-per-file can come later if
// authors want it.
type AreaFile struct {
	ID          string `yaml:"id"`
	Name        string `yaml:"name"`
	Description string `yaml:"description,omitempty"`
}

// ItemFile is the YAML shape for an item-template file (spec
// inventory-equipment-items §2.2). One file declares one template.
//
// Required: id, name, type. Optional: tags, keywords, properties,
// modifiers. Bare ids resolve against the current pack namespace.
type ItemFile struct {
	ID         string         `yaml:"id"`
	Name       string         `yaml:"name"`
	Type       string         `yaml:"type"`
	Tags       []string       `yaml:"tags,omitempty"`
	Keywords   []string       `yaml:"keywords,omitempty"`
	Properties map[string]any `yaml:"properties,omitempty"`
	Modifiers  []ModifierFile `yaml:"modifiers,omitempty"`
}

// ModifierFile is one entry of an ItemFile.Modifiers list.
type ModifierFile struct {
	Stat  string `yaml:"stat"`
	Value int    `yaml:"value"`
}

// SlotFile is the YAML shape for a pack-defined equipment slot (spec
// inventory-equipment-items §3.1). One file declares one slot. The
// pack's namespace is recorded as the slot's scope tag; the slot's
// name itself is NOT namespaced (slot names are global — see
// internal/slot package doc).
type SlotFile struct {
	Name  string `yaml:"name"`
	Label string `yaml:"label"`
	Max   int    `yaml:"max"`
}

// RoomFile is the YAML shape for a single-room file.
//
// Exits is a map keyed by direction long-name ("north", "up") to keep
// the format pleasant to author. Targets may be bare (resolved against
// the current pack namespace) or fully qualified ("other-pack:foo").
type RoomFile struct {
	ID          string            `yaml:"id"`
	Area        string            `yaml:"area"`
	Name        string            `yaml:"name"`
	Description string            `yaml:"description,omitempty"`
	Exits       map[string]string `yaml:"exits,omitempty"`
}
