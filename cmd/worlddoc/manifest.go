package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

// packManifest is the subset of a pack's pack.yaml the doc tool reads: its kind
// (world vs library) and the content map (content-type → globs). The globs are
// authoritative — a type's directory need not match its key (e.g. channel_map →
// channel-map/*.yaml), so generic cataloging must glob by the manifest, not by
// convention.
type packManifest struct {
	Name    string              `yaml:"name"`
	Kind    string              `yaml:"kind"`
	Content map[string][]string `yaml:"content"`
}

func (mf packManifest) isWorld() bool { return mf.Kind == "world" }

// loadManifest reads and parses a pack's pack.yaml.
func loadManifest(base string) (packManifest, error) {
	b, err := os.ReadFile(filepath.Join(base, "pack.yaml"))
	if err != nil {
		return packManifest{}, fmt.Errorf("reading manifest: %w", err)
	}
	var mf packManifest
	if err := yaml.Unmarshal(b, &mf); err != nil {
		return packManifest{}, fmt.Errorf("%s/pack.yaml: %w", base, err)
	}
	return mf, nil
}

// discoverPacks returns every pack directory under content (world and library),
// sorted by directory name. The directory basename is the pack id used for paths
// and links — a manifest's own `name` may differ (core's is "tapestry-core").
func discoverPacks(content string) ([]packManifest, error) {
	entries, err := os.ReadDir(content)
	if err != nil {
		return nil, fmt.Errorf("reading content dir %s: %w", content, err)
	}
	var out []packManifest
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		mf, err := loadManifest(filepath.Join(content, e.Name()))
		if err != nil {
			continue // not a pack directory
		}
		mf.Name = e.Name() // dir basename is the pack id for paths/links
		out = append(out, mf)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
