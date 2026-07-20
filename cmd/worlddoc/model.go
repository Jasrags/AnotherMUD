package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// markupRE strips the engine's {c}…{x} color markup from display names.
var markupRE = regexp.MustCompile(`\{[^}]*\}`)

// dirOrder is the deterministic visit order for the layout BFS, and dirDelta
// the per-direction unit step (room-coordinates §2.1: north = +y, east = +x,
// up = +z). Only these six directions appear in content.
var dirOrder = []string{"north", "south", "east", "west", "up", "down"}
var dirDelta = map[string][3]int{
	"north": {0, 1, 0}, "south": {0, -1, 0},
	"east": {1, 0, 0}, "west": {-1, 0, 0},
	"up": {0, 0, 1}, "down": {0, 0, -1},
}

// --- YAML shapes (partial — only the fields the emitters need) ---

type areaYAML struct {
	ID          string `yaml:"id"`
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Region      string `yaml:"region"`
	WeatherZone string `yaml:"weather_zone"`
}

type doorYAML struct {
	Name     string `yaml:"name"`
	Locked   bool   `yaml:"locked"`
	Key      string `yaml:"key"`
	Pickable bool   `yaml:"pickable"`
}

type roomYAML struct {
	ID          string              `yaml:"id"`
	Area        string              `yaml:"area"`
	Name        string              `yaml:"name"`
	Description string              `yaml:"description"`
	Terrain     string              `yaml:"terrain"`
	Exits       map[string]string   `yaml:"exits"`
	Mobs        []string            `yaml:"mobs"`
	Items       []any               `yaml:"items"`
	Properties  map[string]any      `yaml:"properties"`
	Doors       map[string]doorYAML `yaml:"doors"`
	HiddenExits map[string]any      `yaml:"hidden_exits"`
	Coord       *coordYAML          `yaml:"coord"`
}

// coordYAML is an authored area-local coordinate pin (room-coordinates §):
// the same shape the engine reads (internal/pack CoordFile). Used to place a
// room the exit-graph BFS can't reach (a transit car with no exits).
type coordYAML struct {
	X *int `yaml:"x"`
	Y *int `yaml:"y"`
	Z *int `yaml:"z"`
}

type mobYAML struct {
	ID          string         `yaml:"id"`
	Name        string         `yaml:"name"`
	Tags        []string       `yaml:"tags"`
	Trainer     any            `yaml:"trainer"`
	Mount       any            `yaml:"mount"`
	Hireling    any            `yaml:"hireling"`
	Recruiter   any            `yaml:"recruiter"`
	Faction     string         `yaml:"faction"`
	Properties  map[string]any `yaml:"properties"`
	Disposition struct {
		Default string `yaml:"default"`
	} `yaml:"disposition_rules"`
}

// --- catalog shapes (items/recipes/factions/quests) ---

// itemYAML is the item facts the catalog surfaces. value/weight live under
// properties; the weapon/armor fields are read only when present.
type itemYAML struct {
	ID              string         `yaml:"id"`
	Name            string         `yaml:"name"`
	Type            string         `yaml:"type"`
	Tags            []string       `yaml:"tags"`
	Properties      map[string]any `yaml:"properties"`
	EligibleSlots   []string       `yaml:"eligible_slots"`
	WeaponDamage    string         `yaml:"weapon_damage"`
	WeaponCategory  string         `yaml:"weapon_category"`
	ProficiencyTier string         `yaml:"proficiency_tier"`
	ArmorBonus      any            `yaml:"armor_bonus"`
	ArmorTier       string         `yaml:"armor_tier"`
}

type recipeIO struct {
	Template string `yaml:"template"`
	Quantity int    `yaml:"quantity"`
}

type recipeYAML struct {
	ID          string     `yaml:"id"`
	Name        string     `yaml:"name"`
	Discipline  string     `yaml:"discipline"`
	SkillFloor  int        `yaml:"skill_floor"`
	StationTier int        `yaml:"station_tier"`
	Inputs      []recipeIO `yaml:"inputs"`
	Output      recipeIO   `yaml:"output"`
}

type factionYAML struct {
	ID          string `yaml:"id"`
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

// questYAML is the full quest record the catalog surfaces; the giver link also
// feeds the map's quest badge (a room holding a giver mob gets it).
type questYAML struct {
	ID             string `yaml:"id"`
	Name           string `yaml:"name"`
	Classification string `yaml:"classification"`
	Giver          string `yaml:"giver"`
	Stages         []struct {
		ID     string           `yaml:"id"`
		Spawns []questSpawnYAML `yaml:"spawns"`
	} `yaml:"stages"`
	Reward struct {
		XP         int      `yaml:"xp"`
		Gold       int      `yaml:"gold"`
		Reputation int      `yaml:"reputation"`
		Abilities  []string `yaml:"abilities"`
		Faction    []struct {
			Faction string `yaml:"faction"`
			Delta   int    `yaml:"delta"`
		} `yaml:"faction"`
	} `yaml:"reward"`
}

// questSpawnYAML is one quest-scoped spawn entry (quest-spawns.md §2), read so
// the docs can show what a run creates at runtime — content that is absent from
// the static room lists because the engine spawns it on stage activation.
type questSpawnYAML struct {
	Kind     string `yaml:"kind"`
	Template string `yaml:"template"`
	Room     string `yaml:"room"`
	Count    int    `yaml:"count"`
}

// spawns flattens a quest's per-stage spawn declarations into one list.
func (q questYAML) spawns() []questSpawnYAML {
	var out []questSpawnYAML
	for _, s := range q.Stages {
		out = append(out, s.Spawns...)
	}
	return out
}

// mobJSON is a mob's rendered facts — built by loadMobs, consumed by the
// emitters. Its json tags also make it the map's per-room mob DTO.
type mobJSON struct {
	Name      string `json:"name"`
	Shop      bool   `json:"shop"`
	Trainer   bool   `json:"trainer"`
	Stable    bool   `json:"stable"`
	Hireling  bool   `json:"hireling"`
	Recruiter bool   `json:"recruiter"`
	Hostile   bool   `json:"hostile"`
	Quest     bool   `json:"quest"`
	Faction   string `json:"faction"`
	// Dialogue is the mob's `ask <npc> about <topic>` table (npc-dialogue.md),
	// surfaced in the catalogs mobs table. Excluded from the map JSON (json:"-")
	// — it's reference prose, not a map feature.
	Dialogue []dialogueTopic `json:"-"`
}

// dialogueTopic is one row of an NPC's dialogue table: the topic key and the
// one-or-more lines it answers with (a list rotates in-game).
type dialogueTopic struct {
	Topic string
	Lines []string
}

type pt struct{ x, y, z int }

// worldModel is the shared parse of one pack: everything the emitters read.
// Parse once (loadPack), render many (map, gazetteer, catalogs, …). The map and
// gazetteer read only Areas/Mobs/Rooms/Coords; the catalog fields below are
// parsed for the catalogs emitter and ignored by the others.
type worldModel struct {
	Pack    string
	Kind    string              // "world" or "library"
	Base    string              // the pack's content dir (for generic globbing)
	Content map[string][]string // manifest content map (type → globs)
	Start   string
	Areas   map[string]areaYAML
	Mobs    map[string]mobJSON
	Rooms   map[string]roomYAML
	Coords  map[string]pt

	Items    []itemYAML
	Recipes  []recipeYAML
	Factions []factionYAML
	Quests   []questYAML
}

// loadPack parses one content pack into a worldModel, laying rooms out with a
// BFS over the exit graph seeded at start (start may be empty — layout then
// falls back to a deterministic id-ordered seed and no spawn marker).
func loadPack(content, pack, start string) (*worldModel, error) {
	base := filepath.Join(content, pack)

	mf, err := loadManifest(base)
	if err != nil {
		return nil, err
	}

	areas, err := loadAreas(filepath.Join(base, "areas"))
	if err != nil {
		return nil, fmt.Errorf("loading areas: %w", err)
	}
	quests, err := loadAll(filepath.Join(base, "quests"), func(q questYAML) string { return q.ID })
	if err != nil {
		return nil, fmt.Errorf("loading quests: %w", err)
	}
	questGivers := make(map[string]bool, len(quests))
	for _, q := range quests {
		if q.Giver != "" {
			questGivers[q.Giver] = true
		}
	}
	mobs, err := loadMobs(filepath.Join(base, "mobs"), questGivers)
	if err != nil {
		return nil, fmt.Errorf("loading mobs: %w", err)
	}
	rooms, err := loadRooms(filepath.Join(base, "rooms"))
	if err != nil {
		return nil, fmt.Errorf("loading rooms: %w", err)
	}
	// Only a world pack must have rooms; a library (e.g. tapestry-core) ships
	// shared content — races, classes, abilities — but no world to walk.
	if mf.isWorld() && len(rooms) == 0 {
		return nil, fmt.Errorf("no rooms found under %s", base)
	}
	items, err := loadAll(filepath.Join(base, "items"), func(i itemYAML) string { return i.ID })
	if err != nil {
		return nil, fmt.Errorf("loading items: %w", err)
	}
	recipes, err := loadAll(filepath.Join(base, "recipes"), func(r recipeYAML) string { return r.ID })
	if err != nil {
		return nil, fmt.Errorf("loading recipes: %w", err)
	}
	factions, err := loadAll(filepath.Join(base, "factions"), func(f factionYAML) string { return f.ID })
	if err != nil {
		return nil, fmt.Errorf("loading factions: %w", err)
	}

	return &worldModel{
		Pack:     pack,
		Kind:     mf.Kind,
		Base:     base,
		Content:  mf.Content,
		Start:    start,
		Areas:    areas,
		Mobs:     mobs,
		Rooms:    rooms,
		Coords:   layout(rooms, start),
		Items:    items,
		Recipes:  recipes,
		Factions: factions,
		Quests:   quests,
	}, nil
}

// loadAll parses every *.yaml in dir into a []T (skipping records whose id, via
// the id func, is empty). A missing dir is fine — the pack may ship none.
func loadAll[T any](dir string, id func(T) string) ([]T, error) {
	files, _ := filepath.Glob(filepath.Join(dir, "*.yaml"))
	out := make([]T, 0, len(files))
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		var v T
		if err := yaml.Unmarshal(b, &v); err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		if id(v) != "" {
			out = append(out, v)
		}
	}
	return out, nil
}

// --- loaders ---

func loadAreas(dir string) (map[string]areaYAML, error) {
	files, _ := filepath.Glob(filepath.Join(dir, "*.yaml"))
	out := make(map[string]areaYAML, len(files))
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		var a areaYAML
		if err := yaml.Unmarshal(b, &a); err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		if a.ID != "" {
			out[a.ID] = a
		}
	}
	return out, nil
}

func loadRooms(dir string) (map[string]roomYAML, error) {
	files, _ := filepath.Glob(filepath.Join(dir, "*.yaml"))
	out := make(map[string]roomYAML, len(files))
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		var r roomYAML
		if err := yaml.Unmarshal(b, &r); err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		if r.ID != "" {
			out[r.ID] = r
		}
	}
	return out, nil
}

func loadMobs(dir string, questGivers map[string]bool) (map[string]mobJSON, error) {
	files, _ := filepath.Glob(filepath.Join(dir, "*.yaml"))
	out := make(map[string]mobJSON, len(files))
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		var m mobYAML
		if err := yaml.Unmarshal(b, &m); err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		if m.ID == "" {
			continue
		}
		_, hasShopProp := m.Properties["shop"]
		_, hasStableProp := m.Properties["stable"]
		out[m.ID] = mobJSON{
			Name:      clean(m.Name),
			Shop:      hasShopProp || hasTag(m.Tags, "shop"),
			Trainer:   m.Trainer != nil || hasTag(m.Tags, "skill_trainer"),
			Stable:    hasStableProp || hasTag(m.Tags, "stable"),
			Hireling:  m.Hireling != nil,
			Recruiter: m.Recruiter != nil,
			Hostile:   m.Disposition.Default == "hostile",
			Quest:     questGivers[m.ID],
			Faction:   m.Faction,
			Dialogue:  parseDialogue(m.Properties["dialogue"]),
		}
	}
	return out, nil
}

// parseDialogue reads a mob's `properties.dialogue` map (npc-dialogue.md §2)
// into an ordered topic list. Each value is a single line or a list of lines;
// topics sort alphabetically with the `default` fallback moved to the end so it
// reads last. Returns nil when there is no dialogue.
func parseDialogue(raw any) []dialogueTopic {
	mp, ok := raw.(map[string]any)
	if !ok || len(mp) == 0 {
		return nil
	}
	keys := make([]string, 0, len(mp))
	for k := range mp {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	// The catch-all reads best last, regardless of alpha order.
	sort.SliceStable(keys, func(i, j int) bool {
		return keys[j] == "default" && keys[i] != "default"
	})
	topics := make([]dialogueTopic, 0, len(keys))
	for _, k := range keys {
		if lines := dialogueLines(mp[k]); len(lines) > 0 {
			topics = append(topics, dialogueTopic{Topic: k, Lines: lines})
		}
	}
	return topics
}

// dialogueLines normalizes a topic value (a string or a YAML list of strings)
// into a trimmed, non-empty slice of lines.
func dialogueLines(v any) []string {
	switch x := v.(type) {
	case string:
		if s := strings.TrimSpace(x); s != "" {
			return []string{s}
		}
	case []any:
		out := make([]string, 0, len(x))
		for _, e := range x {
			if s, ok := e.(string); ok {
				if s = strings.TrimSpace(s); s != "" {
					out = append(out, s)
				}
			}
		}
		return out
	}
	return nil
}

// --- layout: global BFS over the exit graph with collision spread ---

func layout(rooms map[string]roomYAML, start string) map[string]pt {
	coords := make(map[string]pt, len(rooms))
	occupied := make(map[pt]string, len(rooms))

	place := func(id string, want pt) {
		if _, taken := occupied[want]; !taken {
			occupied[want] = id
			coords[id] = want
			return
		}
		// Spiral outward on the same z-plane for the nearest free cell.
		for r := 1; r < 2000; r++ {
			for dx := -r; dx <= r; dx++ {
				for dy := -r; dy <= r; dy++ {
					if abs(dx) != r && abs(dy) != r {
						continue // ring only
					}
					q := pt{want.x + dx, want.y + dy, want.z}
					if _, taken := occupied[q]; !taken {
						occupied[q] = id
						coords[id] = q
						return
					}
				}
			}
		}
		coords[id] = want // give up (pathological); overlap is harmless visually
	}

	visited := make(map[string]bool, len(rooms))
	bfs := func(seed string, origin pt) {
		place(seed, origin)
		visited[seed] = true
		queue := []string{seed}
		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]
			cp := coords[cur]
			for _, dir := range dirOrder {
				to := rooms[cur].Exits[dir]
				if to == "" || visited[to] {
					continue
				}
				if _, ok := rooms[to]; !ok {
					continue // dangling / cross-pack target — skip
				}
				d := dirDelta[dir]
				place(to, pt{cp.x + d[0], cp.y + d[1], cp.z + d[2]})
				visited[to] = true
				queue = append(queue, to)
			}
		}
	}

	// Main component from the spawn seed, then any disconnected components
	// stacked below it (deterministic: seeds sorted by id).
	if _, ok := rooms[start]; ok {
		bfs(start, pt{0, 0, 0})
	} else if start != "" {
		// A named-but-missing start is almost always a typo — surface it rather
		// than quietly laying out with no spawn seed.
		fmt.Fprintf(os.Stderr, "worlddoc: start room %q not found; laying out without a spawn seed\n", start)
	}

	// Honor `coord:` pins (room-coordinates §) for rooms the BFS couldn't reach —
	// a transit car with no exits. The engine derives pins per-AREA (area-local
	// origins) while this layout is global, so a pin can't be applied as an
	// absolute cell. Instead anchor an unreached pinned room on an already-placed,
	// pinned room in the SAME area: global = reference.global + (pin - reference.pin),
	// which reproduces the authored area-local geometry (e.g. the elevator car one
	// cell east of the concourse) beside the area's placed rooms. Deterministic:
	// the reference is the lexicographically-smallest placed+pinned room per area.
	pinOf := func(id string) (pt, bool) {
		c := rooms[id].Coord
		if c == nil || c.X == nil || c.Y == nil || c.Z == nil {
			return pt{}, false
		}
		return pt{*c.X, *c.Y, *c.Z}, true
	}
	sortedIDs := make([]string, 0, len(rooms))
	for id := range rooms {
		sortedIDs = append(sortedIDs, id)
	}
	sort.Strings(sortedIDs)
	type ref struct{ global, pin pt }
	areaRef := map[string]ref{}
	for _, id := range sortedIDs {
		if !visited[id] {
			continue
		}
		if p, ok := pinOf(id); ok {
			if _, seen := areaRef[rooms[id].Area]; !seen {
				areaRef[rooms[id].Area] = ref{global: coords[id], pin: p}
			}
		}
	}
	for _, id := range sortedIDs {
		if visited[id] {
			continue
		}
		p, ok := pinOf(id)
		if !ok {
			continue
		}
		r, ok := areaRef[rooms[id].Area]
		if !ok {
			continue // no placed+pinned anchor in this area — fall to the stack below
		}
		place(id, pt{r.global.x + p.x - r.pin.x, r.global.y + p.y - r.pin.y, r.global.z + p.z - r.pin.z})
		visited[id] = true
	}

	ids := make([]string, 0, len(rooms))
	for id := range rooms {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if visited[id] {
			continue
		}
		lowest := 0
		for _, c := range coords {
			if c.y < lowest {
				lowest = c.y
			}
		}
		bfs(id, pt{0, lowest - 4, 0})
	}
	return coords
}

// --- helpers ---

func clean(s string) string { return strings.TrimSpace(markupRE.ReplaceAllString(s, "")) }

func hasTag(tags []string, want string) bool {
	return slices.Contains(tags, want)
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
