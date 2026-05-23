package pack

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/logging"
	"github.com/Jasrags/AnotherMUD/internal/world"
	"gopkg.in/yaml.v3"
)

// Errors callers may distinguish at the boundary.
var (
	ErrMissingArea     = errors.New("room references unknown area")
	ErrMissingExitRoom = errors.New("exit references unknown room")
	ErrInvalidContent  = errors.New("invalid content file")
)

// Load discovers packs under root, orders them by dependencies, and
// populates dst's registries with the resulting content (spec §3.3
// phases 1+2).
//
// M5.1 scope: areas, rooms, item templates. Tags, properties, mobs,
// scripts arrive in later milestones. Phase 1 records the loaded
// manifest list; Phase 2 reads YAML into each registry.
//
// Filter, when non-empty, restricts discovery (spec §2.4). Pass nil to
// load every active pack under root.
func Load(ctx context.Context, root string, filter []string, dst *Registries) error {
	logger := logging.From(ctx).With(slog.String("event", "pack.load"), slog.String("root", root))

	discovered, err := Discover(root, filter)
	if err != nil {
		return fmt.Errorf("discovery: %w", err)
	}
	ordered, err := Order(discovered)
	if err != nil {
		return fmt.Errorf("ordering: %w", err)
	}

	logger.Info("packs discovered", slog.Int("count", len(ordered)))

	// Phase 1: manifest pass. M2 records only; no tags/properties yet.
	for _, p := range ordered {
		logging.From(ctx).Info("pack manifest",
			slog.String("event", "pack.manifest"),
			slog.String("pack", p.Manifest.Name),
			slog.String("namespace", p.Namespace()),
		)
	}

	// Phase 2: content pass.
	for _, p := range ordered {
		if err := loadPackContent(ctx, p, dst); err != nil {
			return fmt.Errorf("pack %q: %w", p.Manifest.Name, err)
		}
	}

	// Cross-pack area validity check (spec §3.3 step 4) runs after every
	// pack has been read so cross-pack room→area refs resolve.
	if err := validateAreas(dst.World); err != nil {
		return err
	}

	// Exit-target validation runs last for the same reason.
	if err := validateExits(dst.World); err != nil {
		return err
	}

	return nil
}

func loadPackContent(ctx context.Context, p Discovered, dst *Registries) error {
	ns := p.Namespace()
	logger := logging.From(ctx).With(slog.String("pack", p.Manifest.Name), slog.String("namespace", ns))

	areaPaths, err := resolveGlobs(p.Dir, p.Manifest.Content.Areas)
	if err != nil {
		return err
	}
	roomPaths, err := resolveGlobs(p.Dir, p.Manifest.Content.Rooms)
	if err != nil {
		return err
	}
	itemPaths, err := resolveGlobs(p.Dir, p.Manifest.Content.Items)
	if err != nil {
		return err
	}

	// Areas first — rooms reference them (spec §3.3 step 2). TryAddArea
	// catches both intra-pack and cross-pack id collisions.
	for _, ap := range areaPaths {
		a, err := decodeArea(ap, ns)
		if err != nil {
			return err
		}
		if err := dst.World.TryAddArea(a); err != nil {
			return fmt.Errorf("%w (in %s)", err, ap)
		}
	}

	for _, rp := range roomPaths {
		r, err := decodeRoom(rp, ns)
		if err != nil {
			return err
		}
		if err := dst.World.TryAddRoom(r); err != nil {
			return fmt.Errorf("%w (in %s)", err, rp)
		}
	}

	// Item templates are namespace-scoped like rooms; TryAdd guards
	// cross-pack collisions. Spec inventory-equipment-items §2.1.
	for _, ip := range itemPaths {
		t, err := decodeItem(ip, ns)
		if err != nil {
			return err
		}
		if err := dst.Items.TryAdd(t); err != nil {
			return fmt.Errorf("%w (in %s)", err, ip)
		}
	}

	logger.Info("pack content loaded",
		slog.String("event", "pack.content"),
		slog.Int("areas", len(areaPaths)),
		slog.Int("rooms", len(roomPaths)),
		slog.Int("items", len(itemPaths)),
	)
	return nil
}

// resolveGlobs expands each pattern (relative to packDir) into matching
// files. Sorted for deterministic load order. Missing patterns surface
// as errors so authors notice typos.
//
// Matches MUST stay within packDir. A pattern containing ".." (or
// otherwise escaping) is rejected — packs may not read host files
// outside their own directory.
func resolveGlobs(packDir string, patterns []string) ([]string, error) {
	cleanRoot, err := filepath.Abs(packDir)
	if err != nil {
		return nil, fmt.Errorf("resolving pack dir %s: %w", packDir, err)
	}
	prefix := cleanRoot + string(os.PathSeparator)

	var out []string
	for _, pat := range patterns {
		full := filepath.Join(cleanRoot, filepath.FromSlash(pat))
		matches, err := filepath.Glob(full)
		if err != nil {
			return nil, fmt.Errorf("bad glob %q: %w", pat, err)
		}
		if len(matches) == 0 {
			return nil, fmt.Errorf("content pattern %q matched no files under %s", pat, packDir)
		}
		for _, m := range matches {
			absMatch, err := filepath.Abs(m)
			if err != nil {
				return nil, fmt.Errorf("resolving match %s: %w", m, err)
			}
			if absMatch != cleanRoot && !strings.HasPrefix(absMatch, prefix) {
				return nil, fmt.Errorf("content pattern %q escapes pack dir (%s)", pat, absMatch)
			}
			out = append(out, m)
		}
	}
	sort.Strings(out)
	return out, nil
}

func decodeArea(path, ns string) (*world.Area, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading area %s: %w", path, err)
	}
	var af AreaFile
	if err := yaml.Unmarshal(raw, &af); err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrInvalidContent, path, err)
	}
	if strings.TrimSpace(af.ID) == "" {
		return nil, fmt.Errorf("%w: %s: missing 'id'", ErrInvalidContent, path)
	}
	if strings.TrimSpace(af.Name) == "" {
		return nil, fmt.Errorf("%w: %s: missing 'name'", ErrInvalidContent, path)
	}
	id, err := qualifyID(af.ID, ns)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrInvalidContent, path, err)
	}
	return &world.Area{
		ID:          world.AreaID(id),
		Name:        af.Name,
		Description: af.Description,
	}, nil
}

func decodeRoom(path, ns string) (*world.Room, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading room %s: %w", path, err)
	}
	var rf RoomFile
	if err := yaml.Unmarshal(raw, &rf); err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrInvalidContent, path, err)
	}
	if strings.TrimSpace(rf.ID) == "" {
		return nil, fmt.Errorf("%w: %s: missing 'id'", ErrInvalidContent, path)
	}
	if strings.TrimSpace(rf.Area) == "" {
		return nil, fmt.Errorf("%w: %s: missing 'area'", ErrInvalidContent, path)
	}
	if strings.TrimSpace(rf.Name) == "" {
		return nil, fmt.Errorf("%w: %s: missing 'name'", ErrInvalidContent, path)
	}

	roomID, err := qualifyID(rf.ID, ns)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrInvalidContent, path, err)
	}
	areaID, err := qualifyID(rf.Area, ns)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: area: %v", ErrInvalidContent, path, err)
	}

	r := &world.Room{
		ID:          world.RoomID(roomID),
		AreaID:      world.AreaID(areaID),
		Name:        rf.Name,
		Description: rf.Description,
		Exits:       make(map[world.Direction]world.Exit, len(rf.Exits)),
	}
	for dirStr, target := range rf.Exits {
		dir, ok := world.ParseDirection(dirStr)
		if !ok {
			return nil, fmt.Errorf("%w: %s: unknown direction %q", ErrInvalidContent, path, dirStr)
		}
		targetID, err := qualifyID(target, ns)
		if err != nil {
			return nil, fmt.Errorf("%w: %s: exit %s: %v", ErrInvalidContent, path, dirStr, err)
		}
		r.Exits[dir] = world.Exit{Target: world.RoomID(targetID)}
	}
	return r, nil
}

func decodeItem(path, ns string) (*item.Template, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading item %s: %w", path, err)
	}
	var f ItemFile
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrInvalidContent, path, err)
	}
	if strings.TrimSpace(f.ID) == "" {
		return nil, fmt.Errorf("%w: %s: missing 'id'", ErrInvalidContent, path)
	}
	if strings.TrimSpace(f.Name) == "" {
		return nil, fmt.Errorf("%w: %s: missing 'name'", ErrInvalidContent, path)
	}
	if strings.TrimSpace(f.Type) == "" {
		return nil, fmt.Errorf("%w: %s: missing 'type'", ErrInvalidContent, path)
	}
	id, err := qualifyID(f.ID, ns)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrInvalidContent, path, err)
	}

	mods := make([]item.Modifier, 0, len(f.Modifiers))
	for i, m := range f.Modifiers {
		if strings.TrimSpace(m.Stat) == "" {
			return nil, fmt.Errorf("%w: %s: modifier[%d] missing 'stat'", ErrInvalidContent, path, i)
		}
		mods = append(mods, item.Modifier{Stat: m.Stat, Value: m.Value})
	}

	return &item.Template{
		ID:         item.TemplateID(id),
		Name:       f.Name,
		Type:       f.Type,
		Tags:       f.Tags,
		Keywords:   f.Keywords,
		Properties: f.Properties,
		Modifiers:  mods,
	}, nil
}

// qualifyID applies the namespace rule (spec §5.2): if id contains ':'
// it is already qualified; otherwise prepend the current pack namespace.
// Both halves of a qualified id must be non-empty after trimming, and
// the id must contain at most one ':' so we never produce a three-part
// "ns:foo:bar" that downstream code can't interpret.
func qualifyID(id, ns string) (string, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", errors.New("empty id")
	}
	if strings.Contains(id, ":") {
		parts := strings.Split(id, ":")
		if len(parts) != 2 {
			return "", fmt.Errorf("malformed qualified id %q (expected namespace:name)", id)
		}
		lhs := strings.TrimSpace(parts[0])
		rhs := strings.TrimSpace(parts[1])
		if lhs == "" || rhs == "" {
			return "", fmt.Errorf("malformed qualified id %q", id)
		}
		return lhs + ":" + rhs, nil
	}
	return ns + ":" + id, nil
}

// validateAreas walks every room in dst and ensures its area is known.
// Per spec §3.3 step 4 this is fatal regardless of validation mode.
func validateAreas(dst *world.World) error {
	for _, r := range dst.Rooms() {
		if _, err := dst.Area(r.AreaID); err != nil {
			return fmt.Errorf("%w: room %q -> area %q", ErrMissingArea, r.ID, r.AreaID)
		}
	}
	return nil
}

// validateExits walks every exit and ensures the target room exists.
func validateExits(dst *world.World) error {
	for _, r := range dst.Rooms() {
		for dir, e := range r.Exits {
			if _, err := dst.Room(e.Target); err != nil {
				return fmt.Errorf("%w: room %q exit %s -> %q", ErrMissingExitRoom, r.ID, dir, e.Target)
			}
		}
	}
	return nil
}
