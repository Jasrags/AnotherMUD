package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// packResult is what one pack's render produced — counts for the landing page
// plus every artifact path written.
type packResult struct {
	Pack  string
	Rooms int
	Areas int
	Paths []string
}

// writeLanding renders docs/world/index.html — the cross-pack landing page, a
// grid of pack cards. Written only on a full (`-pack all`) run. This is the one
// page assembled outside html/template (it has no sidebar shell), so any dynamic
// value written here MUST be escaped via esc() first.
func writeLanding(outDir string, results []packResult) (string, error) {
	sorted := append([]packResult(nil), results...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Pack < sorted[j].Pack })

	var cards strings.Builder
	cards.WriteString(`<div class="cards">`)
	for _, r := range sorted {
		fmt.Fprintf(&cards, `<a class="card" href="%s/index.html"><h3>%s</h3><p>%d rooms · %d areas</p></a>`,
			esc(r.Pack), esc(r.Pack), r.Rooms, r.Areas)
	}
	cards.WriteString(`</div>`)

	page := fmt.Sprintf(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>World Documentation</title>
<style>%s</style>
</head>
<body>
<main>
<div class="page-head"><h1>World Documentation</h1><p class="lede">Generated content documentation, one section per world pack. Derived from the content packs — regenerate with <code>make worlddoc</code> or the world-docs skill; do not hand-edit.</p></div>
%s
</main>
</body>
</html>
`, siteCSS, cards.String())

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", fmt.Errorf("creating output dir: %w", err)
	}
	out := filepath.Join(outDir, "index.html")
	if err := os.WriteFile(out, []byte(page), 0o644); err != nil {
		return "", fmt.Errorf("writing %s: %w", out, err)
	}
	return out, nil
}
