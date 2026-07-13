package main

import (
	"fmt"
	"sort"
	"strings"
)

// catalogsEmitter writes catalogs.html — reference tables of every content type
// the pack's manifest declares, grouped into sections with an in-page sub-nav.
// The five gameplay types (mobs/items/recipes/factions/quests) get curated
// tables; every other declared type is documented generically (id/name/
// description/fields), so coverage tracks the manifest with no per-type code.
var catalogsEmitter = emitter{
	name: "catalogs",
	render: func(m *worldModel, packDir string) ([]string, error) {
		body, err := renderCatalogs(m)
		if err != nil {
			return nil, err
		}
		page, err := renderPage(m.Pack, "catalogs", "Catalogs",
			"Reference tables of the content this pack ships.", body)
		if err != nil {
			return nil, err
		}
		return writeSitePage(packDir, "catalogs.html", page)
	},
}

// catalogGroups is the display order and membership of the sub-nav groups. Any
// declared content type not listed here (and not skipped) lands in a trailing
// "Other" group, so nothing a pack ships goes undocumented.
var catalogGroups = []struct {
	title string
	types []string
}{
	{"Characters", []string{"races", "classes", "tracks", "backgrounds", "languages", "feats"}},
	{"Abilities & Effects", []string{"abilities", "effects", "channel_map", "channels"}},
	{"Creatures & Items", []string{"mobs", "items", "grades", "rarity", "essence"}},
	{"World & Crafting", []string{"biomes", "weather_zones", "recipes", "forage_tables", "node_templates", "node_spawn_tables", "loot_tables"}},
	{"Quests & Factions", []string{"quests", "factions"}},
	{"Engine", []string{"slots", "theme", "help", "emotes", "ranged_flavor"}},
}

// catalogSkip is the content types the catalog does not table: areas/rooms are
// the map + gazetteer, and scripts are Lua code, not data.
var catalogSkip = map[string]bool{"areas": true, "rooms": true, "scripts": true}

type catSection struct {
	typ, id, label, body string
	count                int
}

// renderCatalogs builds the grouped, manifest-driven catalog body. A load error
// for any declared type is propagated (not silently dropped), so corrupt content
// surfaces as a failed render rather than a missing section.
func renderCatalogs(m *worldModel) (string, error) {
	// Group order, with every ungrouped-but-declared type appended to "Other" so
	// nothing a pack ships goes undocumented.
	type grp struct {
		title string
		types []string
	}
	assigned := map[string]bool{}
	order := make([]grp, 0, len(catalogGroups)+1)
	for _, g := range catalogGroups {
		order = append(order, grp{g.title, g.types})
		for _, t := range g.types {
			assigned[t] = true
		}
	}
	var others []string
	for t := range m.Content {
		if !assigned[t] && !catalogSkip[t] {
			others = append(others, t)
		}
	}
	sort.Strings(others)
	order = append(order, grp{"Other", others})

	// Resolve each group's non-empty sections.
	type resolvedGroup struct {
		title string
		secs  []catSection
	}
	var groups []resolvedGroup
	for _, g := range order {
		var secs []catSection
		for _, t := range g.types {
			if _, declared := m.Content[t]; !declared || catalogSkip[t] {
				continue
			}
			s, err := resolveCatSection(m, t)
			if err != nil {
				return "", err
			}
			if s.count > 0 {
				secs = append(secs, s)
			}
		}
		if len(secs) > 0 {
			groups = append(groups, resolvedGroup{g.title, secs})
		}
	}

	if len(groups) == 0 {
		return `<p class="empty">This pack declares no catalogable content.</p>`, nil
	}

	var b strings.Builder
	// Grouped sub-nav.
	b.WriteString(`<p class="note">`)
	for gi, g := range groups {
		if gi > 0 {
			b.WriteString("<br>")
		}
		fmt.Fprintf(&b, "<strong>%s:</strong> ", esc(g.title))
		links := make([]string, len(g.secs))
		for i, s := range g.secs {
			links[i] = fmt.Sprintf(`<a href="#%s">%s (%d)</a>`, esc(s.id), esc(s.label), s.count)
		}
		b.WriteString(strings.Join(links, " · "))
	}
	b.WriteString("</p>")
	// Sections.
	for _, g := range groups {
		fmt.Fprintf(&b, "<h2>%s</h2>", esc(g.title))
		for _, s := range g.secs {
			fmt.Fprintf(&b, `<h3 id="%s">%s <span class="count %s">%d</span></h3>`,
				esc(s.id), esc(s.label), countClass(s.count), s.count)
			b.WriteString(s.body)
		}
	}
	return b.String(), nil
}

// resolveCatSection produces one content type's section — a curated table for
// the five gameplay types, a generic table for everything else.
func resolveCatSection(m *worldModel, typ string) (catSection, error) {
	s := catSection{typ: typ, id: typ, label: titleize(typ)}
	switch typ {
	case "mobs":
		s.count, s.body = len(m.Mobs), catalogMobs(m)
	case "items":
		s.count, s.body = len(m.Items), catalogItems(m)
	case "recipes":
		s.count, s.body = len(m.Recipes), catalogRecipes(m)
	case "factions":
		s.count, s.body = len(m.Factions), catalogFactions(m)
	case "quests":
		s.count, s.body = len(m.Quests), catalogQuests(m)
	default:
		recs, err := loadGeneric(m.Base, m.Content[typ])
		if err != nil {
			return catSection{}, fmt.Errorf("cataloging %s: %w", typ, err)
		}
		s.count, s.body = len(recs), renderGeneric(recs)
	}
	return s, nil
}

func catalogMobs(m *worldModel) string {
	placement := map[string][]string{}
	for _, r := range m.Rooms {
		for _, mid := range r.Mobs {
			placement[mid] = append(placement[mid], r.ID)
		}
	}
	rows := make([][]string, 0, len(m.Mobs))
	for _, id := range sortedKeys(m.Mobs) {
		mob := m.Mobs[id]
		rooms := placement[id]
		sort.Strings(rooms)
		faction := ""
		if mob.Faction != "" {
			faction = tag("faction", mob.Faction)
		}
		rows = append(rows, []string{
			codeID(id), escName(mob.Name),
			strings.Join(roleTags(mob, false), " "),
			dialogueCell(mob.Dialogue),
			faction,
			codeList(rooms),
		})
	}
	return htmlTable([]string{"ID", "Name", "Roles", "Dialogue", "Faction", "Rooms"}, rows)
}

// dialogueCell renders an NPC's `ask about <topic>` table (npc-dialogue.md) as a
// collapsed disclosure: the summary lists the topic keys, and expanding it shows
// each topic's line(s). Empty when the NPC has no dialogue (htmlTable renders a
// dash). Line text is escaped; the topic keys drive the summary.
func dialogueCell(topics []dialogueTopic) string {
	if len(topics) == 0 {
		return ""
	}
	names := make([]string, len(topics))
	for i, t := range topics {
		names[i] = t.Topic
	}
	var b strings.Builder
	fmt.Fprintf(&b, "<details class=\"dialogue\"><summary>%s</summary><dl>", esc(strings.Join(names, ", ")))
	for _, t := range topics {
		fmt.Fprintf(&b, "<dt>%s</dt>", esc(t.Topic))
		for _, ln := range t.Lines {
			fmt.Fprintf(&b, "<dd>%s</dd>", esc(ln))
		}
	}
	b.WriteString("</dl></details>")
	return b.String()
}

func catalogItems(m *worldModel) string {
	items := append([]itemYAML(nil), m.Items...)
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	rows := make([][]string, 0, len(items))
	for _, it := range items {
		rows = append(rows, []string{
			codeID(it.ID), escName(it.Name), esc(orNone(it.Type)),
			esc(propStr(it.Properties, "value")),
			esc(propStr(it.Properties, "weight")),
			esc(itemDetails(it)),
		})
	}
	note := `<p class="note">Placement and source (loot tables, shops, recipes) are not yet cross-referenced.</p>`
	return note + htmlTable([]string{"ID", "Name", "Type", "Value", "Weight", "Details"}, rows)
}

func catalogRecipes(m *worldModel) string {
	recipes := append([]recipeYAML(nil), m.Recipes...)
	sort.Slice(recipes, func(i, j int) bool { return recipes[i].ID < recipes[j].ID })
	rows := make([][]string, 0, len(recipes))
	for _, r := range recipes {
		inputs := make([]string, 0, len(r.Inputs))
		for _, x := range r.Inputs {
			inputs = append(inputs, ioHTML(x))
		}
		rows = append(rows, []string{
			codeID(r.ID), escName(r.Name), esc(orNone(r.Discipline)),
			fmt.Sprintf("%d", r.StationTier),
			strings.Join(inputs, ", "),
			ioHTML(r.Output),
		})
	}
	return htmlTable([]string{"ID", "Name", "Discipline", "Station tier", "Inputs", "Output"}, rows)
}

func catalogFactions(m *worldModel) string {
	factions := append([]factionYAML(nil), m.Factions...)
	sort.Slice(factions, func(i, j int) bool { return factions[i].ID < factions[j].ID })
	rows := make([][]string, 0, len(factions))
	for _, f := range factions {
		rows = append(rows, []string{codeID(f.ID), escName(f.Name), esc(firstLine(f.Description))})
	}
	return htmlTable([]string{"ID", "Name", "Description"}, rows)
}

func catalogQuests(m *worldModel) string {
	quests := append([]questYAML(nil), m.Quests...)
	sort.Slice(quests, func(i, j int) bool { return quests[i].ID < quests[j].ID })
	rows := make([][]string, 0, len(quests))
	for _, q := range quests {
		giver := ""
		if q.Giver != "" {
			giver = codeID(q.Giver)
		}
		rows = append(rows, []string{
			codeID(q.ID), escName(q.Name), esc(orNone(q.Classification)), giver,
			fmt.Sprintf("%d", len(q.Stages)),
			questSpawns(q),
			esc(questReward(q)),
		})
	}
	return htmlTable([]string{"ID", "Name", "Type", "Giver", "Stages", "Spawns", "Reward"}, rows)
}

// questSpawns renders a quest's quest-scoped spawns (quest-spawns.md) — the
// mobs/items it creates at runtime, which don't appear in the static room
// content. One line per entry: "N× <template> (<kind>) → <room>". Empty (a
// dash) for quests that spawn nothing.
func questSpawns(q questYAML) string {
	sp := q.spawns()
	if len(sp) == 0 {
		return ""
	}
	lines := make([]string, 0, len(sp))
	for _, s := range sp {
		n := s.Count
		if n < 1 {
			n = 1
		}
		lines = append(lines, fmt.Sprintf("%d× %s <subtle>(%s)</subtle> → %s",
			n, codeID(s.Template), esc(orNone(s.Kind)), codeID(s.Room)))
	}
	return strings.Join(lines, "<br>")
}

// --- formatting helpers ---

func countClass(n int) string {
	if n == 0 {
		return "zero"
	}
	return "some"
}

func codeList(ids []string) string {
	parts := make([]string, len(ids))
	for i, x := range ids {
		parts[i] = codeID(x)
	}
	return strings.Join(parts, ", ")
}

func ioHTML(x recipeIO) string {
	if x.Template == "" {
		return "—"
	}
	return fmt.Sprintf("%s ×%d", codeID(x.Template), x.Quantity)
}

func itemDetails(it itemYAML) string {
	var parts []string
	if it.WeaponDamage != "" {
		w := it.WeaponDamage
		if it.WeaponCategory != "" {
			w += " " + it.WeaponCategory
		}
		if it.ProficiencyTier != "" {
			w += " (" + it.ProficiencyTier + ")"
		}
		parts = append(parts, "weapon: "+w)
	}
	if it.ArmorBonus != nil {
		a := fmt.Sprintf("armor +%v", it.ArmorBonus)
		if it.ArmorTier != "" {
			a += " " + it.ArmorTier
		}
		parts = append(parts, a)
	}
	if len(it.EligibleSlots) > 0 {
		parts = append(parts, "slots: "+strings.Join(it.EligibleSlots, "/"))
	}
	return joinOrDash(parts, "; ")
}

func questReward(q questYAML) string {
	var parts []string
	r := q.Reward
	if r.XP != 0 {
		parts = append(parts, fmt.Sprintf("%d xp", r.XP))
	}
	if r.Gold != 0 {
		parts = append(parts, fmt.Sprintf("%d gold", r.Gold))
	}
	if r.Reputation != 0 {
		parts = append(parts, fmt.Sprintf("%+d renown", r.Reputation))
	}
	for _, f := range r.Faction {
		parts = append(parts, fmt.Sprintf("%+d %s", f.Delta, f.Faction))
	}
	if len(r.Abilities) > 0 {
		parts = append(parts, "teaches "+strings.Join(r.Abilities, ", "))
	}
	return joinOrDash(parts, "; ")
}

func propStr(p map[string]any, key string) string {
	if p == nil {
		return "—"
	}
	if v, ok := p[key]; ok {
		return fmt.Sprintf("%v", v)
	}
	return "—"
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

func joinOrDash(parts []string, sep string) string {
	if len(parts) == 0 {
		return "—"
	}
	return strings.Join(parts, sep)
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
