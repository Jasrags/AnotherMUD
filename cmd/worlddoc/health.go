package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// healthEmitter writes health.md — an authoring-gap audit. It is report-only:
// findings never fail a run (no exit code). Checks: unreachable rooms, orphan
// rooms, dangling exit targets, one-way exits, undescribed rooms, empty areas,
// unknown mob references, dangling quest givers, and dangling quest reward
// factions. Cross-pack ids (containing ":") are assumed to resolve elsewhere and
// are not flagged.
var healthEmitter = emitter{
	name: "health",
	render: func(m *worldModel, packDir string) ([]string, error) {
		md := renderHealth(m)
		out := filepath.Join(packDir, "health.md")
		if err := os.MkdirAll(packDir, 0o755); err != nil {
			return nil, fmt.Errorf("creating output dir: %w", err)
		}
		if err := os.WriteFile(out, []byte(md), 0o644); err != nil {
			return nil, fmt.Errorf("writing %s: %w", out, err)
		}
		return []string{out}, nil
	},
}

func renderHealth(m *worldModel) string {
	rooms := m.Rooms
	roomIDs := sortedKeys(rooms)

	// Reachability from the start seed (skipped when there is no known start).
	_, startKnown := rooms[m.Start]
	reach := reachableFrom(rooms, m.Start)

	// Inbound-exit degree, for orphan detection.
	inbound := map[string]int{}
	for _, r := range rooms {
		for _, dir := range dirOrder {
			if to := r.Exits[dir]; to != "" {
				if _, ok := rooms[to]; ok {
					inbound[to]++
				}
			}
		}
	}

	var unreachable, orphans, dangling, oneWay, undescribed []string
	for _, id := range roomIDs {
		r := rooms[id]
		if startKnown && !reach[id] {
			unreachable = append(unreachable, roomLine(id, r))
		}
		if inbound[id] == 0 && id != m.Start {
			orphans = append(orphans, roomLine(id, r))
		}
		if strings.TrimSpace(r.Description) == "" {
			undescribed = append(undescribed, roomLine(id, r))
		}
		for _, dir := range dirOrder {
			to := r.Exits[dir]
			if to == "" {
				continue
			}
			tr, ok := rooms[to]
			if !ok {
				if !isCrossPack(to) {
					dangling = append(dangling, fmt.Sprintf("`%s` %s → `%s`", id, dir, to))
				}
				continue
			}
			if !hasExitTo(tr, id) {
				oneWay = append(oneWay, fmt.Sprintf("`%s` %s → `%s` (no return exit)", id, dir, to))
			}
		}
	}

	// Empty areas (declared but holding no rooms).
	usedAreas := map[string]bool{}
	for _, r := range rooms {
		usedAreas[r.Area] = true
	}
	var emptyAreas []string
	for _, aid := range sortedKeys(m.Areas) {
		if !usedAreas[aid] {
			emptyAreas = append(emptyAreas, fmt.Sprintf("`%s` (%s)", aid, orNone(m.Areas[aid].Name)))
		}
	}

	// Unknown mob references from room mob lists.
	var unknownMobs []string
	for _, id := range roomIDs {
		for _, mid := range rooms[id].Mobs {
			if isCrossPack(mid) {
				continue
			}
			if _, ok := m.Mobs[mid]; !ok {
				unknownMobs = append(unknownMobs, fmt.Sprintf("`%s` references mob `%s`", id, mid))
			}
		}
	}

	// Quest givers not placed / unknown, and dangling reward factions.
	placed := map[string]bool{}
	for _, r := range rooms {
		for _, mid := range r.Mobs {
			placed[mid] = true
		}
	}
	factionSet := map[string]bool{}
	for _, f := range m.Factions {
		factionSet[f.ID] = true
	}
	quests := append([]questYAML(nil), m.Quests...)
	sort.Slice(quests, func(i, j int) bool { return quests[i].ID < quests[j].ID })
	var questGivers, danglingFactions []string
	for _, q := range quests {
		if g := q.Giver; g != "" && !isCrossPack(g) {
			switch {
			case !mobExists(m, g):
				questGivers = append(questGivers, fmt.Sprintf("quest `%s` giver `%s` is not a known mob", q.ID, g))
			case !placed[g]:
				questGivers = append(questGivers, fmt.Sprintf("quest `%s` giver `%s` is not placed in any room", q.ID, g))
			}
		}
		for _, f := range q.Reward.Faction {
			if f.Faction != "" && !isCrossPack(f.Faction) && !factionSet[f.Faction] {
				danglingFactions = append(danglingFactions, fmt.Sprintf("quest `%s` reward references faction `%s`", q.ID, f.Faction))
			}
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# %s — World Health\n\n", m.Pack)
	fmt.Fprintf(&b, "Authoring-gap audit for the `%s` content pack — %d rooms across %d areas. ", m.Pack, len(rooms), len(m.Areas))
	b.WriteString("Report only: no finding fails a build. Derived from the pack YAML — regenerate with `make worlddoc` or the `world-docs` skill; do not hand-edit.\n")

	reachNote := ""
	if !startKnown {
		reachNote = fmt.Sprintf("_Reachability check skipped: no start room resolved for `%s`._", m.Pack)
	} else {
		reachNote = fmt.Sprintf("Rooms not reachable by walking exits from the start room `%s`.", m.Start)
	}

	healthSection(&b, "Unreachable rooms", unreachable, reachNote)
	healthSection(&b, "Orphan rooms", orphans, "Rooms with no inbound exit (you can leave but never arrive).")
	healthSection(&b, "Dangling exit targets", dangling, "Exits pointing at a room id that does not exist in this pack.")
	healthSection(&b, "One-way exits", oneWay, "Exits with no return exit from the destination — often intentional (chutes, portals).")
	healthSection(&b, "Rooms missing a description", undescribed, "")
	healthSection(&b, "Empty areas", emptyAreas, "Areas declared but holding no rooms.")
	healthSection(&b, "Unknown mob references", unknownMobs, "Room mob lists naming a mob id this pack does not define.")
	healthSection(&b, "Dangling quest givers", questGivers, "Quest givers that are unknown or never placed in a room.")
	healthSection(&b, "Dangling quest reward factions", danglingFactions, "Quest rewards granting standing with a faction this pack does not define.")
	return b.String()
}

// reachableFrom returns the set of rooms reachable by walking exits (to
// in-pack rooms) from start. Empty/unknown start yields an empty set.
func reachableFrom(rooms map[string]roomYAML, start string) map[string]bool {
	reach := map[string]bool{}
	if _, ok := rooms[start]; !ok {
		return reach
	}
	reach[start] = true
	queue := []string{start}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, dir := range dirOrder {
			to := rooms[cur].Exits[dir]
			if to == "" || reach[to] {
				continue
			}
			if _, ok := rooms[to]; !ok {
				continue
			}
			reach[to] = true
			queue = append(queue, to)
		}
	}
	return reach
}

func hasExitTo(r roomYAML, target string) bool {
	for _, dir := range dirOrder {
		if r.Exits[dir] == target {
			return true
		}
	}
	return false
}

func mobExists(m *worldModel, id string) bool {
	_, ok := m.Mobs[id]
	return ok
}

func roomLine(id string, r roomYAML) string {
	return fmt.Sprintf("`%s` (%s) — area `%s`", id, orNone(clean(r.Name)), orNone(r.Area))
}

func healthSection(b *strings.Builder, title string, lines []string, note string) {
	fmt.Fprintf(b, "\n## %s (%d)\n", title, len(lines))
	if note != "" {
		fmt.Fprintf(b, "\n%s\n", note)
	}
	b.WriteString("\n")
	if len(lines) == 0 {
		b.WriteString("_None._\n")
		return
	}
	for _, l := range lines {
		fmt.Fprintf(b, "- %s\n", l)
	}
}

// isCrossPack reports whether an id is namespaced to another pack (contains ":"),
// in which case it resolves outside this pack and is not a dangling reference.
func isCrossPack(id string) bool { return strings.Contains(id, ":") }
