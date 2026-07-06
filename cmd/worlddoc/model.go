package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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
	Terrain     string              `yaml:"terrain"`
	Exits       map[string]string   `yaml:"exits"`
	Mobs        []string            `yaml:"mobs"`
	Items       []any               `yaml:"items"`
	Properties  map[string]any      `yaml:"properties"`
	Doors       map[string]doorYAML `yaml:"doors"`
	HiddenExits map[string]any      `yaml:"hidden_exits"`
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

// questYAML picks just the giver link (quests/*.yaml). A room holding a giver
// mob gets the quest badge.
type questYAML struct {
	Giver string `yaml:"giver"`
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
}

type pt struct{ x, y, z int }

// worldModel is the shared parse of one pack: everything the emitters read.
// Parse once (loadPack), render many (map, gazetteer, catalogs, …).
type worldModel struct {
	Pack   string
	Start  string
	Areas  map[string]areaYAML
	Mobs   map[string]mobJSON
	Rooms  map[string]roomYAML
	Coords map[string]pt
}

// loadPack parses one content pack into a worldModel, laying rooms out with a
// BFS over the exit graph seeded at start (start may be empty — layout then
// falls back to a deterministic id-ordered seed and no spawn marker).
func loadPack(content, pack, start string) (*worldModel, error) {
	base := filepath.Join(content, pack)

	areas, err := loadAreas(filepath.Join(base, "areas"))
	if err != nil {
		return nil, fmt.Errorf("loading areas: %w", err)
	}
	questGivers, err := loadQuests(filepath.Join(base, "quests"))
	if err != nil {
		return nil, fmt.Errorf("loading quests: %w", err)
	}
	mobs, err := loadMobs(filepath.Join(base, "mobs"), questGivers)
	if err != nil {
		return nil, fmt.Errorf("loading mobs: %w", err)
	}
	rooms, err := loadRooms(filepath.Join(base, "rooms"))
	if err != nil {
		return nil, fmt.Errorf("loading rooms: %w", err)
	}
	if len(rooms) == 0 {
		return nil, fmt.Errorf("no rooms found under %s", base)
	}

	return &worldModel{
		Pack:   pack,
		Start:  start,
		Areas:  areas,
		Mobs:   mobs,
		Rooms:  rooms,
		Coords: layout(rooms, start),
	}, nil
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
		}
	}
	return out, nil
}

// loadQuests returns the set of mob ids that give a quest (quests/*.yaml
// `giver:`). Missing dir is fine — the pack may ship no quests.
func loadQuests(dir string) (map[string]bool, error) {
	files, _ := filepath.Glob(filepath.Join(dir, "*.yaml"))
	givers := make(map[string]bool, len(files))
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		var q questYAML
		if err := yaml.Unmarshal(b, &q); err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		if q.Giver != "" {
			givers[q.Giver] = true
		}
	}
	return givers, nil
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
	for _, t := range tags {
		if t == want {
			return true
		}
	}
	return false
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
