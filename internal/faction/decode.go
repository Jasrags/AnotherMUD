package faction

import "gopkg.in/yaml.v3"

// yamlRank / yamlDef decode a faction content file. Min/Max/Starting are
// pointers so Decode can tell an omitted field (→ registry default) from an
// explicit zero.
type yamlRank struct {
	Name      string `yaml:"name"`
	Threshold int    `yaml:"threshold"`
}

type yamlDef struct {
	ID          string     `yaml:"id"`
	Name        string     `yaml:"name"`
	Description string     `yaml:"description"`
	Ranks       []yamlRank `yaml:"ranks"`
	Min         *int       `yaml:"min"`
	Max         *int       `yaml:"max"`
	Starting    *int       `yaml:"starting"`
}

// Decode parses a faction definition file (faction.md §2). It returns the
// Definition plus flags recording which of the optional bounds/starting fields
// the source actually supplied, so Registry.AddWithFlags fills defaults only
// for the omitted ones. The ladder is left as authored (empty → Add inherits
// the registry default ladder); Add sorts it.
func Decode(data []byte) (def Definition, hasMin, hasMax, hasStarting bool, err error) {
	var y yamlDef
	if err = yaml.Unmarshal(data, &y); err != nil {
		return Definition{}, false, false, false, err
	}
	def.ID = y.ID
	def.Name = y.Name
	def.Description = y.Description
	for _, r := range y.Ranks {
		def.Ranks = append(def.Ranks, Rank{Name: r.Name, Threshold: r.Threshold})
	}
	if y.Min != nil {
		def.Min, hasMin = *y.Min, true
	}
	if y.Max != nil {
		def.Max, hasMax = *y.Max, true
	}
	if y.Starting != nil {
		def.Starting, hasStarting = *y.Starting, true
	}
	return def, hasMin, hasMax, hasStarting, nil
}
