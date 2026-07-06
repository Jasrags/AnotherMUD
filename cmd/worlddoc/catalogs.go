package main

import (
	"fmt"
	"sort"
	"strings"
)

// catalogsEmitter writes catalogs.html — reference tables of what the pack ships
// (mobs, items, recipes, factions, quests) as one page with in-page section
// links. Derived from the shared parse. Item source/placement (loot, shop,
// recipe) is deliberately not cross-referenced yet — see the plan's open
// question — so the item table lists what exists and its facts, not where it drops.
var catalogsEmitter = emitter{
	name: "catalogs",
	render: func(m *worldModel, packDir string) ([]string, error) {
		secs := []struct {
			id, title, body string
			count           int
		}{
			{"mobs", "Mobs", catalogMobs(m), len(m.Mobs)},
			{"items", "Items", catalogItems(m), len(m.Items)},
			{"recipes", "Recipes", catalogRecipes(m), len(m.Recipes)},
			{"factions", "Factions", catalogFactions(m), len(m.Factions)},
			{"quests", "Quests", catalogQuests(m), len(m.Quests)},
		}

		var b strings.Builder
		b.WriteString(`<p class="note">`)
		links := make([]string, len(secs))
		for i, s := range secs {
			links[i] = fmt.Sprintf(`<a href="#%s">%s (%d)</a>`, s.id, esc(s.title), s.count)
		}
		b.WriteString(strings.Join(links, " · "))
		b.WriteString("</p>")
		for _, s := range secs {
			fmt.Fprintf(&b, `<h2 id="%s">%s <span class="count %s">%d</span></h2>`,
				s.id, esc(s.title), countClass(s.count), s.count)
			if s.count == 0 {
				fmt.Fprintf(&b, `<p class="empty">No %s.</p>`, esc(strings.ToLower(s.title)))
				continue
			}
			b.WriteString(s.body)
		}

		lede := "Reference tables of the content this pack ships."
		page, err := renderPage(m.Pack, "catalogs", "Catalogs", lede, b.String())
		if err != nil {
			return nil, err
		}
		return writeSitePage(packDir, "catalogs.html", page)
	},
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
			faction,
			codeList(rooms),
		})
	}
	return htmlTable([]string{"ID", "Name", "Roles", "Faction", "Rooms"}, rows)
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
			esc(questReward(q)),
		})
	}
	return htmlTable([]string{"ID", "Name", "Type", "Giver", "Stages", "Reward"}, rows)
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
