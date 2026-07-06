package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// catalogsEmitter writes the reference tables under docs/world/<pack>/catalogs/:
// mobs, items, recipes, factions, and quests. Each is a deterministic Markdown
// table derived from the shared parse. Item source/placement (loot, shop,
// recipe) is deliberately not cross-referenced yet — see the plan's open
// question — so items.md catalogs what exists and its facts, not where it drops.
var catalogsEmitter = emitter{
	name: "catalogs",
	render: func(m *worldModel, packDir string) ([]string, error) {
		dir := filepath.Join(packDir, "catalogs")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("creating catalogs dir: %w", err)
		}
		files := []struct {
			name string
			body string
		}{
			{"mobs.md", catalogMobs(m)},
			{"items.md", catalogItems(m)},
			{"recipes.md", catalogRecipes(m)},
			{"factions.md", catalogFactions(m)},
			{"quests.md", catalogQuests(m)},
		}
		paths := make([]string, 0, len(files))
		for _, f := range files {
			out := filepath.Join(dir, f.name)
			if err := os.WriteFile(out, []byte(f.body), 0o644); err != nil {
				return nil, fmt.Errorf("writing %s: %w", out, err)
			}
			paths = append(paths, out)
		}
		return paths, nil
	},
}

func catalogMobs(m *worldModel) string {
	// mob id → the rooms that place it (sorted, deduped-by-append order).
	placement := map[string][]string{}
	for _, r := range m.Rooms {
		for _, mid := range r.Mobs {
			placement[mid] = append(placement[mid], r.ID)
		}
	}
	ids := sortedKeys(m.Mobs)
	rows := make([][]string, 0, len(ids))
	for _, id := range ids {
		mob := m.Mobs[id]
		rooms := placement[id]
		sort.Strings(rooms)
		rows = append(rows, []string{
			id, mob.Name,
			joinOrDash(mobRoles(mob, false), ", "),
			orNone(mob.Faction),
			joinOrDash(rooms, ", "),
		})
	}
	return catalogDoc(m.Pack, "Mob Catalog", len(rows), "mobs",
		[]string{"ID", "Name", "Roles", "Faction", "Rooms"}, rows, "")
}

func catalogItems(m *worldModel) string {
	items := append([]itemYAML(nil), m.Items...)
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	rows := make([][]string, 0, len(items))
	for _, it := range items {
		rows = append(rows, []string{
			it.ID, clean(it.Name), orNone(it.Type),
			propStr(it.Properties, "value"),
			propStr(it.Properties, "weight"),
			itemDetails(it),
		})
	}
	note := "Placement and source (loot tables, shops, recipes) are not yet cross-referenced."
	return catalogDoc(m.Pack, "Item Catalog", len(rows), "items",
		[]string{"ID", "Name", "Type", "Value", "Weight", "Details"}, rows, note)
}

func catalogRecipes(m *worldModel) string {
	recipes := append([]recipeYAML(nil), m.Recipes...)
	sort.Slice(recipes, func(i, j int) bool { return recipes[i].ID < recipes[j].ID })
	rows := make([][]string, 0, len(recipes))
	for _, r := range recipes {
		rows = append(rows, []string{
			r.ID, clean(r.Name), orNone(r.Discipline),
			fmt.Sprintf("%d", r.StationTier),
			joinOrDash(ioStrings(r.Inputs), ", "),
			ioStr(r.Output),
		})
	}
	return catalogDoc(m.Pack, "Recipe Catalog", len(rows), "recipes",
		[]string{"ID", "Name", "Discipline", "Station tier", "Inputs", "Output"}, rows, "")
}

func catalogFactions(m *worldModel) string {
	factions := append([]factionYAML(nil), m.Factions...)
	sort.Slice(factions, func(i, j int) bool { return factions[i].ID < factions[j].ID })
	rows := make([][]string, 0, len(factions))
	for _, f := range factions {
		rows = append(rows, []string{f.ID, clean(f.Name), firstLine(f.Description)})
	}
	return catalogDoc(m.Pack, "Faction Catalog", len(rows), "factions",
		[]string{"ID", "Name", "Description"}, rows, "")
}

func catalogQuests(m *worldModel) string {
	quests := append([]questYAML(nil), m.Quests...)
	sort.Slice(quests, func(i, j int) bool { return quests[i].ID < quests[j].ID })
	rows := make([][]string, 0, len(quests))
	for _, q := range quests {
		rows = append(rows, []string{
			q.ID, clean(q.Name), orNone(q.Classification), orNone(q.Giver),
			fmt.Sprintf("%d", len(q.Stages)),
			questReward(q),
		})
	}
	return catalogDoc(m.Pack, "Quest Catalog", len(rows), "quests",
		[]string{"ID", "Name", "Type", "Giver", "Stages", "Reward"}, rows, "")
}

// catalogDoc assembles a catalog markdown file: a heading, a one-line summary
// (+ optional note), and the table (or a "none" line when empty).
func catalogDoc(pack, title string, count int, noun string, headers []string, rows [][]string, note string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s — %s\n\n", pack, title)
	fmt.Fprintf(&b, "%d %s in the `%s` content pack. ", count, noun, pack)
	b.WriteString("Derived from the pack YAML — regenerate with `make worlddoc` or the `world-docs` skill; do not hand-edit.\n")
	if note != "" {
		fmt.Fprintf(&b, "\n> %s\n", note)
	}
	b.WriteString("\n")
	if len(rows) == 0 {
		fmt.Fprintf(&b, "_No %s._\n", noun)
		return b.String()
	}
	mdTable(&b, headers, rows)
	return b.String()
}

// --- small formatting helpers ---

func mdTable(b *strings.Builder, headers []string, rows [][]string) {
	writeRow := func(cells []string) {
		esc := make([]string, len(cells))
		for i, c := range cells {
			esc[i] = strings.ReplaceAll(strings.ReplaceAll(c, "|", "\\|"), "\n", " ")
		}
		b.WriteString("| " + strings.Join(esc, " | ") + " |\n")
	}
	writeRow(headers)
	seps := make([]string, len(headers))
	for i := range seps {
		seps[i] = "---"
	}
	b.WriteString("| " + strings.Join(seps, " | ") + " |\n")
	for _, r := range rows {
		writeRow(r)
	}
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

func ioStrings(io []recipeIO) []string {
	out := make([]string, 0, len(io))
	for _, x := range io {
		out = append(out, ioStr(x))
	}
	return out
}

func ioStr(x recipeIO) string {
	if x.Template == "" {
		return "—"
	}
	return fmt.Sprintf("%s×%d", x.Template, x.Quantity)
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
