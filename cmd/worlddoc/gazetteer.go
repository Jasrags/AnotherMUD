package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// gazetteerEmitter renders gazetteer.html — a region → area → room reference. It
// reuses assemble() (the map's resolver) so exits, doors, hidden flags, and NPC
// roles stay consistent with the map, then groups the id-sorted rooms under
// their area and region.
var gazetteerEmitter = emitter{
	name:      "gazetteer",
	worldOnly: true,
	render: func(m *worldModel, packDir string) ([]string, error) {
		w := assemble(m)
		lede := fmt.Sprintf("Region → area → room reference — %d rooms across %d areas.", len(w.Rooms), len(w.Areas))
		body := renderGazetteer(w, roomSpawnIndex(m))
		page, err := renderPage(m.Pack, "gazetteer", "Gazetteer", lede, body)
		if err != nil {
			return nil, err
		}
		return writeSitePage(packDir, "gazetteer.html", page)
	},
}

// renderGazetteer walks the resolved world room-by-room (so every room appears
// exactly once) grouped region → area → room. Rooms drive the grouping, so an
// area with no rooms simply doesn't appear (the health report flags those).
func renderGazetteer(w worldJSON, roomSpawns map[string][]string) string {
	areaName := make(map[string]string, len(w.Areas))
	for _, a := range w.Areas {
		areaName[a.ID] = a.Name
	}
	areaWeather := map[string]string{}
	for _, r := range w.Rooms {
		if r.Weather != "" {
			areaWeather[r.Area] = r.Weather
		}
	}

	type row struct {
		region string
		areaID string
		aName  string
		r      roomJSON
	}
	rows := make([]row, 0, len(w.Rooms))
	for _, r := range w.Rooms {
		rows = append(rows, row{region: r.Region, areaID: r.Area, aName: areaName[r.Area], r: r})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if a.region != b.region {
			return emptyLast(a.region, b.region)
		}
		if a.aName != b.aName {
			return emptyLast(a.aName, b.aName)
		}
		if a.areaID != b.areaID {
			return a.areaID < b.areaID
		}
		return a.r.ID < b.r.ID
	})

	var b strings.Builder
	const none = "\x00"
	curRegion, curArea := none, none
	for _, rw := range rows {
		if rw.region != curRegion {
			curRegion, curArea = rw.region, none
			fmt.Fprintf(&b, "<h2>%s</h2>", esc(regionTitle(rw.region)))
		}
		if rw.areaID != curArea {
			curArea = rw.areaID
			title := rw.aName
			if title == "" {
				title = orNone(rw.areaID)
			}
			var annot []string
			if rw.areaID != "" {
				annot = append(annot, codeID(rw.areaID))
			}
			if wz := areaWeather[rw.areaID]; wz != "" {
				annot = append(annot, "weather: "+esc(wz))
			}
			suffix := ""
			if len(annot) > 0 {
				suffix = ` <span class="id">` + strings.Join(annot, " · ") + `</span>`
			}
			fmt.Fprintf(&b, "<h3>%s%s</h3>", escName(title), suffix)
		}
		writeGazRoom(&b, rw.r, roomSpawns[rw.r.ID])
	}
	return b.String()
}

func writeGazRoom(b *strings.Builder, r roomJSON, spawns []string) {
	fmt.Fprintf(b, `<div class="entry"><h4>%s <span class="id">%s</span></h4>`, escName(r.Name), codeID(r.ID))

	fmt.Fprintf(b, `<div><span class="attr">Terrain</span> %s`, esc(orNone(r.Terrain)))
	for _, n := range roomNoteTags(r) {
		b.WriteString(" " + n)
	}
	b.WriteString("</div>")

	b.WriteString(`<div><span class="attr">Exits</span>`)
	if len(r.Exits) == 0 {
		b.WriteString(` <span class="empty">none</span></div>`)
	} else {
		b.WriteString("<ul>")
		for _, e := range r.Exits {
			var marks []string
			if e.Cross {
				marks = append(marks, `<span class="marker cross">cross-area</span>`)
			}
			switch {
			case e.Locked && e.Door != "":
				marks = append(marks, tag("locked", "locked door: "+e.Door))
			case e.Locked:
				marks = append(marks, tag("locked", "locked"))
			case e.Door != "":
				marks = append(marks, tag("", "door: "+e.Door))
			}
			if e.Hidden {
				marks = append(marks, tag("hidden", "hidden"))
			}
			fmt.Fprintf(b, "<li>%s → %s %s</li>", esc(e.Dir), codeID(e.To), strings.Join(marks, " "))
		}
		b.WriteString("</ul></div>")
	}

	if len(r.Mobs) > 0 {
		b.WriteString(`<div><span class="attr">NPCs</span> `)
		parts := make([]string, 0, len(r.Mobs))
		for _, m := range r.Mobs {
			parts = append(parts, npcHTML(m))
		}
		b.WriteString(strings.Join(parts, "  ·  "))
		b.WriteString("</div>")
	}
	// Quest-scoped spawns (quest-spawns.md): content a run creates here at
	// runtime, which is absent from the static NPC/item lists above.
	if len(spawns) > 0 {
		b.WriteString(`<div><span class="attr">Quest spawns</span> `)
		b.WriteString(strings.Join(spawns, " · "))
		b.WriteString("</div>")
	}
	b.WriteString("</div>")
}

// roomSpawnIndex maps a room id to the quest-scoped spawns targeting it
// (quest-spawns.md) — one entry per quest that spawns there, rendered as
// "<quest> — N× <template> (<kind>), …". Lets the gazetteer note runtime
// content the static room lists can't show.
func roomSpawnIndex(m *worldModel) map[string][]string {
	out := map[string][]string{}
	for _, q := range m.Quests {
		byRoom := map[string][]string{}
		order := []string{}
		for _, s := range q.spawns() {
			if _, seen := byRoom[s.Room]; !seen {
				order = append(order, s.Room)
			}
			n := s.Count
			if n < 1 {
				n = 1
			}
			byRoom[s.Room] = append(byRoom[s.Room],
				fmt.Sprintf("%d× %s <subtle>(%s)</subtle>", n, codeID(s.Template), esc(orNone(s.Kind))))
		}
		for _, room := range order {
			out[room] = append(out[room], codeID(q.ID)+" — "+strings.Join(byRoom[room], ", "))
		}
	}
	return out
}

// roomNoteTags renders the start/craft/items/dark flags as pills.
func roomNoteTags(r roomJSON) []string {
	var t []string
	if r.Spawn {
		t = append(t, tag("start", "start room"))
	}
	if r.Station {
		t = append(t, tag("craft", "craft station"))
	}
	if r.Items {
		t = append(t, tag("", "items"))
	}
	if r.Light == "black" || r.Light == "dark" {
		t = append(t, tag("dark", "dark"))
	}
	return t
}

// npcHTML renders a mob as its name plus role pills.
func npcHTML(m mobJSON) string {
	var s strings.Builder
	s.WriteString("<strong>" + escName(m.Name) + "</strong>")
	for _, r := range roleTags(m, true) {
		s.WriteString(" " + r)
	}
	return s.String()
}

// roleTags renders a mob's roles as colored pills. withFaction appends the
// faction pill (the mob catalog keeps faction in its own column and omits it).
func roleTags(m mobJSON, withFaction bool) []string {
	var t []string
	if m.Shop {
		t = append(t, tag("shop", "shop"))
	}
	if m.Trainer {
		t = append(t, tag("trainer", "trainer"))
	}
	if m.Stable {
		t = append(t, tag("stable", "stable"))
	}
	if m.Hireling {
		t = append(t, tag("hire", "hireling"))
	}
	if m.Recruiter {
		t = append(t, tag("recruiter", "recruiter"))
	}
	if m.Quest {
		t = append(t, tag("quest", "quest giver"))
	}
	if m.Hostile {
		t = append(t, tag("hostile", "hostile"))
	}
	if withFaction && m.Faction != "" {
		t = append(t, tag("faction", "faction: "+m.Faction))
	}
	return t
}

// regionTitle prettifies a region id ("two-rivers" → "Two Rivers"); the empty
// region (areas that declare none) renders as a clear catch-all heading.
func regionTitle(id string) string {
	if id == "" {
		return "Unassigned region"
	}
	parts := strings.Split(id, "-")
	for i, p := range parts {
		if p != "" {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, " ")
}

// emptyLast orders non-empty strings alphabetically with the empty string last.
func emptyLast(x, y string) bool {
	if (x == "") != (y == "") {
		return x != ""
	}
	return x < y
}

func orNone(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

// writeSitePage writes one HTML page into packDir and returns its path (the
// []string shape every emitter returns).
func writeSitePage(packDir, name, html string) ([]string, error) {
	if err := os.MkdirAll(packDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating output dir: %w", err)
	}
	out := filepath.Join(packDir, name)
	if err := os.WriteFile(out, []byte(html), 0o644); err != nil {
		return nil, fmt.Errorf("writing %s: %w", out, err)
	}
	return []string{out}, nil
}
