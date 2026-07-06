package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// genericRecord is a schema-agnostic view of one content document: its id, name,
// description, and the remaining top-level fields rendered compactly. It lets the
// catalog document any content type the manifest declares without a bespoke
// struct per type.
type genericRecord struct {
	ID     string
	Name   string
	Desc   string
	Fields []string // "key: value" for remaining top-level fields, key-sorted
}

// loadGeneric loads every file matched by the manifest globs (relative to base)
// into generic records, sorted by id. Files without a top-level `id` fall back
// to their filename; a top-level list/scalar doc is summarized.
func loadGeneric(base string, globs []string) ([]genericRecord, error) {
	var recs []genericRecord
	for _, g := range globs {
		files, err := filepath.Glob(filepath.Join(base, g))
		if err != nil {
			return nil, fmt.Errorf("glob %q: %w", g, err)
		}
		sort.Strings(files)
		for _, f := range files {
			b, err := os.ReadFile(f)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", f, err)
			}
			var doc any
			if err := yaml.Unmarshal(b, &doc); err != nil {
				return nil, fmt.Errorf("%s: %w", f, err)
			}
			fname := strings.TrimSuffix(filepath.Base(f), filepath.Ext(f))
			recs = append(recs, toGeneric(doc, fname))
		}
	}
	// Stable so files with a duplicate/absent id keep file order (byte-stable).
	sort.SliceStable(recs, func(i, j int) bool { return recs[i].ID < recs[j].ID })
	return recs, nil
}

// toGeneric projects one YAML document into a genericRecord.
func toGeneric(doc any, fallbackID string) genericRecord {
	m, ok := doc.(map[string]any)
	if !ok {
		// A top-level list or scalar (rare) — summarize it under the filename.
		return genericRecord{ID: fallbackID, Fields: []string{compactValue(doc)}}
	}
	r := genericRecord{ID: yamlStr(m["id"]), Name: yamlStr(m["name"])}
	if r.ID == "" {
		r.ID = fallbackID
	}
	r.Desc = firstLine(yamlStr(m["description"]))
	if r.Desc == "" {
		r.Desc = firstLine(yamlStr(m["tagline"]))
	}
	skip := map[string]bool{"id": true, "name": true, "description": true, "tagline": true}
	keys := make([]string, 0, len(m))
	for k := range m {
		if !skip[k] {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		if v := compactValue(m[k]); v != "" {
			r.Fields = append(r.Fields, k+": "+v)
		}
	}
	return r
}

// renderGeneric renders records as an ID/Name/Description/Details table.
func renderGeneric(records []genericRecord) string {
	rows := make([][]string, 0, len(records))
	for _, r := range records {
		rows = append(rows, []string{
			codeID(r.ID), escName(r.Name), esc(r.Desc),
			esc(strings.Join(r.Fields, " · ")),
		})
	}
	return htmlTable([]string{"ID", "Name", "Description", "Details"}, rows)
}

// compactValue renders an arbitrary YAML value as a short one-line string:
// scalars verbatim, short scalar lists joined, and anything nested collapsed to
// a count so the Details cell stays scannable.
func compactValue(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return clean(x)
	case bool, int, int64, float64:
		return fmt.Sprint(x)
	case []any:
		parts := make([]string, 0, len(x))
		allScalar := true
		for _, e := range x {
			switch e.(type) {
			case string, bool, int, int64, float64:
				parts = append(parts, fmt.Sprint(e))
			default:
				allScalar = false
			}
		}
		if allScalar && len(parts) <= 10 {
			return strings.Join(parts, ", ")
		}
		return fmt.Sprintf("%d entries", len(x))
	case map[string]any:
		return fmt.Sprintf("%d fields", len(x))
	default:
		return fmt.Sprint(x)
	}
}

// yamlStr returns v as a string when it is a scalar, else "".
func yamlStr(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case bool, int, int64, float64:
		return fmt.Sprint(x)
	default:
		return ""
	}
}

// titleize turns a content-type key into a heading ("node_spawn_tables" →
// "Node Spawn Tables", "channel_map" → "Channel Map").
func titleize(s string) string {
	parts := strings.FieldsFunc(s, func(r rune) bool { return r == '_' || r == '-' })
	for i, p := range parts {
		if p != "" {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, " ")
}
