// Package pack discovers content packs, parses their manifests, and
// orders them for the two-phase loader (spec: scripting-and-packs §2–§3).
//
// M2 scope: manifest parse, discovery walk, dependency-ordered load
// sequence. Content loading (rooms, areas) lives in later phases of M2.
package pack

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// EngineNamespace is the reserved namespace for engine-scope packs
// (spec §2.3). Tags/properties declared by a pack with this namespace
// are visible to all packs without a qualifier.
const EngineNamespace = "tapestry-core"

// ManifestFilenames are the filenames the loader recognizes as manifests,
// in lookup order (spec §2.4 acceptance criteria).
var ManifestFilenames = []string{"pack.yaml", "tapestry.yaml"}

// Sentinel errors so callers can switch on classes of failure.
var (
	ErrManifestMissing = errors.New("pack manifest missing")
	ErrManifestInvalid = errors.New("pack manifest invalid")
)

// Manifest is the parsed pack.yaml / tapestry.yaml content.
//
// Only fields meaningful to M2 are wired today. Cosmetic and
// help-precedence fields are accepted (so manifests don't fail YAML
// strict-mode) but unused.
type Manifest struct {
	// Name is the pack name — bare ("legends-forgotten") or scoped
	// ("@scope/pack"). Required.
	Name string `yaml:"name"`

	// Active controls whether the pack is loaded at all (spec §2.1).
	// Default true; only false skips the pack entirely.
	Active *bool `yaml:"active,omitempty"`

	// Version is informational today (spec §2.2).
	Version string `yaml:"version,omitempty"`

	// Dependencies maps pack-name → version-constraint. Only the keys
	// are consulted by the loader (spec §2.2, §3.2); the values are
	// not interpreted today.
	Dependencies map[string]string `yaml:"dependencies,omitempty"`

	// LoadOrder affects help-topic precedence only (spec §2.2).
	// Pack discovery/load order is alphabetical with dep-aware sort.
	LoadOrder int `yaml:"load_order,omitempty"`

	// Validation is "strict" (default) or "lenient" (spec §2.2).
	Validation string `yaml:"validation,omitempty"`

	// Content paths — only the categories M2 cares about are listed.
	// Unknown keys are tolerated (no strict YAML decoding) so future
	// content types do not fail manifests authored ahead of time.
	Content ContentPaths `yaml:"content,omitempty"`
}

// ContentPaths enumerates per-category file globs (spec §2.2 "content"
// block). Paths are relative to the pack directory.
type ContentPaths struct {
	Areas     []string `yaml:"areas,omitempty"`
	Rooms     []string `yaml:"rooms,omitempty"`
	Items     []string `yaml:"items,omitempty"`
	Slots     []string `yaml:"slots,omitempty"`
	Mobs      []string `yaml:"mobs,omitempty"`
	Tracks    []string `yaml:"tracks,omitempty"`
	Races     []string `yaml:"races,omitempty"`
	Classes   []string `yaml:"classes,omitempty"`
	Abilities []string `yaml:"abilities,omitempty"`
	Theme     []string `yaml:"theme,omitempty"`
	Help      []string `yaml:"help,omitempty"`
	Quests    []string `yaml:"quests,omitempty"`
	Effects   []string `yaml:"effects,omitempty"`
	// WeatherZones are zone-definition files for the M15.4
	// weather substrate (spec world-rooms-movement §6).
	// Loaded into Registries.Weather; areas reference zones
	// by id through their `weather_zone` field.
	WeatherZones []string `yaml:"weather_zones,omitempty"`
}

// IsActive reports whether the manifest is active (default true).
func (m *Manifest) IsActive() bool {
	return m.Active == nil || *m.Active
}

// Namespace derives the pack namespace per spec §2.3.
//
// A scoped name `@scope/name` becomes `scope-name`. A bare name passes
// through unchanged.
func (m *Manifest) Namespace() string {
	return DeriveNamespace(m.Name)
}

// DeriveNamespace applies the spec §2.3 rule to an arbitrary pack name.
//
// Exposed because discovery needs to produce namespaces before a
// Manifest value exists (e.g. for error messages on bad parses).
func DeriveNamespace(name string) string {
	trimmed := strings.TrimPrefix(name, "@")
	return strings.ReplaceAll(trimmed, "/", "-")
}

// LoadManifest reads and parses the manifest at path. It does not
// validate cross-pack references — that happens in the two-phase
// loader.
func LoadManifest(path string) (*Manifest, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s", ErrManifestMissing, path)
		}
		return nil, fmt.Errorf("reading manifest %s: %w", path, err)
	}

	var m Manifest
	if err := yaml.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrManifestInvalid, path, err)
	}

	if strings.TrimSpace(m.Name) == "" {
		return nil, fmt.Errorf("%w: %s: missing required field 'name'", ErrManifestInvalid, path)
	}

	return &m, nil
}
