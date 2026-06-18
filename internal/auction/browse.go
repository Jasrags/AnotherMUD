package auction

import (
	"sort"
	"strings"
	"time"
)

// SortKey selects the browse ordering (§5).
type SortKey int

const (
	// SortByTime — closing soonest first (the default; rewards urgency).
	SortByTime SortKey = iota
	// SortByPrice — cheapest first.
	SortByPrice
)

// defaultPageSize is used when the config leaves PageSize unset.
const defaultPageSize = 10

// BrowseFilter narrows and orders the active listings (§5). A zero filter
// (no name, no category, SortByTime, page 0→1) lists everything by closing
// time, first page.
type BrowseFilter struct {
	Name     string  // case-insensitive substring of the item name
	Category string  // exact (case-insensitive) item category/type
	Sort     SortKey // SortByTime (default) or SortByPrice
	Page     int     // 1-based; <=0 means page 1
}

// BrowsePage is one page of browse results plus the paging totals so the
// verb can render "page X/Y — N listings".
type BrowsePage struct {
	Listings   []Listing
	Page       int
	TotalPages int
	Total      int
}

// Browse filters, sorts, and paginates the active listings (§5). now is used
// for the time-remaining sort and is supplied by the caller (the verb passes
// the engine clock) so this stays testable.
func (m *Manager) Browse(now time.Time, f BrowseFilter) BrowsePage {
	all := m.store.ActiveListings()

	name := strings.ToLower(strings.TrimSpace(f.Name))
	cat := strings.ToLower(strings.TrimSpace(f.Category))
	filtered := all[:0:0]
	for _, l := range all {
		if name != "" && !strings.Contains(strings.ToLower(l.Item.Name), name) {
			continue
		}
		if cat != "" && !strings.EqualFold(l.Category, cat) {
			continue
		}
		filtered = append(filtered, l)
	}

	switch f.Sort {
	case SortByPrice:
		sort.SliceStable(filtered, func(i, j int) bool { return filtered[i].Price < filtered[j].Price })
	default: // SortByTime — soonest expiry first.
		sort.SliceStable(filtered, func(i, j int) bool { return filtered[i].ExpiresAt.Before(filtered[j].ExpiresAt) })
	}

	pageSize := m.cfg.PageSize
	if pageSize <= 0 {
		pageSize = defaultPageSize
	}
	total := len(filtered)
	totalPages := (total + pageSize - 1) / pageSize
	if totalPages == 0 {
		totalPages = 1
	}
	page := f.Page
	if page <= 0 {
		page = 1
	}
	if page > totalPages {
		page = totalPages
	}

	start := (page - 1) * pageSize
	end := start + pageSize
	if start > total {
		start = total
	}
	if end > total {
		end = total
	}
	return BrowsePage{
		Listings:   filtered[start:end],
		Page:       page,
		TotalPages: totalPages,
		Total:      total,
	}
}

// FindActiveByRef resolves a browse reference to an active listing id: the
// numeric suffix of the listing id (what browse displays) maps to "au-<n>",
// or the caller may pass the full id. Returns "" when no active listing
// matches. A stable per-listing reference (the id) — not a per-page ordinal
// — so it survives filter/sort/page changes (§5).
func (m *Manager) FindActiveByRef(ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	candidate := ref
	if !strings.HasPrefix(ref, "au-") {
		candidate = "au-" + ref
	}
	l, ok := m.store.Get(candidate)
	if !ok || l.Status != StatusActive {
		return ""
	}
	return l.ID
}
