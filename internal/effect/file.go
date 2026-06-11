package effect

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/stats"
)

// File is the on-disk YAML shape for a single effect template,
// one template per file (mirrors the engine's other content
// conventions: one item per file, one mob per file, etc.).
//
// Schema:
//
//	id: bless
//	duration: 300            # ticks; >0 bounded, <0 permanent
//	modifiers:               # optional
//	  - stat: hit_mod
//	    value: 2
//	flags:                   # optional
//	  - blessed
type File struct {
	ID          string         `yaml:"id"`
	Duration    int            `yaml:"duration,omitempty"`
	Modifiers   []ModifierFile `yaml:"modifiers,omitempty"`
	Flags       []string       `yaml:"flags,omitempty"`
	Refreshable bool           `yaml:"refreshable,omitempty"`
	// RecurringSave, when set, gives the bearer a per-tick shake-off save to
	// end the effect early (conditions §4). Used by incapacitating
	// conditions (stunned/frightened) so they don't simply run a fixed
	// duration. Omitted ⇒ the effect runs its full duration.
	RecurringSave *SaveFile `yaml:"recurring_save,omitempty"`
}

// SaveFile is the on-disk shape for a condition save (conditions §4): an
// axis (fortitude/reflex/will) and a difficulty class.
type SaveFile struct {
	Axis string `yaml:"axis"`
	DC   int    `yaml:"dc"`
}

// ModifierFile mirrors stats.Modifier with explicit YAML tags so
// the on-disk shape matches the rest of the engine's stat-modifier
// content (ability effect blocks use the same shape).
type ModifierFile struct {
	Stat  string `yaml:"stat"`
	Value int    `yaml:"value"`
}

// Decode parses YAML bytes into a progression.EffectTemplate ready
// for Registry.Register. Returns an error on malformed YAML, missing
// id, or empty modifier slot with a non-empty Stat (malformed YAML
// surfaces as a clear load-time error instead of a silent zero).
func Decode(data []byte) (progression.EffectTemplate, error) {
	var f File
	if err := yaml.Unmarshal(data, &f); err != nil {
		return progression.EffectTemplate{}, fmt.Errorf("effect decode: %w", err)
	}
	if f.ID == "" {
		return progression.EffectTemplate{}, fmt.Errorf("effect decode: empty id")
	}
	tpl := progression.EffectTemplate{
		ID:          f.ID,
		Duration:    f.Duration,
		Flags:       append([]string(nil), f.Flags...),
		Refreshable: f.Refreshable,
	}
	if f.RecurringSave != nil {
		axis := progression.SaveType(strings.ToLower(strings.TrimSpace(f.RecurringSave.Axis)))
		switch axis {
		case progression.SaveFortitude, progression.SaveReflex, progression.SaveWill:
		default:
			return progression.EffectTemplate{}, fmt.Errorf(
				"effect decode (%q): recurring_save axis %q must be fortitude/reflex/will", f.ID, f.RecurringSave.Axis)
		}
		tpl.RecurringSave = &progression.ConditionSave{Axis: axis, DC: f.RecurringSave.DC}
	}
	if len(f.Modifiers) > 0 {
		tpl.Modifiers = make([]stats.Modifier, 0, len(f.Modifiers))
		for i, m := range f.Modifiers {
			if m.Stat == "" {
				return progression.EffectTemplate{}, fmt.Errorf(
					"effect decode (%q): modifier[%d] has empty stat", f.ID, i)
			}
			tpl.Modifiers = append(tpl.Modifiers, stats.Modifier{
				Stat:  m.Stat,
				Value: m.Value,
			})
		}
	}
	return tpl, nil
}
