package transit

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Jasrags/AnotherMUD/internal/pack"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// Default timing (in Step units) applied when a line omits a field. One Step is
// one Service.Step call, i.e. one firing of the transit tick handler.
const (
	defaultTravelSteps uint64 = 2
	defaultDwellSteps  uint64 = 6
	defaultWarnSteps   uint64 = 1
	defaultOutKeyword         = "out"
	defaultDoorName           = "elevator door"
	// defaultDoorKeyID is a key id no player carries; the landing door stays
	// closed+locked with it while the car is away so it can't be hand-opened.
	defaultDoorKeyID = "transit-control"
)

// stopFile is the YAML shape of one stop in a line file.
type stopFile struct {
	Landing string `yaml:"landing"`
	Label   string `yaml:"label"`
	Code    string `yaml:"code"`
}

// lineFile is the YAML shape of a transit line (content/<pack>/transit/*.yaml).
// Ids (car, landings, safe_landing) are namespace-qualified against the owning
// pack the same way room ids are: an unqualified id resolves to "<ns>:id"; a
// qualified "other:id" crosses packs.
type lineFile struct {
	ID          string     `yaml:"id"`
	Name        string     `yaml:"name"`
	Policy      string     `yaml:"policy"`
	Car         string     `yaml:"car_room"`
	DoorDir     string     `yaml:"door_direction"`
	DoorName    string     `yaml:"door_name"`
	OutKeyword  string     `yaml:"out_keyword"`
	TravelSteps uint64     `yaml:"travel_steps"`
	DwellSteps  uint64     `yaml:"dwell_steps"`
	WarnSteps   uint64     `yaml:"warn_steps"`
	DefaultStop int        `yaml:"default_stop"`
	SafeLanding string     `yaml:"safe_landing"`
	Stops       []stopFile `yaml:"stops"`
}

// Load discovers the active packs under root (honoring the same filter
// pack.Load uses) and reads every content/<pack>/transit/*.yaml into a Line.
// Ids are namespace-qualified against the owning pack. The returned lines are
// sorted by id for deterministic wiring. A malformed line file is a hard error
// (fail fast at boot, like the pack loader).
func Load(root string, filter []string) ([]Line, error) {
	discovered, err := pack.Discover(root, filter)
	if err != nil {
		return nil, fmt.Errorf("discovering packs for transit load: %w", err)
	}
	var lines []Line
	for _, d := range discovered {
		ns := d.Namespace()
		dir := filepath.Join(d.Dir, "transit")
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue // a pack with no transit content is fine
			}
			return nil, fmt.Errorf("reading transit dir %s: %w", dir, err)
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
				continue
			}
			path := filepath.Join(dir, e.Name())
			l, err := decodeLine(path, ns)
			if err != nil {
				return nil, err
			}
			lines = append(lines, l)
		}
	}
	sort.Slice(lines, func(i, j int) bool { return lines[i].ID < lines[j].ID })
	return lines, nil
}

// decodeLine parses one line file and qualifies its ids against ns.
func decodeLine(path, ns string) (Line, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Line{}, fmt.Errorf("reading transit line %s: %w", path, err)
	}
	var lf lineFile
	if err := yaml.Unmarshal(raw, &lf); err != nil {
		return Line{}, fmt.Errorf("parsing transit line %s: %w", path, err)
	}
	if strings.TrimSpace(lf.ID) == "" {
		return Line{}, fmt.Errorf("transit line %s: missing id", path)
	}
	if len(lf.Stops) < 2 {
		return Line{}, fmt.Errorf("transit line %s: needs at least 2 stops", path)
	}
	if strings.TrimSpace(lf.Car) == "" {
		return Line{}, fmt.Errorf("transit line %s: missing car_room", path)
	}
	policy := CallPolicy(strings.TrimSpace(lf.Policy))
	if policy == "" {
		policy = PolicyOnDemand
	}
	if policy != PolicyOnDemand && policy != PolicyScheduled {
		return Line{}, fmt.Errorf("transit line %s: unknown policy %q", path, lf.Policy)
	}

	dir := world.DirNorth
	if d := strings.TrimSpace(lf.DoorDir); d != "" {
		parsed, ok := world.ParseDirection(d)
		if !ok {
			return Line{}, fmt.Errorf("transit line %s: unknown door_direction %q", path, lf.DoorDir)
		}
		dir = parsed
	}

	l := Line{
		ID:          qualify(lf.ID, ns),
		Name:        strings.TrimSpace(lf.Name),
		Policy:      policy,
		Car:         world.RoomID(qualify(lf.Car, ns)),
		DoorDir:     dir,
		DoorName:    strings.TrimSpace(orDefault(lf.DoorName, defaultDoorName)),
		DoorKeyID:   defaultDoorKeyID,
		OutKeyword:  strings.ToLower(strings.TrimSpace(orDefault(lf.OutKeyword, defaultOutKeyword))),
		TravelSteps: orDefaultU(lf.TravelSteps, defaultTravelSteps),
		DwellSteps:  orDefaultU(lf.DwellSteps, defaultDwellSteps),
		WarnSteps:   orDefaultU(lf.WarnSteps, defaultWarnSteps),
		DefaultStop: lf.DefaultStop,
	}
	if l.Name == "" {
		l.Name = "the elevator"
	}
	for i, s := range lf.Stops {
		if strings.TrimSpace(s.Landing) == "" {
			return Line{}, fmt.Errorf("transit line %s: stop %d missing landing", path, i)
		}
		label := strings.TrimSpace(s.Label)
		if label == "" {
			label = fmt.Sprintf("Stop %d", i+1)
		}
		l.Stops = append(l.Stops, Stop{
			Landing: world.RoomID(qualify(s.Landing, ns)),
			Label:   label,
			Code:    strings.ToUpper(strings.TrimSpace(s.Code)),
		})
	}
	if l.DefaultStop < 0 || l.DefaultStop >= len(l.Stops) {
		return Line{}, fmt.Errorf("transit line %s: default_stop %d out of range", path, l.DefaultStop)
	}
	if strings.TrimSpace(lf.SafeLanding) != "" {
		l.SafeLanding = world.RoomID(qualify(lf.SafeLanding, ns))
	} else {
		l.SafeLanding = l.Stops[l.DefaultStop].Landing
	}
	return l, nil
}

// qualify prepends "<ns>:" to an unqualified id; a qualified "pack:id" is kept.
func qualify(id, ns string) string {
	id = strings.TrimSpace(id)
	if strings.Contains(id, ":") {
		return id
	}
	return ns + ":" + id
}

func orDefault(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

func orDefaultU(v, def uint64) uint64 {
	if v == 0 {
		return def
	}
	return v
}
