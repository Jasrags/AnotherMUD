package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// artifact is one emitted file: the emitter that produced it and its path.
type artifact struct {
	Emitter string
	Path    string
}

// packResult is what one pack's render produced — counts for the index summary
// plus every artifact written.
type packResult struct {
	Pack      string
	Rooms     int
	Areas     int
	Artifacts []artifact
}

// writeIndex renders docs/world/index.md — the cross-pack table of contents.
// Written only on a full (`-pack all`) run so a single-pack render never clobbers
// the roll-up. No timestamp: the index is a diffable doc, kept churn-free.
func writeIndex(outDir string, results []packResult) (string, error) {
	sorted := append([]packResult(nil), results...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Pack < sorted[j].Pack })

	var b strings.Builder
	b.WriteString("# World Documentation\n\n")
	b.WriteString("Generated content documentation, one section per world pack. ")
	b.WriteString("Derived from the content packs — regenerate with `make worlddoc` or the ")
	b.WriteString("`world-docs` skill; do not hand-edit.\n\n")

	for _, r := range sorted {
		b.WriteString(fmt.Sprintf("## %s\n\n", r.Pack))
		b.WriteString(fmt.Sprintf("%d rooms · %d areas\n\n", r.Rooms, r.Areas))
		for _, a := range r.Artifacts {
			rel, err := filepath.Rel(outDir, a.Path)
			if err != nil {
				rel = a.Path
			}
			b.WriteString(fmt.Sprintf("- [%s](%s)\n", a.Emitter, filepath.ToSlash(rel)))
		}
		b.WriteString("\n")
	}

	out := filepath.Join(outDir, "index.md")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", fmt.Errorf("creating output dir: %w", err)
	}
	if err := os.WriteFile(out, []byte(b.String()), 0o644); err != nil {
		return "", fmt.Errorf("writing %s: %w", out, err)
	}
	return out, nil
}
