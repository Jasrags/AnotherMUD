package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// guideEmitter writes guide.md — a player-facing orientation assembled straight
// from the pack YAML (no hand-authored prose, so it regenerates in sync). It
// opens where the player starts, tours the world region → area using each area's
// own description, and ends with a directory of where to find services.
var guideEmitter = emitter{
	name: "guide",
	render: func(m *worldModel, packDir string) ([]string, error) {
		md := renderGuide(m)
		out := filepath.Join(packDir, "guide.md")
		if err := os.MkdirAll(packDir, 0o755); err != nil {
			return nil, fmt.Errorf("creating output dir: %w", err)
		}
		if err := os.WriteFile(out, []byte(md), 0o644); err != nil {
			return nil, fmt.Errorf("writing %s: %w", out, err)
		}
		return []string{out}, nil
	},
}

// guideServiceOrder is the fixed display order for player-facing services and
// their labels; it drives both the per-area "notable stops" and the directory.
var guideServiceOrder = []struct{ key, label, plural string }{
	{"shop", "shop", "Shops"},
	{"trainer", "trainer", "Trainers"},
	{"quest", "quest giver", "Quest givers"},
	{"stable", "stable", "Stables"},
	{"recruiter", "recruiter", "Recruiters"},
}

func renderGuide(m *worldModel) string {
	roomsByArea := map[string][]string{}
	for id, r := range m.Rooms {
		roomsByArea[r.Area] = append(roomsByArea[r.Area], id)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# %s — Player's Guide\n\n", m.Pack)
	fmt.Fprintf(&b, "A traveler's orientation to the `%s` world, drawn from the world itself. ", m.Pack)
	b.WriteString("Derived from the pack YAML — regenerate with `make worlddoc` or the `world-docs` skill; do not hand-edit.\n")

	writeGuideStart(&b, m)
	writeGuideWorld(&b, m, roomsByArea)
	writeGuideDirectory(&b, m)
	return b.String()
}

func writeGuideStart(b *strings.Builder, m *worldModel) {
	b.WriteString("\n## Getting Started\n\n")
	start, ok := m.Rooms[m.Start]
	if !ok {
		b.WriteString("This world has no designated starting room.\n")
		return
	}
	area := m.Areas[start.Area]
	where := orNone(clean(start.Name))
	if area.Name != "" {
		where += ", in " + clean(area.Name)
		if area.Region != "" {
			where += " (" + regionTitle(area.Region) + ")"
		}
	}
	fmt.Fprintf(b, "You begin in **%s**.\n", where)
	if d := cleanPara(start.Description); d != "" {
		fmt.Fprintf(b, "\n%s\n", d)
	}
	if paths := exitSentence(m, start); paths != "" {
		fmt.Fprintf(b, "\n%s\n", paths)
	}
}

// writeGuideWorld tours regions → areas, using each area's description. Only
// areas that actually hold rooms appear, so every region with content shows up.
func writeGuideWorld(b *strings.Builder, m *worldModel, roomsByArea map[string][]string) {
	regionAreas := map[string][]string{}
	for aid := range m.Areas {
		if len(roomsByArea[aid]) > 0 {
			region := m.Areas[aid].Region
			regionAreas[region] = append(regionAreas[region], aid)
		}
	}
	regions := make([]string, 0, len(regionAreas))
	for r := range regionAreas {
		regions = append(regions, r)
	}
	sort.Slice(regions, func(i, j int) bool { return emptyLast(regions[i], regions[j]) })

	b.WriteString("\n## The World\n")
	for _, region := range regions {
		fmt.Fprintf(b, "\n### %s\n", regionTitle(region))
		areas := regionAreas[region]
		sort.Slice(areas, func(i, j int) bool {
			return emptyLast(m.Areas[areas[i]].Name, m.Areas[areas[j]].Name)
		})
		for _, aid := range areas {
			a := m.Areas[aid]
			fmt.Fprintf(b, "\n**%s** — %s\n", orNone(clean(a.Name)), orNone(cleanPara(a.Description)))
			writeAreaStops(b, m, roomsByArea[aid])
		}
	}
}

// writeAreaStops lists the rooms in an area that offer a player service.
func writeAreaStops(b *strings.Builder, m *worldModel, roomIDs []string) {
	ids := append([]string(nil), roomIDs...)
	sort.Strings(ids)
	var stops []string
	for _, id := range ids {
		r := m.Rooms[id]
		if labels := serviceLabels(roomServices(m, r)); len(labels) > 0 {
			stops = append(stops, fmt.Sprintf("- **%s**: %s", orNone(clean(r.Name)), strings.Join(labels, ", ")))
		}
	}
	if len(stops) > 0 {
		b.WriteString("\nNotable stops:\n")
		for _, s := range stops {
			b.WriteString(s + "\n")
		}
	}
}

// writeGuideDirectory aggregates every service across the world into a
// where-to-find-it list.
func writeGuideDirectory(b *strings.Builder, m *worldModel) {
	byService := map[string][]string{}
	for _, id := range sortedKeys(m.Rooms) {
		r := m.Rooms[id]
		svc := roomServices(m, r)
		loc := clean(r.Name)
		if a := m.Areas[r.Area]; a.Name != "" {
			loc += " (" + clean(a.Name) + ")"
		}
		for k := range svc {
			byService[k] = append(byService[k], loc)
		}
	}

	b.WriteString("\n## Where to Find Things\n")
	any := false
	for _, s := range guideServiceOrder {
		locs := byService[s.key]
		if len(locs) == 0 {
			continue
		}
		any = true
		sort.Strings(locs)
		fmt.Fprintf(b, "\n**%s:** %s\n", s.plural, strings.Join(locs, "; "))
	}
	if !any {
		b.WriteString("\nNo shops, trainers, quest givers, or stables are placed in this world yet.\n")
	}
}

// roomServices returns the player-facing service keys offered by a room's mobs.
func roomServices(m *worldModel, r roomYAML) map[string]bool {
	s := map[string]bool{}
	for _, mid := range r.Mobs {
		mob, ok := m.Mobs[mid]
		if !ok {
			continue
		}
		if mob.Shop {
			s["shop"] = true
		}
		if mob.Trainer {
			s["trainer"] = true
		}
		if mob.Stable {
			s["stable"] = true
		}
		if mob.Quest {
			s["quest"] = true
		}
		if mob.Hireling || mob.Recruiter {
			s["recruiter"] = true
		}
	}
	return s
}

// serviceLabels renders a room's service set in the fixed display order.
func serviceLabels(svc map[string]bool) []string {
	var out []string
	for _, s := range guideServiceOrder {
		if svc[s.key] {
			out = append(out, s.label)
		}
	}
	return out
}

// exitSentence renders a room's exits as "Paths lead north to <room>, …".
func exitSentence(m *worldModel, r roomYAML) string {
	var parts []string
	for _, dir := range dirOrder {
		to := r.Exits[dir]
		if to == "" {
			continue
		}
		dest := to
		if tr, ok := m.Rooms[to]; ok && tr.Name != "" {
			dest = clean(tr.Name)
		}
		parts = append(parts, fmt.Sprintf("%s to %s", dir, dest))
	}
	if len(parts) == 0 {
		return ""
	}
	return "Paths lead " + strings.Join(parts, ", ") + "."
}

// cleanPara flattens a (possibly multi-line block) description into one trimmed,
// markup-free line of prose.
func cleanPara(s string) string {
	s = clean(s)
	return strings.Join(strings.Fields(s), " ")
}
