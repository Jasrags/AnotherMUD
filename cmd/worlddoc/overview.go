package main

import (
	"fmt"
	"sort"
	"strings"
)

// overviewEmitter writes index.html — a pack's landing page: a summary, its
// regions, and cards linking to each section. It is the sidebar's "Overview".
var overviewEmitter = emitter{
	name: "overview",
	render: func(m *worldModel, packDir string) ([]string, error) {
		lede := fmt.Sprintf("%d rooms across %d areas.", len(m.Rooms), len(m.Areas))
		if m.Kind != "world" {
			lede = fmt.Sprintf("Library pack — %d content types shared with the worlds that depend on it.", catalogableTypeCount(m))
		}
		page, err := renderPage(m.Pack, "overview", "Overview", lede, renderOverview(m))
		if err != nil {
			return nil, err
		}
		return writeSitePage(packDir, "index.html", page)
	},
}

// overviewCards is the section directory shown on a pack's landing page. Cards
// flagged worldOnly are omitted for library packs (which have no rooms).
var overviewCards = []struct {
	file, title, desc string
	worldOnly         bool
}{
	{"map.html", "Map", "Interactive pan/zoom map of every room, region-tinted, with feature badges, filters, and search.", true},
	{"gazetteer.html", "Gazetteer", "Region → area → room reference: exits (with door/locked/hidden markers), resident NPCs, and per-room notes.", true},
	{"catalogs.html", "Catalogs", "Reference tables of the content this pack ships — creatures, items, characters, abilities, world systems, and more.", false},
	{"health.html", "World Health", "Authoring-gap audit: unreachable/orphan rooms, dangling exits, undescribed rooms, and more.", true},
	{"guide.html", "Player's Guide", "A player-facing orientation: where you start, a tour of the world, and where to find services.", true},
}

// catalogableTypeCount counts the content types a pack declares that the catalog
// documents (everything except areas/rooms/scripts).
func catalogableTypeCount(m *worldModel) int {
	n := 0
	for t := range m.Content {
		if !catalogSkip[t] {
			n++
		}
	}
	return n
}

func renderOverview(m *worldModel) string {
	var b strings.Builder

	regionSet := map[string]bool{}
	for _, a := range m.Areas {
		if a.Region != "" {
			regionSet[a.Region] = true
		}
	}
	if len(regionSet) > 0 {
		regions := make([]string, 0, len(regionSet))
		for r := range regionSet {
			regions = append(regions, r)
		}
		sort.Strings(regions)
		b.WriteString("<h2>Regions</h2><p>")
		parts := make([]string, len(regions))
		for i, r := range regions {
			parts[i] = tag("faction", regionTitle(r))
		}
		b.WriteString(strings.Join(parts, " "))
		b.WriteString("</p>")
	}

	b.WriteString(`<h2>Sections</h2><div class="cards">`)
	for _, c := range overviewCards {
		if c.worldOnly && m.Kind != "world" {
			continue
		}
		fmt.Fprintf(&b, `<a class="card" href="%s"><h3>%s</h3><p>%s</p></a>`, c.file, esc(c.title), esc(c.desc))
	}
	b.WriteString("</div>")
	return b.String()
}
