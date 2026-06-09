package biome

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// File is the on-disk YAML shape for a biome definition (biomes.md §2). All
// fields except id are optional. Resource/spawn-table fields name tables
// defined by the relevant feature (gathering = Milestone B); they are
// carried verbatim here.
type File struct {
	ID              string   `yaml:"id"`
	Name            string   `yaml:"name,omitempty"`
	Description     string   `yaml:"description,omitempty"`
	WeatherShielded bool     `yaml:"weather_shielded,omitempty"`
	TimeShielded    bool     `yaml:"time_shielded,omitempty"`
	Ambience        []string `yaml:"ambience,omitempty"`
	ForageTable     string   `yaml:"forage_table,omitempty"`
	NodeSpawnTable  string   `yaml:"node_spawn_table,omitempty"`
	MobSpawnTable   string   `yaml:"mob_spawn_table,omitempty"`
}

// Decode parses one biome YAML document into a *Biome. The id is required;
// everything else defaults to absent/no-effect. The caller registers the
// result (engine- or pack-scope).
func Decode(data []byte) (*Biome, error) {
	var f File
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("biome decode: %w", err)
	}
	if f.ID == "" {
		return nil, fmt.Errorf("biome decode: empty id")
	}
	return &Biome{
		ID:              f.ID,
		DisplayName:     f.Name,
		Description:     f.Description,
		WeatherShielded: f.WeatherShielded,
		TimeShielded:    f.TimeShielded,
		Ambience:        append([]string(nil), f.Ambience...),
		ForageTable:     f.ForageTable,
		NodeSpawnTable:  f.NodeSpawnTable,
		MobSpawnTable:   f.MobSpawnTable,
	}, nil
}
