package main

import (
	"fmt"
	"sort"
	"strings"
)

// guideEmitter writes guide.html — a player-facing orientation assembled straight
// from the pack YAML (no hand-authored prose, so it regenerates in sync). It
// opens where the player starts, tours the world region → area using each area's
// own description, and ends with a directory of where to find services.
var guideEmitter = emitter{
	name:      "guide",
	worldOnly: true,
	render: func(m *worldModel, packDir string) ([]string, error) {
		body := renderGuide(m)
		page, err := renderPage(m.Pack, "guide", "Player's Guide", "A traveler's orientation, drawn from the world itself.", body)
		if err != nil {
			return nil, err
		}
		return writeSitePage(packDir, "guide.html", page)
	},
}

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
	writeGuideStart(&b, m)
	writeGuideWorld(&b, m, roomsByArea)
	writeGuideDirectory(&b, m)
	return b.String()
}

func writeGuideStart(b *strings.Builder, m *worldModel) {
	b.WriteString("<h2>Getting Started</h2>")
	start, ok := m.Rooms[m.Start]
	if !ok {
		b.WriteString(`<p class="empty">This world has no designated starting room.</p>`)
		return
	}
	area := m.Areas[start.Area]
	where := escName(orNone(start.Name))
	if area.Name != "" {
		where += ", in " + escName(area.Name)
		if area.Region != "" {
			where += " (" + esc(regionTitle(area.Region)) + ")"
		}
	}
	fmt.Fprintf(b, "<p>You begin in <strong>%s</strong>.</p>", where)
	if d := cleanPara(start.Description); d != "" {
		fmt.Fprintf(b, "<p>%s</p>", esc(d))
	}
	if paths := exitSentence(m, start); paths != "" {
		fmt.Fprintf(b, "<p>%s</p>", paths)
	}
}

func writeGuideWorld(b *strings.Builder, m *worldModel, roomsByArea map[string][]string) {
	regionAreas := map[string][]string{}
	for aid := range m.Areas {
		if len(roomsByArea[aid]) > 0 {
			regionAreas[m.Areas[aid].Region] = append(regionAreas[m.Areas[aid].Region], aid)
		}
	}
	regions := make([]string, 0, len(regionAreas))
	for r := range regionAreas {
		regions = append(regions, r)
	}
	sort.Slice(regions, func(i, j int) bool { return emptyLast(regions[i], regions[j]) })

	b.WriteString("<h2>The World</h2>")
	for _, region := range regions {
		fmt.Fprintf(b, "<h3>%s</h3>", esc(regionTitle(region)))
		areas := regionAreas[region]
		sort.Slice(areas, func(i, j int) bool {
			return emptyLast(m.Areas[areas[i]].Name, m.Areas[areas[j]].Name)
		})
		for _, aid := range areas {
			a := m.Areas[aid]
			fmt.Fprintf(b, "<p><strong>%s</strong> — %s</p>", escName(orNone(a.Name)), esc(orNone(cleanPara(a.Description))))
			writeAreaStops(b, m, roomsByArea[aid])
		}
	}
}

func writeAreaStops(b *strings.Builder, m *worldModel, roomIDs []string) {
	ids := append([]string(nil), roomIDs...)
	sort.Strings(ids)
	var stops []string
	for _, id := range ids {
		r := m.Rooms[id]
		svc := roomServices(m, r)
		if tags := serviceTags(svc); tags != "" {
			stops = append(stops, fmt.Sprintf("<li><strong>%s</strong> %s</li>", escName(orNone(r.Name)), tags))
		}
	}
	if len(stops) > 0 {
		b.WriteString("<ul>")
		for _, s := range stops {
			b.WriteString(s)
		}
		b.WriteString("</ul>")
	}
}

func writeGuideDirectory(b *strings.Builder, m *worldModel) {
	byService := map[string][]string{}
	for _, id := range sortedKeys(m.Rooms) {
		r := m.Rooms[id]
		loc := escName(r.Name)
		if a := m.Areas[r.Area]; a.Name != "" {
			loc += " (" + escName(a.Name) + ")"
		}
		for k := range roomServices(m, r) {
			byService[k] = append(byService[k], loc)
		}
	}

	b.WriteString("<h2>Where to Find Things</h2>")
	found := false
	for _, s := range guideServiceOrder {
		locs := byService[s.key]
		if len(locs) == 0 {
			continue
		}
		found = true
		sort.Strings(locs)
		fmt.Fprintf(b, "<p><strong>%s:</strong> %s</p>", esc(s.plural), strings.Join(locs, "; "))
	}
	if !found {
		b.WriteString(`<p class="empty">No shops, trainers, quest givers, or stables are placed in this world yet.</p>`)
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

// serviceTags renders a room's service set as pills in the fixed display order.
func serviceTags(svc map[string]bool) string {
	var out []string
	for _, s := range guideServiceOrder {
		if svc[s.key] {
			out = append(out, tag(s.key, s.label))
		}
	}
	return strings.Join(out, " ")
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
		parts = append(parts, esc(dir)+" to "+esc(dest))
	}
	if len(parts) == 0 {
		return ""
	}
	return "Paths lead " + strings.Join(parts, ", ") + "."
}

// cleanPara flattens a (possibly multi-line block) description into one trimmed,
// markup-free line of prose.
func cleanPara(s string) string {
	return strings.Join(strings.Fields(clean(s)), " ")
}
