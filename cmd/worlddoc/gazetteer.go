package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// gazetteerEmitter renders gazetteer.md — a human-readable region → area → room
// reference. It reuses assemble() (the map's resolver) so exits, doors, hidden
// flags, and NPC roles stay consistent with the map, then groups the id-sorted
// rooms under their area and region.
var gazetteerEmitter = emitter{
	name: "gazetteer",
	render: func(m *worldModel, packDir string) ([]string, error) {
		md := renderGazetteer(assemble(m))
		out := filepath.Join(packDir, "gazetteer.md")
		if err := os.MkdirAll(packDir, 0o755); err != nil {
			return nil, fmt.Errorf("creating output dir: %w", err)
		}
		if err := os.WriteFile(out, []byte(md), 0o644); err != nil {
			return nil, fmt.Errorf("writing %s: %w", out, err)
		}
		return []string{out}, nil
	},
}

// renderGazetteer walks the resolved world room-by-room (so every room appears
// exactly once) grouped region → area → room. Rooms drive the grouping, so an
// area with no rooms simply doesn't appear (the health report flags those).
func renderGazetteer(w worldJSON) string {
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
	// Named regions/areas first (alphabetical), unassigned last; rooms by id.
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
	fmt.Fprintf(&b, "# %s — Gazetteer\n\n", w.Pack)
	fmt.Fprintf(&b, "Region → area → room reference for the `%s` content pack — %d rooms across %d areas. ",
		w.Pack, len(w.Rooms), len(w.Areas))
	b.WriteString("Derived from the pack YAML — regenerate with `make worlddoc` or the `world-docs` skill; do not hand-edit.\n")

	const none = "\x00" // sentinel so an empty region/area id still triggers a header
	curRegion, curArea := none, none
	for _, rw := range rows {
		if rw.region != curRegion {
			curRegion, curArea = rw.region, none
			fmt.Fprintf(&b, "\n## %s\n", regionTitle(rw.region))
		}
		if rw.areaID != curArea {
			curArea = rw.areaID
			title := rw.aName
			if title == "" {
				title = orNone(rw.areaID)
			}
			fmt.Fprintf(&b, "\n### %s", title)
			var annot []string
			if rw.areaID != "" {
				annot = append(annot, "`"+rw.areaID+"`")
			}
			if wz := areaWeather[rw.areaID]; wz != "" {
				annot = append(annot, "weather: "+wz)
			}
			if len(annot) > 0 {
				fmt.Fprintf(&b, " (%s)", strings.Join(annot, " · "))
			}
			b.WriteString("\n")
		}
		writeGazRoom(&b, rw.r)
	}
	return b.String()
}

func writeGazRoom(b *strings.Builder, r roomJSON) {
	fmt.Fprintf(b, "\n#### %s `%s`\n\n", r.Name, r.ID)
	fmt.Fprintf(b, "- Terrain: %s\n", orNone(r.Terrain))

	var notes []string
	if r.Spawn {
		notes = append(notes, "start room")
	}
	if r.Station {
		notes = append(notes, "craft station")
	}
	if r.Items {
		notes = append(notes, "items present")
	}
	if r.Light == "black" || r.Light == "dark" {
		notes = append(notes, "dark")
	}
	if len(notes) > 0 {
		fmt.Fprintf(b, "- Notes: %s\n", strings.Join(notes, ", "))
	}

	if len(r.Exits) > 0 {
		b.WriteString("- Exits:\n")
		for _, e := range r.Exits {
			var marks []string
			if e.Cross {
				marks = append(marks, "cross-area")
			}
			switch {
			case e.Locked && e.Door != "":
				marks = append(marks, "locked door: "+e.Door)
			case e.Locked:
				marks = append(marks, "locked")
			case e.Door != "":
				marks = append(marks, "door: "+e.Door)
			}
			if e.Hidden {
				marks = append(marks, "hidden")
			}
			suffix := ""
			if len(marks) > 0 {
				suffix = " (" + strings.Join(marks, ", ") + ")"
			}
			fmt.Fprintf(b, "    - %s → %s%s\n", e.Dir, e.To, suffix)
		}
	} else {
		b.WriteString("- Exits: none\n")
	}

	if len(r.Mobs) > 0 {
		b.WriteString("- NPCs:\n")
		for _, m := range r.Mobs {
			if roles := mobRoles(m, true); len(roles) > 0 {
				fmt.Fprintf(b, "    - %s (%s)\n", m.Name, strings.Join(roles, ", "))
			} else {
				fmt.Fprintf(b, "    - %s\n", m.Name)
			}
		}
	}
}

// mobRoles collapses a mob's flags into human-readable role labels. withFaction
// appends a "faction: <id>" label (the gazetteer wants it inline; the mob
// catalog keeps faction in its own column and passes false).
func mobRoles(m mobJSON, withFaction bool) []string {
	var r []string
	if m.Shop {
		r = append(r, "shop")
	}
	if m.Trainer {
		r = append(r, "trainer")
	}
	if m.Stable {
		r = append(r, "stable")
	}
	if m.Hireling {
		r = append(r, "hireling")
	}
	if m.Recruiter {
		r = append(r, "recruiter")
	}
	if m.Quest {
		r = append(r, "quest giver")
	}
	if m.Hostile {
		r = append(r, "hostile")
	}
	if withFaction && m.Faction != "" {
		r = append(r, "faction: "+m.Faction)
	}
	return r
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
